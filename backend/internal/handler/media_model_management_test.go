package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func managementPrice(id, scope, reference, model, amount string) media.PriceCard {
	card := media.PriceCard{ID: id, Version: "1", Purpose: "sale", Status: "enabled", MediaType: "video", Operation: "video", Rules: []media.Rule{{Charges: []media.Charge{{Name: "generation", Unit: "request", Price: &amount, QuantitySource: "task_count", Trigger: "success", Method: "linear"}}}}}
	card.Scope.Kind, card.Scope.Reference = scope, reference
	card.ModelMatch.Kind, card.ModelMatch.Value = "exact", model
	return card
}

func TestAdminMediaPricingStatusUsesPublicNamesAndGroupPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cap := media.Capability{Model: "PublicVideo", UpstreamModel: "provider-video", MediaType: "video", AccountIDs: []int64{2139}, GroupIDs: []int64{19}, ChannelID: 8, Operations: []string{"video"}}
	channel := managementPrice("channel", "channel", "8", cap.Model, "5")
	group := managementPrice("group-zero", "group", "19", cap.Model, "0")
	for _, tc := range []struct {
		name    string
		cards   []media.PriceCard
		matched bool
		id      string
	}{
		{"channel", []media.PriceCard{channel}, true, "channel"},
		{"group zero wins", []media.PriceCard{channel, group}, true, "group-zero"},
		{"upstream alias does not match", []media.PriceCard{managementPrice("wrong", "channel", "8", cap.UpstreamModel, "2")}, false, ""},
		{"case sensitive", []media.PriceCard{managementPrice("wrong", "channel", "8", "publicvideo", "2")}, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/admin/media/pricing-status?group_id=19&account_id=2139", nil)
			c.Set(string(middleware.ContextKeyUserRole), "admin")
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
			h := &MediaHandler{Engine: &media.Engine{Config: func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
				return []media.Capability{cap}, tc.cards, nil
			}}}
			h.AdminPricingStatus(c)
			require.Equal(t, 200, w.Code, w.Body.String())
			var body struct {
				Data []struct {
					Model   string       `json:"model"`
					Matched bool         `json:"matched"`
					Reason  string       `json:"reason"`
					PriceID string       `json:"price_id"`
					Rules   []media.Rule `json:"rules"`
				}
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Len(t, body.Data, 1)
			row := body.Data[0]
			require.Equal(t, cap.Model, row.Model)
			require.Equal(t, tc.matched, row.Matched)
			require.Equal(t, tc.id, row.PriceID)
			if !tc.matched {
				require.Equal(t, "price_not_configured", row.Reason)
			}
			if tc.id == "group-zero" {
				require.Equal(t, "0", *row.Rules[0].Charges[0].Price)
			}
		})
	}
}

func TestMediaModelsIsolatesBrokenTemplateBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	var template media.ProtocolTemplate
	for _, candidate := range media.BuiltinTemplates() {
		for _, typ := range candidate.MediaTypes {
			if typ == "video" {
				template = candidate
				break
			}
		}
		if template.ID != "" {
			break
		}
	}
	require.NotEmpty(t, template.ID)
	for range media.BuiltinTemplates() {
		mock.ExpectExec("WITH seed AS").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	templateRaw, err := json.Marshal(template)
	require.NoError(t, err)
	cap := media.Capability{ID: "public", Model: "public-video", UpstreamModel: "upstream", MediaType: "video", Operations: []string{"video"}, GroupIDs: []int64{19}, AccountIDs: []int64{2139, 2140}, Enabled: true, Status: "enabled", TemplateRef: &media.TemplateRef{ID: template.ID, Version: 1}}
	binding := media.ModelBinding{ID: "broken", GroupID: 19, AccountID: 2139, Model: cap.Model, Status: "enabled", Rules: []media.ParameterRule{{Phase: "wire", Op: "rename", Source: "request", Target: "request.nested"}}}
	raw, err := json.Marshal(binding)
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		mock.ExpectQuery("SELECT h.id,h.version,h.published_version,v.status,v.config").WithArgs("binding").WillReturnRows(sqlmock.NewRows([]string{"id", "version", "published_version", "status", "config"}).AddRow("broken", 1, 1, "enabled", raw))
		mock.ExpectQuery("SELECT id,version,NULL::bigint,status,config").WithArgs("template", template.ID, int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "version", "published_version", "status", "config"}).AddRow(template.ID, 1, nil, "enabled", templateRaw))
	}
	price := managementPrice("group", "group", "19", cap.Model, "0")
	h := &MediaHandler{Engine: &media.Engine{Store: &media.Store{DB: db}, Config: func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
		return []media.Capability{cap}, []media.PriceCard{price}, nil
	}}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/v1/models", nil)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, Group: &service.Group{ID: 19, Platform: service.PlatformImageVideo}})
	h.Models(c, func(*gin.Context) { t.Fatal("media model listing must not fall back") })
	require.Equal(t, 200, w.Code, w.Body.String())
	var body struct {
		Data []struct {
			ID string `json:"id"`
		}
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.Equal(t, cap.Model, body.Data[0].ID)
	require.NotContains(t, w.Body.String(), "broken")
	require.NotContains(t, w.Body.String(), "2139")
	require.NoError(t, mock.ExpectationsWereMet())
}
