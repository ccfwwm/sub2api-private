package handler

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type mediaGroupAuthorizer interface {
	GetAvailableGroups(context.Context, int64) ([]service.Group, error)
}

// Capabilities is a JWT-authenticated, read-only user endpoint. It does not
// create an API key, reserve funds or bind any supplier account.
func (h *MediaHandler) Capabilities(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Query("group_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid group_id")
		return
	}
	if h.groups == nil {
		response.Error(c, 503, "Media catalog unavailable")
		return
	}
	groups, err := h.groups.GetAvailableGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var selected *service.Group
	for i := range groups {
		if groups[i].ID == id && groups[i].IsActive() && groups[i].Platform == service.PlatformImageVideo {
			selected = &groups[i]
			break
		}
	}
	if selected == nil {
		response.Forbidden(c, "Media group unavailable")
		return
	}
	entries, err := h.Engine.PublicCapabilities(c.Request.Context(), id)
	if err != nil {
		mediaError(c, err)
		return
	}
	visible := []media.PublicCapability{}
	for _, entry := range entries {
		if selected.ModelAllowlist.Allows(entry.Model) {
			visible = append(visible, entry)
		}
	}
	response.Success(c, gin.H{"group_id": id, "group_name": selected.Name, "platform": selected.Platform, "models": visible})
}
