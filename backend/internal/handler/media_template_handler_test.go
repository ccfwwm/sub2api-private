package handler

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProtocolEndpointsDenyMissingAdministratorBeforeAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &MediaHandler{}
	for _, handler := range []gin.HandlerFunc{h.AdminProtocolList, h.AdminProtocolHistory, h.AdminProtocolSave, h.AdminProtocolExport, h.AdminProtocolArchive, h.AdminProtocolPreview, h.AdminProtocolDiscover, h.AdminProtocolTest, h.AdminProtocolEffective} {
		for _, identity := range []struct {
			name   string
			role   string
			userID int64
		}{
			{name: "missing role"},
			{name: "ordinary user", role: "user", userID: 10},
			{name: "admin without identity", role: "admin"},
			{name: "admin with invalid identity", role: "admin", userID: 0},
		} {
			t.Run(identity.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest("GET", "/", nil)
				if identity.role != "" {
					c.Set(string(middleware.ContextKeyUserRole), identity.role)
				}
				if identity.userID != 0 {
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: identity.userID})
				}
				handler(c)
				require.Equal(t, 403, w.Code)
			})
		}
	}
}

func protocolRequest(t *testing.T, path string, body any, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUserRole), "admin")
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	})
	router.POST(path, handler)
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, request)
	return w
}

func protocolTestHandler(t *testing.T) (*MediaHandler, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	})
	return &MediaHandler{Engine: &media.Engine{Store: &media.Store{DB: db}}}, mock
}

func protocolTemplate() media.ProtocolTemplate {
	template := media.BuiltinTemplates()[0]
	template.ID = "handler-template"
	return template
}

func expectProtocolVersion(mock sqlmock.Sqlmock, kind, id string, version int64, config any) {
	raw, _ := json.Marshal(config)
	mock.ExpectQuery(`SELECT id,version,NULL::bigint,status,config FROM media_config_versions WHERE kind=$1 AND id=$2 AND version=$3`).
		WithArgs(kind, id, version).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version", "published_version", "status", "config"}).AddRow(id, version, nil, "enabled", raw))
}

func expectProtocolHead(mock sqlmock.Sqlmock, kind, id string, current int64) {
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`).WithArgs("media-config").WillReturnResult(sqlmock.NewResult(0, 1))
	rows := sqlmock.NewRows([]string{"version", "published_version"})
	if current > 0 {
		rows.AddRow(current, nil)
	}
	mock.ExpectQuery(`SELECT version,published_version FROM media_config_heads WHERE kind=$1 AND id=$2 FOR UPDATE`).WithArgs(kind, id).WillReturnRows(rows)
}

type protocolStoredConfig struct {
	id, status, name string
	version          int64
}

func (expected protocolStoredConfig) Match(value driver.Value) bool {
	raw, ok := value.([]byte)
	if !ok {
		return false
	}
	var config map[string]any
	return json.Unmarshal(raw, &config) == nil && config["id"] == expected.id && config["status"] == expected.status && config["version"] == fmt.Sprint(expected.version) && (expected.name == "" || config["name"] == expected.name)
}

func expectProtocolWrite(mock sqlmock.Sqlmock, kind, id string, current int64, publish bool, name string) {
	expectProtocolHead(mock, kind, id, current)
	status := "draft"
	var published driver.Value
	if publish {
		status = "enabled"
		published = current + 1
	}
	mock.ExpectExec(`INSERT INTO media_config_versions(kind,id,version,status,config,actor_id) VALUES($1,$2,$3,$4,$5,$6)`).
		WithArgs(kind, id, current+1, status, protocolStoredConfig{id: id, status: status, name: name, version: current + 1}, int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO media_config_heads(kind,id,version,published_version) VALUES($1,$2,$3,$4) ON CONFLICT(kind,id) DO UPDATE SET version=excluded.version,published_version=excluded.published_version`).
		WithArgs(kind, id, current+1, published).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func TestProtocolActionContractsRejectBeforeEngineAccess(t *testing.T) {
	for _, resource := range []string{"templates", "bindings"} {
		for _, tc := range []struct {
			name, action string
			body         map[string]any
		}{
			{"import requires explicit expected version", "/import", map[string]any{"id": "config", "config": gin.H{}}},
			{"import forbids null expected version", "/import", map[string]any{"id": "config", "expected_version": nil, "config": gin.H{}}},
			{"import existing expected version", "/import", map[string]any{"id": "config", "expected_version": 1, "config": gin.H{}}},
			{"import cannot publish", "/import", map[string]any{"id": "config", "publish": true, "config": gin.H{}}},
			{"import cannot use history", "/import", map[string]any{"id": "config", "source_version": 1, "config": gin.H{}}},
			{"import cannot upgrade bindings", "/import", map[string]any{"id": "config", "upgrade_bindings": []string{"binding"}, "config": gin.H{}}},
			{"import requires config", "/import", map[string]any{"id": "config"}},
			{"publish requires existing version", "/publish", map[string]any{"id": "config", "expected_version": 0, "config": gin.H{}}},
			{"publish cannot use history", "/publish", map[string]any{"id": "config", "expected_version": 2, "source_version": 1}},
			{"rollback requires source", "/rollback", map[string]any{"id": "config", "expected_version": 2}},
			{"rollback requires existing head", "/rollback", map[string]any{"id": "config", "source_version": 1}},
			{"rollback requires positive source", "/rollback", map[string]any{"id": "config", "expected_version": 2, "source_version": -1}},
			{"rollback forbids configuration", "/rollback", map[string]any{"id": "config", "expected_version": 2, "source_version": 1, "config": gin.H{}}},
			{"rollback forbids null configuration", "/rollback", map[string]any{"id": "config", "expected_version": 2, "source_version": 1, "config": nil}},
			{"save cannot use history", "", map[string]any{"id": "config", "expected_version": 2, "source_version": 1}},
			{"save cannot supply zero source", "", map[string]any{"id": "config", "source_version": 0}},
			{"save requires config object", "", map[string]any{"id": "config", "config": []any{}}},
			{"save forbids null config", "", map[string]any{"id": "config", "config": nil}},
		} {
			t.Run(resource+"/"+tc.name, func(t *testing.T) {
				h := &MediaHandler{}
				w := protocolRequest(t, "/admin/media/"+resource+tc.action, tc.body, h.AdminProtocolSave)
				require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			})
		}
	}
}

