package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type effectivePreviewKeyRepo struct {
	service.APIKeyRepository
	key     *service.APIKey
	lookups int
	nilKey  bool
}

func (s *effectivePreviewKeyRepo) GetByID(_ context.Context, id int64) (*service.APIKey, error) {
	s.lookups++
	if s.nilKey {
		return nil, nil
	}
	if s.key == nil || id != s.key.ID {
		return nil, service.ErrAPIKeyNotFound
	}
	return s.key, nil
}

func TestAdminProtocolPaidTestRejectsInactiveKeyBeforeSubmission(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*effectivePreviewKeyRepo)
	}{
		{name: "nil key", mutate: func(repo *effectivePreviewKeyRepo) { repo.nilKey = true }},
		{name: "expired", mutate: func(repo *effectivePreviewKeyRepo) {
			expires := time.Now().Add(-time.Hour)
			repo.key.ExpiresAt = &expires
		}},
		{name: "quota exhausted", mutate: func(repo *effectivePreviewKeyRepo) { repo.key.Quota, repo.key.QuotaUsed = 1, 1 }},
		{name: "disabled key", mutate: func(repo *effectivePreviewKeyRepo) { repo.key.Status = "disabled" }},
		{name: "disabled group", mutate: func(repo *effectivePreviewKeyRepo) { repo.key.Group.Status = "disabled" }},
		{name: "disabled user", mutate: func(repo *effectivePreviewKeyRepo) { repo.key.User.Status = "disabled" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo := effectivePreviewHandlerFixture(t)
			tc.mutate(repo)
			h.Engine = nil // Reaching task creation or submission is a test failure.
			w := protocolRequest(t, "/admin/media/templates/test", gin.H{"api_key_id": 11, "operation": "video", "confirm_paid_submission": true, "request": gin.H{"model": "public-video", "seconds": 5}}, func(c *gin.Context) {
				c.Request.Header.Set("Idempotency-Key", "invalid-key-must-not-submit")
				h.AdminProtocolTest(c)
				_, keyPresent := middleware.GetAPIKeyFromContext(c)
				require.False(t, keyPresent)
			})
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "An active assigned API key is required")
			require.Equal(t, 1, repo.lookups)
		})
	}
}

func effectivePreviewHandlerFixture(t *testing.T) (*MediaHandler, *effectivePreviewKeyRepo) {
	t.Helper()
	groupID := int64(7)
	repo := &effectivePreviewKeyRepo{key: &service.APIKey{ID: 11, UserID: 12, Key: "sk-never-return-test-value", Status: service.StatusActive, GroupID: &groupID, User: &service.User{ID: 12, Status: service.StatusActive}, Group: &service.Group{ID: groupID, Status: service.StatusActive, Platform: service.PlatformImageVideo, RateMultiplier: 2, VideoRateIndependent: true, VideoRateMultiplier: 3}}}
	template := media.BuiltinTemplates()[0]
	cap := media.Capability{ID: "preview", Version: "1", Status: "enabled", Enabled: true, Model: "public-video", UpstreamModel: "private-upstream", MediaType: "video", Operations: []string{"video"}, GroupIDs: []int64{groupID}, AccountIDs: []int64{9}, Template: &template, TemplateRef: &media.TemplateRef{ID: template.ID, Version: 1}}
	price := "0.2"
	card := media.PriceCard{ID: "retail", Version: "4", Purpose: "sale", Currency: "USD", Status: "enabled", MediaType: "video", Operation: "video", SuccessPolicy: "any_output", MultiplierPolicy: "effective_media_then_time", Rules: []media.Rule{{Priority: 1, Charges: []media.Charge{{Name: "task", Unit: "request", QuantitySource: "task_count", Price: &price, Trigger: "success", Refundable: true, Method: "linear", Rounding: media.Rounding{Mode: "none", Step: "1", Minimum: "0", Scope: "per_output"}}}}}}
	card.ModelMatch.Kind, card.ModelMatch.Value = "exact", cap.Model
	card.Scope.Kind, card.Scope.Reference = "group", "7"
	engine := &media.Engine{Adapter: &media.Adapter{Client: &http.Client{}}, Config: func(_ context.Context, group int64) ([]media.Capability, []media.PriceCard, error) {
		require.Equal(t, groupID, group)
		return []media.Capability{cap}, []media.PriceCard{card}, nil
	}, Lookup: func(account, group int64) (media.Account, error) {
		require.EqualValues(t, 9, account)
		require.Equal(t, groupID, group)
		return media.Account{ID: account, Allowed: true, Key: "upstream-secret-never-return"}, nil
	}}
	cfg := &config.Config{}
	return &MediaHandler{keys: service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg), cache: service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil), Engine: engine}, repo
}

