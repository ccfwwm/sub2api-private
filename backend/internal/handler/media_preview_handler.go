package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func activeMediaAdminTestKey(key *service.APIKey) bool {
	return key != nil && key.IsActive() && !key.IsExpired() && !key.IsQuotaExhausted() && key.Group != nil && key.Group.IsActive() && key.User != nil && key.User.IsActive()
}

func (h *MediaHandler) adminEffectiveProtocolPreview(c *gin.Context, keyID, groupID, accountID int64, operation string, input map[string]any, measure bool) {
	if h.keys == nil || h.cache == nil || h.Engine == nil {
		response.Error(c, 503, "Media preview is unavailable")
		return
	}
	key, err := h.keys.GetByID(c.Request.Context(), keyID)
	if err != nil || !activeMediaAdminTestKey(key) {
		response.BadRequest(c, "An active assigned API key and user are required")
		return
	}
	if groupID > 0 && key.Group.ID != groupID {
		response.BadRequest(c, "Preview group must match the API key's assigned group")
		return
	}
	model, ok := input["model"].(string)
	if !ok || model == "" {
		response.BadRequest(c, "Effective preview requires a public model in request.model")
		return
	}
	if key.Group.ModelAllowlistEnabled() && !key.Group.ModelAllowlist.Allows(model) {
		response.Forbidden(c, "Model is not allowed in this key's group")
		return
	}
	// Preserve the administrator's original auth context and audit identity.
	identityContext := c.Copy()
	identityContext.Set(string(middleware.ContextKeyAPIKey), key)
	identityContext.Set(string(middleware.ContextKeySubscription), nil)
	if key.Group.IsSubscriptionType() {
		if h.db == nil {
			response.Error(c, 503, "Subscription identity is unavailable")
			return
		}
		var count, subscriptionID int64
		err = h.db.QueryRowContext(c.Request.Context(), `SELECT count(*),coalesce(min(id),0) FROM user_subscriptions WHERE user_id=$1 AND group_id=$2 AND status='active' AND expires_at>now() AND deleted_at IS NULL`, key.UserID, key.Group.ID).Scan(&count, &subscriptionID)
		if err != nil {
			mediaError(c, err)
			return
		}
		if count != 1 {
			response.BadRequest(c, "A unique active subscription is required for this key's group")
			return
		}
		identityContext.Set(string(middleware.ContextKeySubscription), &service.UserSubscription{ID: subscriptionID})
	}
	identity, err := h.identity(identityContext)
	if err != nil {
		mediaError(c, err)
		return
	}
	preview, err := h.Engine.PreviewEffective(c.Request.Context(), operation, input, identity, accountID, measure)
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, preview)
}