func TestProtocolImportCreatesOnlyNewDrafts(t *testing.T) {
	for _, kind := range []string{"template", "binding"} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/existing=%v", kind, existing), func(t *testing.T) {
				h, mock := protocolTestHandler(t)
				id := "handler-template"
				var config any = protocolTemplate()
				resource := "templates"
				if kind == "binding" {
					id, resource = "handler-binding", "bindings"
					config = media.ModelBinding{ID: id, GroupID: 7}
				}
				if existing {
					expectProtocolHead(mock, kind, id, 3)
					mock.ExpectRollback()
				} else {
					expectProtocolWrite(mock, kind, id, 0, false, "")
				}
				w := protocolRequest(t, "/admin/media/"+resource+"/import", gin.H{"id": id, "expected_version": 0, "publish": false, "config": config}, h.AdminProtocolSave)
				if existing {
					require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
					require.Contains(t, w.Body.String(), `"version_conflict"`)
				} else {
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					require.Contains(t, w.Body.String(), `"status":"draft"`)
					require.Contains(t, w.Body.String(), `"published_version":null`)
				}
			})
		}
	}
}

func TestProtocolPublishAlwaysEnablesExistingConfiguration(t *testing.T) {
	for _, includeConfig := range []bool{false, true} {
		t.Run(fmt.Sprint(includeConfig), func(t *testing.T) {
			h, mock := protocolTestHandler(t)
			template := protocolTemplate()
			body := gin.H{"id": template.ID, "expected_version": 3, "publish": false}
			if includeConfig {
				body["config"] = template
			} else {
				expectProtocolVersion(mock, "template", template.ID, 3, template)
			}
			expectProtocolWrite(mock, "template", template.ID, 3, true, template.Name)
			w := protocolRequest(t, "/admin/media/templates/publish", body, h.AdminProtocolSave)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), `"published_version":4`)
			require.Contains(t, w.Body.String(), `"status":"enabled"`)
		})
	}
}

