package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *MediaHandler) MarkBillingDelegate(c *gin.Context) {
	if h == nil || c.Request.Method != http.MethodPost {
		return
	}
	operation := ""
	path := c.Request.URL.Path
	switch {
	case strings.HasSuffix(path, "/images/generations"):
		operation = "image"
	case strings.HasSuffix(path, "/images/edits"):
		operation = "image_edit"
	case strings.HasSuffix(path, "/videos"):
		operation = "video"
	case strings.HasSuffix(path, "/chat/completions"):
		operation = "chat_music"
	case strings.HasPrefix(path, "/v1beta/models/") && strings.HasSuffix(path, ":generateContent"):
		operation = "gemini_image"
	case path == "/v1/audio/generations" || path == "/v1/music/generations":
		operation = "audio"
	}
	if operation == "" {
		return
	}
	p, _, err := mediaPayload(c, operation)
	if err != nil {
		return
	}
	model, _ := p["model"].(string)
	if operation == "gemini_image" {
		model = strings.TrimSuffix(strings.TrimPrefix(path, "/v1beta/models/"), ":generateContent")
	}
	c.Set("media_billing_delegate", func(key *service.APIKey) bool {
		if key == nil || key.Group == nil {
			return false
		}
		cap, err := h.Engine.Capability(c.Request.Context(), model, operation, media.Identity{GroupID: key.Group.ID})
		return err == nil && cap != nil
	})
}
func (h *MediaHandler) Models(c *gin.Context, next gin.HandlerFunc) {
	if h == nil {
		next(c)
		return
	}
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || key.Group == nil {
		next(c)
		return
	}
	catalog, err := h.Engine.Catalog(c.Request.Context(), key.Group.ID)
	if err != nil {
		mediaError(c, err)
		return
	}
	models := []string{}
	managed, err := h.Engine.ManagedModels(c.Request.Context(), key.Group.ID, "")
	if err != nil {
		mediaError(c, err)
		return
	}
	for _, entry := range catalog {
		if !key.Group.ModelAllowlistEnabled() || key.Group.ModelAllowlist.Allows(entry.Model) {
			models = append(models, entry.Model)
		}
	}
	if len(models) == 0 && len(managed) == 0 {
		next(c)
		return
	}
	original := c.Writer
	captured := &modelCapture{ResponseWriter: original, code: http.StatusOK}
	c.Writer = captured
	next(c)
	c.Writer = original
	var body map[string]any
	if captured.code != 200 || json.Unmarshal(captured.body.Bytes(), &body) != nil {
		c.Status(captured.code)
		_, _ = c.Writer.Write(captured.body.Bytes())
		return
	}
	list, _ := body["data"].([]any)
	available := map[string]bool{}
	for _, model := range models {
		available[model] = true
	}
	filtered := []any{}
	for _, item := range list {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := object["id"].(string)
		if !managed[id] || available[id] {
			filtered = append(filtered, item)
		}
	}
	list = filtered
	seen := map[string]bool{}
	for _, item := range list {
		if obj, ok := item.(map[string]any); ok {
			id, _ := obj["id"].(string)
			seen[id] = true
		}
	}
	for _, model := range models {
		if !seen[model] {
			seen[model] = true
			list = append(list, gin.H{"id": model, "object": "model", "created": time.Now().Unix(), "owned_by": "sub2api"})
		}
	}
	body["data"] = list
	c.JSON(200, body)
}

type modelCapture struct {
	gin.ResponseWriter
	body bytes.Buffer
	code int
}

func (w *modelCapture) WriteHeader(code int)                  { w.code = code }
func (w *modelCapture) WriteHeaderNow()                       {}
func (w *modelCapture) Write(raw []byte) (int, error)         { return w.body.Write(raw) }
func (w *modelCapture) WriteString(value string) (int, error) { return w.body.WriteString(value) }
func (w *modelCapture) Status() int                           { return w.code }
func (w *modelCapture) Size() int                             { return w.body.Len() }
func (w *modelCapture) Written() bool                         { return w.body.Len() > 0 }
