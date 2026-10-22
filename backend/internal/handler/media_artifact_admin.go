package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

type adminArtifactTicket struct {
	Admin      int64  `json:"admin"`
	Task       string `json:"task"`
	Key        string `json:"key"`
	Expires    int64  `json:"expires"`
	Credential string `json:"credential"`
}

func requireMediaAdmin(c *gin.Context) bool {
	role, ok := middleware.GetUserRoleFromContext(c)
	subject, authenticated := middleware.GetAuthSubjectFromContext(c)
	if !ok || role != "admin" || !authenticated || subject.UserID <= 0 {
		c.AbortWithStatus(403)
		return false
	}
	return true
}

// Provider extension field names and media payloads remain visible to an
// authenticated administrator. Account credentials remain redacted while
// signed media URLs remain reproducible.
func mediaReviewValue(value any, depth int) any { return media.SafeAdminReviewValue(value, nil, depth) }

func (h *MediaHandler) adminTaskReview(c *gin.Context) (gin.H, error) {
	t, err := h.Engine.Store.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		return nil, err
	}
	rows, err := h.Engine.Store.TaskRecords(c.Request.Context(), []media.StoredTask{t}, true)
	if err != nil {
		return nil, err
	}
	result := gin.H{
		"record":             rows[0],
		"incoming_request":   map[string]any{},
		"normalized_request": map[string]any{},
		"prepared_request":   nil,
		"request":            map[string]any{}, // backwards-compatible alias for normalized_request
		"response":           map[string]any{},
	}
	var request media.Request
	path, _ := t.Snapshot.Request.Parameters["request_blob"].(string)
	if path != "" && h.Engine.Blobs.ReadJSON(path, &request) == nil {
		result["incoming_request"] = mediaReviewValue(request.Incoming, 0)
		result["normalized_request"] = mediaReviewValue(request.Parameters, 0)
		result["request"] = result["normalized_request"]
		if request.Wire != nil {
			result["prepared_request"] = mediaReviewValue(request.Wire, 0)
		}
	}
	result["submission"] = nil
	receipt := h.Engine.ReadSubmission(t.ID)
	if receipt != nil {
		result["submission"] = receipt
		result["upstream_request"] = gin.H{
			"method":       receipt.Method,
			"path":         receipt.Path,
			"encoding":     receipt.Encoding,
			"body":         receipt.Request,
			"files":        receipt.Files,
			"references":   receipt.References,
			"request_hash": receipt.RequestHash,
		}
	}
	result["submission_status"] = "not_recorded"
	if receipt != nil {
		result["submission_status"] = receipt.Stage
	}
	var upstream map[string]any
	if t.ResultPath != "" && h.Engine.Blobs.ReadJSON(t.ResultPath, &upstream) == nil {
		result["response"] = mediaReviewValue(upstream, 0)
		result["upstream_response"] = result["response"]
	}
	// Also redact known credentials echoed inside otherwise permitted text fields.
	secrets := []string{}
	var key, providerKey string
	if err := h.Engine.Store.DB.QueryRowContext(c.Request.Context(), `SELECT k.key,coalesce(a.credentials->>'api_key','') FROM api_keys k LEFT JOIN accounts a ON a.id=$2 WHERE k.id=$1`, t.Identity.KeyID, t.AccountID).Scan(&key, &providerKey); err != nil {
		return nil, err
	}
	secrets = append(secrets, key, providerKey)
	for field, value := range result {
		result[field] = media.SafeAdminReviewValue(value, secrets, 0)
	}
	return result, nil
}
func (h *MediaHandler) AdminTask(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	result, err := h.adminTaskReview(c)
	if err != nil {
		mediaError(c, err)
		return
	}
	compact, ok := media.CompactEvidence(result).(map[string]any)
	if !ok {
		response.Error(c, http.StatusInternalServerError, "Failed to format media task evidence")
		return
	}
	result = compact
	c.Header("Cache-Control", "no-store")
	response.Success(c, result)
}
func (h *MediaHandler) AdminTaskExport(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	result, err := h.adminTaskReview(c)
	if err != nil {
		mediaError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "media-task.json"}))
	c.JSON(200, result)
}
func (h *MediaHandler) adminCredential(c *gin.Context, id int64) (string, error) {
	var email, password string
	err := h.Engine.Store.DB.QueryRowContext(c.Request.Context(), `SELECT email,password_hash FROM users WHERE id=$1 AND role='admin' AND status='active' AND deleted_at IS NULL`, id).Scan(&email, &password)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(email + "\n" + password))
	return hex.EncodeToString(sum[:]), nil
}
func (h *MediaHandler) AdminArtifactAccess(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	t, err := h.Engine.Store.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		mediaError(c, err)
		return
	}
	// Validate availability before issuing a scoped ticket; do not read a whole file.
	file, _, _, err := h.Engine.Store.ArtifactFile(c.Request.Context(), t.ID, c.Param("key"))
	if err != nil {
		mediaError(c, err)
		return
	}
	_ = file.Close()
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	credential, err := h.adminCredential(c, subject.UserID)
	if err != nil {
		c.Status(403)
		return
	}
	ticket := adminArtifactTicket{Admin: subject.UserID, Task: t.ID, Key: c.Param("key"), Expires: time.Now().Add(5 * time.Minute).Unix(), Credential: credential}
	raw, _ := json.Marshal(ticket)
	sealed, err := h.Engine.Blobs.SealAccess(raw)
	if err != nil {
		mediaError(c, err)
		return
	}
	access := base64.RawURLEncoding.EncodeToString(sealed)
	path := fmt.Sprintf("/api/v1/media/review/tasks/%s/artifacts/%s/content?access=%s", url.PathEscape(t.ID), url.PathEscape(ticket.Key), access)
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"url": path, "expires_at": ticket.Expires})
}
func (h *MediaHandler) ArtifactAccessAuth(c *gin.Context) {
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
	var ticket adminArtifactTicket
	if json.Unmarshal(opened, &ticket) != nil || ticket.Admin <= 0 || ticket.Expires <= time.Now().Unix() || ticket.Expires > time.Now().Add(5*time.Minute).Unix() || ticket.Task != c.Param("id") || ticket.Key != c.Param("key") {
		c.AbortWithStatus(403)
		return
	}
	credential, err := h.adminCredential(c, ticket.Admin)
	if err != nil || credential != ticket.Credential {
		c.AbortWithStatus(403)
		return
	}
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: ticket.Admin})
	c.Set(string(middleware.ContextKeyUserRole), "admin")
	c.Set("auth_method", "media_artifact_ticket")
	c.Next()
}
func (h *MediaHandler) AdminArtifactContent(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	t, err := h.Engine.Store.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		mediaError(c, err)
		return
	}
	h.serveArtifact(c, t)
}
func (h *MediaHandler) serveArtifact(c *gin.Context, t media.StoredTask) {
	if t.State != "completed" && !isMediaAdmin(c) {
		c.JSON(409, gin.H{"error": gin.H{"type": "artifact_not_ready", "message": "任务尚未完成"}})
		return
	}
	file, kind, hash, err := h.Engine.Store.ArtifactFile(c.Request.Context(), t.ID, c.Param("key"))
	if err != nil {
		mediaError(c, err)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		mediaError(c, err)
		return
	}
	c.Header("Content-Type", kind)
	c.Header("ETag", `"`+hash+`"`)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	if c.Query("download") == "1" {
		extensions, _ := mime.ExtensionsByType(kind)
		extension := ""
		if len(extensions) > 0 {
			extension = extensions[0]
		}
		name := h.Engine.Store.ArtifactFilename(c.Request.Context(), t.ID, c.Param("key"))
		if name == "" {
			name = "artifact" + extension
		}
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	}
	http.ServeContent(c.Writer, c.Request, "artifact", info.ModTime(), file)
}

func isMediaAdmin(c *gin.Context) bool {
	role, _ := middleware.GetUserRoleFromContext(c)
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	return ok && role == "admin" && subject.UserID > 0
}