func TestProtocolPublishCannotCreateMissingConfiguration(t *testing.T) {
	h, mock := protocolTestHandler(t)
	template := protocolTemplate()
	expectProtocolHead(mock, "template", template.ID, 0)
	mock.ExpectRollback()
	w := protocolRequest(t, "/admin/media/templates/publish", gin.H{"id": template.ID, "expected_version": 2, "config": template}, h.AdminProtocolSave)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestProtocolRollbackPublishesHistoryAsNewVersion(t *testing.T) {
	h, mock := protocolTestHandler(t)
	template := protocolTemplate()
	template.Name = "Historical template"
	expectProtocolVersion(mock, "template", template.ID, 2, template)
	expectProtocolWrite(mock, "template", template.ID, 5, true, template.Name)
	w := protocolRequest(t, "/admin/media/templates/rollback", gin.H{"id": template.ID, "expected_version": 5, "source_version": 2, "publish": false}, h.AdminProtocolSave)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"version":6`)
	require.Contains(t, w.Body.String(), `"published_version":6`)
	require.Contains(t, w.Body.String(), template.Name)
}

func TestProtocolBindingPublicationChecksAccountEligibility(t *testing.T) {
	for _, action := range []string{"publish", "rollback"} {
		t.Run(action, func(t *testing.T) {
			h, mock := protocolTestHandler(t)
			binding := media.ModelBinding{ID: "handler-binding", GroupID: 7, Model: "public-model", AccountID: 9}
			body := gin.H{"id": binding.ID, "expected_version": 3, "publish": false}
			if action == "rollback" {
				body["source_version"] = 1
				expectProtocolVersion(mock, "binding", binding.ID, 1, binding)
			} else {
				body["config"] = binding
			}
			lookups := 0
			h.Engine.Lookup = func(account, group int64) (media.Account, error) {
				lookups++
				require.EqualValues(t, 9, account)
				require.EqualValues(t, 7, group)
				return media.Account{ID: account, Allowed: false}, nil
			}
			w := protocolRequest(t, "/admin/media/bindings/"+action, body, h.AdminProtocolSave)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Equal(t, 1, lookups)
			require.Contains(t, w.Body.String(), "Account is not eligible")
		})
	}
}

func TestProtocolSaveRetainsLegacyPublishFlag(t *testing.T) {
	for _, publish := range []bool{false, true} {
		t.Run(fmt.Sprint(publish), func(t *testing.T) {
			h, mock := protocolTestHandler(t)
			template := protocolTemplate()
			expectProtocolWrite(mock, "template", template.ID, 3, publish, template.Name)
			w := protocolRequest(t, "/admin/media/templates", gin.H{"id": template.ID, "expected_version": 3, "publish": publish, "config": template}, h.AdminProtocolSave)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		})
	}
}

func TestProtocolPreviewNeedsNoEngineNetworkOrDatabase(t *testing.T) {
	// A nil engine makes any accidental DB/client access fail this local preview.
	h := &MediaHandler{}
	template := protocolTemplate()
	w := protocolRequest(t, "/admin/media/templates/preview", gin.H{
		"template": template, "operation": "video", "model": "preview-model",
		"request": gin.H{"prompt": "Preview", "seconds": 5, "images": []any{"https://example.invalid/reference.png"}},
	}, h.AdminProtocolPreview)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"online_media_get":false`)
	require.Contains(t, w.Body.String(), `"paid_submission":false`)
	require.Contains(t, w.Body.String(), `"request_hash"`)
	require.Contains(t, w.Body.String(), "https://example.invalid/reference.png")
}

func TestProtocolEffectiveSanitizesArbitraryConfigurationMaps(t *testing.T) {
	h, mock := protocolTestHandler(t)
	for _, template := range media.BuiltinTemplates() {
		mock.ExpectExec(`WITH seed AS (
 INSERT INTO media_config_versions(kind,id,version,status,config,actor_id)
 VALUES('template',$1,1,$2,$3,0) ON CONFLICT DO NOTHING RETURNING id
) INSERT INTO media_config_heads(kind,id,version,published_version)
 SELECT 'template',id,1,CASE WHEN $2='enabled' THEN 1 ELSE NULL END FROM seed ON CONFLICT DO NOTHING`).WithArgs(template.ID, template.Status, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectQuery(`SELECT h.id,h.version,h.published_version,v.status,v.config FROM media_config_heads h JOIN media_config_versions v ON v.kind=h.kind AND v.id=h.id AND v.version=h.published_version WHERE h.kind=$1 ORDER BY h.id`).WithArgs("binding").WillReturnRows(sqlmock.NewRows([]string{"id", "version", "published_version", "status", "config"}))
	h.Engine.Config = func(_ context.Context, group int64) ([]media.Capability, []media.PriceCard, error) {
		require.EqualValues(t, 7, group)
		return []media.Capability{{ID: "cap", Template: &media.ProtocolTemplate{Body: map[string]any{"api_key": "fixture-private-value", "custom": "https://example.invalid/image?token=fixture-query-value", "nested": map[string]any{"password": "fixture-password-value"}}}}}, nil, nil
	}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/media/bindings/effective?group_id=7", nil)
	c.Set(string(middleware.ContextKeyUserRole), "admin")
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	h.AdminProtocolEffective(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	for _, private := range []string{"fixture-private-value", "fixture-query-value", "fixture-password-value"} {
		require.NotContains(t, w.Body.String(), private)
	}
	require.Contains(t, w.Body.String(), `"api_key":"[redacted]"`)
	require.Contains(t, w.Body.String(), `"custom":"https://example.invalid/image"`)
}
