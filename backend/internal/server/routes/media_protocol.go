package routes

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Media keys never enter legacy text routing or its pricing fallbacks.
func mediaProtocolGuard(c *gin.Context) {
	if getGroupPlatform(c) != service.PlatformImageVideo {
		return
	}
	path := c.FullPath()
	if strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	allowed := false
	if c.Request.Method == http.MethodPost {
		switch path {
		case "/images/generations", "/images/edits", "/videos", "/videos/generations", "/chat/completions", "/audio/generations", "/music/generations", "/sub2api/estimates":
			allowed = true
		case "/v1beta/models/*modelAction":
			allowed = strings.HasSuffix(c.Param("modelAction"), ":generateContent")
		}
	} else if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		switch path {
		case "/models", "/sub2api/billing", "/videos/:request_id", "/videos/:request_id/content", "/videos/generations/:request_id", "/videos/generations/:request_id/content", "/audio/generations/:id", "/audio/generations/:id/content", "/tasks/:id/artifacts", "/tasks/:id/artifacts/:key/content":
			allowed = true
		}
	}
	if !allowed {
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "unsupported_media_endpoint", "message": "This endpoint is not available for imagevideo groups"}})
	}
}
