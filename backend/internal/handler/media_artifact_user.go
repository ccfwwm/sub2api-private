package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

type userArtifactTicket struct {
	Audience   string `json:"audience"`
	User       int64  `json:"user"`
	Task       string `json:"task"`
	Key        string `json:"key"`
	Expires    int64  `json:"expires"`
	Credential string `json:"credential"`
}

func (h *MediaHandler) userOwnedTask(c *gin.Context) (media.StoredTask, error) {
	t, err := h.Engine.Store.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		return t, err
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 || t.Identity.UserID != subject.UserID {
		return t, &media.Error{Status: 404, Code: "task_not_found", Message: "Task does not exist"}
	}
	return t, nil
}

func (h *MediaHandler) UserTask(c *gin.Context) {
	t, err := h.userOwnedTask(c)
	if err != nil {
		mediaError(c, err)
		return
	}
	rows, err := h.Engine.Store.TaskRecords(c.Request.Context(), []media.StoredTask{t}, false)
	if err != nil {
		mediaError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"record": rows[0]})
}

func (h *MediaHandler) userArtifactCredential(c *gin.Context, id int64) (string, error) {
	var email, password string
	err := h.Engine.Store.DB.QueryRowContext(c.Request.Context(), `SELECT email,password_hash FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL`, id).Scan(&email, &password)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(email + "\n" + password))
	return hex.EncodeToString(sum[:]), nil
}

func (h *MediaHandler) UserArtifactAccess(c *gin.Context) {
	t, err := h.userOwnedTask(c)
	if err != nil {
		mediaError(c, err)
		return
	}
	if t.State != "completed" {
		response.Error(c, 409, "Artifact not ready")
		return
	}
	file, _, _, err := h.Engine.Store.ArtifactFile(c.Request.Context(), t.ID, c.Param("key"))
	if err != nil {
		mediaError(c, err)
		return
	}
	_ = file.Close()
	credential, err := h.userArtifactCredential(c, t.Identity.UserID)
	if err != nil {
		c.AbortWithStatus(403)
		return
	}
	ticket := userArtifactTicket{Audience: "user_media_artifact", User: t.Identity.UserID, Task: t.ID, Key: c.Param("key"), Expires: time.Now().Add(5 * time.Minute).Unix(), Credential: credential}
	raw, err := json.Marshal(ticket)
	if err != nil {
		mediaError(c, err)
		return
	}
	sealed, err := h.Engine.Blobs.SealAccess(raw)
	if err != nil {
		mediaError(c, err)
		return
	}
	path := fmt.Sprintf("/api/v1/media/playback/tasks/%s/artifacts/%s/content?access=%s", url.PathEscape(t.ID), url.PathEscape(ticket.Key), base64.RawURLEncoding.EncodeToString(sealed))
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"url": path, "expires_at": ticket.Expires})
}

func (h *MediaHandler) UserArtifactContent(c *gin.Context) {
	raw, err := base64.RawURLEncoding.DecodeString(c.Query("access"))
	if err != nil {
		c.AbortWithStatus(403)
		return
	}
	opened, err := h.Engine.Blobs.OpenAccess(raw)
	if err != nil {
		c.AbortWithStatus(403)
		return
	}
	var ticket userArtifactTicket
	now := time.Now()
	if json.Unmarshal(opened, &ticket) != nil || ticket.Audience != "user_media_artifact" || ticket.User <= 0 || ticket.Task != c.Param("id") || ticket.Key != c.Param("key") || ticket.Expires <= now.Unix() || ticket.Expires > now.Add(5*time.Minute).Unix() {
		c.AbortWithStatus(403)
		return
	}
	credential, err := h.userArtifactCredential(c, ticket.User)
	if err != nil || credential != ticket.Credential {
		c.AbortWithStatus(403)
		return
	}
	t, err := h.Engine.Store.GetTask(c.Request.Context(), ticket.Task)
	if err != nil || t.Identity.UserID != ticket.User {
		c.AbortWithStatus(403)
		return
	}
	h.serveArtifact(c, t)
}
