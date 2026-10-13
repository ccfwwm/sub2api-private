package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) MediaTestResult(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	result, err := h.accountTestService.MediaTestResult(c.Request.Context(), id, c.Param("test_id"))
	if err != nil {
		response.NotFound(c, "Test not found")
		return
	}
	response.Success(c, result)
}
func (h *AccountHandler) MediaTestArtifact(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	path, mime, err := h.accountTestService.MediaTestArtifact(c.Request.Context(), id, c.Param("test_id"), c.Param("key"))
	if err != nil {
		response.NotFound(c, "Artifact not found")
		return
	}
	c.Header("Content-Type", mime)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.File(path)
}
