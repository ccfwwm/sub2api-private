package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *SystemHandler) ImageReleases(c *gin.Context) {
	versions, err := h.updateSvc.ListImageReleases(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusBadGateway, "Private release list is unavailable; check the read-only GitHub credential")
		return
	}
	response.Success(c, gin.H{"versions": versions})
}

func (h *SystemHandler) ImageDeploymentStatus(c *gin.Context) {
	result, err := service.CallImageDeployment(c.Request.Context(), nil)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *SystemHandler) RequestImageDeployment(c *gin.Context) {
	var req service.ImageDeploymentRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm ||
		(req.Action != "prepare" && req.Action != "deploy" && req.Action != "rollback") {
		response.Error(c, http.StatusBadRequest, "confirm the deployment action explicitly")
		return
	}
	result, err := service.CallImageDeployment(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusConflict, err.Error())
		return
	}
	response.Success(c, result)
}