func TestAdminEffectivePreviewUsesRealKeyGroupAndMediaRateWithoutChangingAuditIdentity(t *testing.T) {
	h, repo := effectivePreviewHandlerFixture(t)
	w := protocolRequest(t, "/admin/media/templates/preview", gin.H{"mode": "effective", "api_key_id": 11, "group_id": 7, "account_id": 9, "operation": "video", "request": gin.H{"model": "public-video", "seconds": 5, "prompt": "animate"}}, func(c *gin.Context) {
		h.AdminProtocolPreview(c)
		subject, ok := middleware.GetAuthSubjectFromContext(c)
		require.True(t, ok)
		require.EqualValues(t, 42, subject.UserID)
		_, keyPresent := middleware.GetAPIKeyFromContext(c)
		require.False(t, keyPresent)
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, repo.lookups)
	var payload struct {
		Data media.EffectivePreview `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.True(t, payload.Data.RequestComplete)
	require.False(t, payload.Data.PaidSubmission)
	pricing, ok := payload.Data.Pricing.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "0.60000000", pricing["maximum_charge"])
	require.NotContains(t, w.Body.String(), "sk-never-return-test-value")
	require.NotContains(t, w.Body.String(), "upstream-secret-never-return")
	require.Equal(t, "private-upstream", payload.Data.Effective["upstream_model"])
}

func TestAdminEffectivePreviewRejectsInvalidIdentityAndUnpublishedOverrides(t *testing.T) {
	for _, tc := range []struct {
		name      string
		change    func(gin.H)
		mutateKey func(*service.APIKey)
		want      int
	}{
		{name: "group mismatch", change: func(b gin.H) { b["group_id"] = 99 }, want: 400},
		{name: "missing key", change: func(b gin.H) { delete(b, "api_key_id") }, want: 400},
		{name: "unknown mode", change: func(b gin.H) { b["mode"] = "guess" }, want: 400},
		{name: "draft template", change: func(b gin.H) { b["template"] = gin.H{"id": "draft"} }, want: 400},
		{name: "rule override", change: func(b gin.H) {
			b["rules"] = []any{gin.H{"phase": "semantic", "op": "fixed", "target": "seconds", "value": 99}}
		}, want: 400},
		{name: "upstream model override", change: func(b gin.H) { b["upstream_model"] = "another" }, want: 400},
		{name: "expired key", mutateKey: func(k *service.APIKey) { value := time.Now().Add(-time.Hour); k.ExpiresAt = &value }, want: 400},
		{name: "exhausted key", mutateKey: func(k *service.APIKey) { k.Quota, k.QuotaUsed = 1, 1 }, want: 400},
		{name: "disabled user", mutateKey: func(k *service.APIKey) { k.User.Status = "disabled" }, want: 400},
		{name: "disabled group", mutateKey: func(k *service.APIKey) { k.Group.Status = "disabled" }, want: 400},
		{name: "model allowlist", mutateKey: func(k *service.APIKey) {
			k.Group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"another-public-model"}}
		}, want: 403},
		{name: "not candidate account", change: func(b gin.H) { b["account_id"] = 99 }, want: 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo := effectivePreviewHandlerFixture(t)
			if tc.mutateKey != nil {
				tc.mutateKey(repo.key)
			}
			body := gin.H{"mode": "effective", "api_key_id": 11, "group_id": 7, "operation": "video", "request": gin.H{"model": "public-video", "seconds": 5}}
			if tc.change != nil {
				tc.change(body)
			}
			w := protocolRequest(t, "/admin/media/templates/preview", body, h.AdminProtocolPreview)
			require.Equal(t, tc.want, w.Code, w.Body.String())
		})
	}
}

func TestAdminEffectivePreviewSubscriptionIdentityOnlyReadsActiveSubscription(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			h, repo := effectivePreviewHandlerFixture(t)
			repo.key.Group.SubscriptionType = service.SubscriptionTypeSubscription
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			h.db = db
			mock.ExpectQuery(`SELECT count(*),coalesce(min(id),0) FROM user_subscriptions WHERE user_id=$1 AND group_id=$2 AND status='active' AND expires_at>now() AND deleted_at IS NULL`).WithArgs(int64(12), int64(7)).WillReturnRows(sqlmock.NewRows([]string{"count", "id"}).AddRow(count, 91))
			w := protocolRequest(t, "/admin/media/templates/preview", gin.H{"mode": "effective", "api_key_id": 11, "operation": "video", "request": gin.H{"model": "public-video", "seconds": 5}}, func(c *gin.Context) {
				h.AdminProtocolPreview(c)
				_, subscriptionPresent := middleware.GetSubscriptionFromContext(c)
				require.False(t, subscriptionPresent)
			})
			if count == 1 {
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				require.Contains(t, w.Body.String(), `"paid_submission":false`)
			} else {
				require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAdminEffectivePreviewOfflineURLPreservesIncompleteEvidence(t *testing.T) {
	h, _ := effectivePreviewHandlerFixture(t)
	w := protocolRequest(t, "/admin/media/templates/preview", gin.H{"mode": "effective", "api_key_id": 11, "operation": "video", "measure_media": false, "request": gin.H{"model": "public-video", "seconds": 5, "images": []any{"https://example.org/reference.png"}}}, h.AdminProtocolPreview)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload struct {
		Data media.EffectivePreview `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.False(t, payload.Data.RequestComplete)
	require.Len(t, payload.Data.MeasurementRequired, 1)
	require.NotNil(t, payload.Data.Request)
	require.Nil(t, payload.Data.Pricing)
}
