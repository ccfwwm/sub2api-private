package handler

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type MediaHandler struct {
	groups  mediaGroupAuthorizer
	Engine  *media.Engine
	gateway *service.OpenAIGatewayService
	openAI  *OpenAIGatewayHandler
	cache   *service.BillingCacheService
}

func NewMediaHandler(db *sql.DB, cfg *config.Config, accounts service.AccountRepository, keys *service.APIKeyService, cache *service.BillingCacheService, gateway *service.OpenAIGatewayService, openAI *OpenAIGatewayHandler, groups service.GroupRepository, channels *service.ChannelService) (*MediaHandler, error) {
	root := os.Getenv("SUB2API_MEDIA_DATA_DIR")
	if root == "" {
		root = filepath.Join("data", "media")
	}
	blobs, err := media.NewBlobStore(root, cfg.JWT.Secret)
	if err != nil {
		return nil, err
	}
	lookup := func(id, group int64) (media.Account, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		a, err := accounts.GetByID(ctx, id)
		if err != nil {
			return media.Account{}, err
		}
		allowed := false
		for _, g := range a.GroupIDs {
			if g == group {
				allowed = true
			}
		}
		key := a.GetCredential("api_key")
		base := a.GetCredential("base_url")
		if base == "" || key == "" {
			return media.Account{}, fmt.Errorf("media account requires base URL and API key")
		}
		return media.Account{ID: id, BaseURL: base, Key: key, Platform: a.Platform, Allowed: allowed && a.IsSchedulable(), CostMultiplier: strconv.FormatFloat(a.BillingRateMultiplier(), 'f', -1, 64)}, nil
	}
	invalidate := func(identity media.Identity) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		key, err := keys.GetByID(ctx, identity.KeyID)
		if err != nil {
			return err
		}
		keys.InvalidateAuthCacheByKey(ctx, key.Key)
		if err = cache.InvalidateUserBalance(ctx, identity.UserID); err != nil {
			return err
		}
		if err = cache.InvalidateAPIKeyRateLimit(ctx, identity.KeyID); err != nil {
			return err
		}
		if identity.SubscriptionID > 0 {
			return cache.InvalidateSubscription(ctx, identity.UserID, identity.GroupID)
		}
		return nil
	}
	engine := media.NewEngine(&media.Store{DB: db}, blobs, lookup, invalidate)
	engine.Config = service.UnifiedMediaConfig(accounts, groups, channels)
	var configured bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM accounts WHERE platform='imagevideo' AND deleted_at IS NULL)`).Scan(&configured); err != nil {
		return nil, err
	}
	if configured {
		engine.PlatformActive.Store(true)
	}
	engine.Store.LegacyBalanceHeld = func(ctx context.Context, user int64) (string, error) {
		held, err := cache.MediaLegacyBalanceHeld(ctx, user)
		return strconv.FormatFloat(held, 'f', -1, 64), err
	}
	cache.SetMediaBalanceGuard(func(ctx context.Context, user int64) (func(), *float64, error) {
		if !engine.PlatformActive.Load() {
			return func() {}, nil, nil
		}
		conn, err := db.Conn(ctx)
		if err != nil {
			return nil, nil, err
		}
		name := fmt.Sprintf("billing-user:%d", user)
		if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, name); err != nil {
			_ = conn.Close()
			return nil, nil, err
		}
		unlock := func() {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, unlockErr := conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, name)
			if unlockErr != nil {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
			_ = conn.Close()
		}
		var balance float64
		if err = conn.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, user).Scan(&balance); err != nil {
			unlock()
			return nil, nil, err
		}
		return unlock, &balance, nil
	})
	engine.Start()
	cache.SetMediaAdmissionCheck(func(ctx context.Context, user *service.User, key *service.APIKey, group *service.Group, sub *service.UserSubscription, platform string) error {
		if !engine.PlatformActive.Load() {
			return nil
		}
		var keyID, subscriptionID int64
		if key != nil {
			keyID = key.ID
		}
		if sub != nil {
			subscriptionID = sub.ID
		}
		scope, err := engine.Store.AdmissionQuota(ctx, keyID, subscriptionID)
		if err != nil {
			return service.ErrBillingServiceUnavailable
		}
		switch scope {
		case "key_total":
			return service.ErrAPIKeyQuotaExhausted
		case "key_5h":
			return service.ErrAPIKeyRateLimit5hExceeded
		case "key_1d":
			return service.ErrAPIKeyRateLimit1dExceeded
		case "key_7d":
			return service.ErrAPIKeyRateLimit7dExceeded
		case "subscription_daily":
			return service.ErrDailyLimitExceeded
		case "subscription_weekly":
			return service.ErrWeeklyLimitExceeded
		case "subscription_monthly":
			return service.ErrMonthlyLimitExceeded
		}
		if user == nil || platform == "" {
			return nil
		}
		quota, err := engine.Store.PlatformQuota(ctx, user.ID, platform)
		if err != nil {
			return service.ErrBillingServiceUnavailable
		}
		if quota == nil {
			return nil
		}
		hasMedia := false
		for _, v := range quota.Usage {
			if v != "0" && v != "0.00000000" {
				hasMedia = true
			}
		}
		if !hasMedia {
			return nil
		}
		baseline, err := cache.MediaPlatformBaseline(ctx, user.ID, platform)
		if err != nil {
			return service.ErrBillingServiceUnavailable
		}
		for _, part := range []string{"daily", "weekly", "monthly"} {
			if quota.Limits[part] == nil {
				continue
			}
			limit, err := decimal.NewFromString(*quota.Limits[part])
			if err != nil {
				return service.ErrBillingServiceUnavailable
			}
			usage, err := decimal.NewFromString(quota.Usage[part])
			if err != nil {
				return service.ErrBillingServiceUnavailable
			}
			if usage.Add(decimal.NewFromFloat(baseline[part])).GreaterThanOrEqual(limit) {
				switch part {
				case "daily":
					return service.ErrUserPlatformDailyQuotaExhausted
				case "weekly":
					return service.ErrUserPlatformWeeklyQuotaExhausted
				case "monthly":
					return service.ErrUserPlatformMonthlyQuotaExhausted
				}
			}
		}
		return nil
	})
	return &MediaHandler{Engine: engine, gateway: gateway, openAI: openAI, cache: cache, groups: keys}, nil
}
func (h *MediaHandler) Stop() {
	if h != nil && h.Engine != nil {
		h.Engine.Stop()
	}
}
func mediaError(c *gin.Context, err error) {
	var problem *media.Error
	if errors.As(err, &problem) {
		c.JSON(problem.Status, gin.H{"error": gin.H{"type": problem.Code, "message": problem.Message}})
		return
	}
	c.JSON(500, gin.H{"error": gin.H{"type": "media_service_error", "message": "Media operation failed; inspect the task or server diagnostics"}})
}
func (h *MediaHandler) identity(c *gin.Context) (media.Identity, error) {
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || key.Group == nil {
		return media.Identity{}, &media.Error{Status: 401, Code: "key_required", Message: "Assigned API key required"}
	}
	base := h.gateway.ResolveUserGroupRateMultiplier(c.Request.Context(), key.UserID, key.Group.ID, key.Group.RateMultiplier)
	identity := media.Identity{UserID: key.UserID, KeyID: key.ID, GroupID: key.Group.ID, Platform: key.Group.Platform, Multiplier: strconv.FormatFloat(base, 'f', -1, 64), TimeMultiplier: strconv.FormatFloat(key.Group.PeakMultiplierAt(time.Now()), 'f', -1, 64)}
	if key.Group.ImageRateIndependent {
		s := strconv.FormatFloat(key.Group.ImageRateMultiplier, 'f', -1, 64)
		identity.ImageOverride = &s
	}
	if key.Group.VideoRateIndependent {
		s := strconv.FormatFloat(key.Group.VideoRateMultiplier, 'f', -1, 64)
		identity.VideoOverride = &s
	}
	if subscription, ok := middleware.GetSubscriptionFromContext(c); ok && subscription != nil {
		identity.SubscriptionID = subscription.ID
	}
	baseline, err := h.cache.MediaPlatformBaseline(c.Request.Context(), key.UserID, key.Group.Platform)
	if err != nil {
		return identity, err
	}
	identity.PlatformBaseline = map[string]string{}
	for part, value := range baseline {
		identity.PlatformBaseline[part] = strconv.FormatFloat(value, 'f', -1, 64)
	}
	return identity, nil
}
func mediaPayload(c *gin.Context, operation string) (map[string]any, []media.Upload, error) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, (256<<20)+1))
	if err != nil {
		return nil, nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if len(raw) > 256<<20 {
		return nil, nil, &media.Error{Status: 413, Code: "body_too_large", Message: "Request exceeds media limit"}
	}
	p := map[string]any{}
	files := []media.Upload{}
	mimeType, params, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if mimeType == "multipart/form-data" {
		reader := multipart.NewReader(bytes.NewReader(raw), params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, nil, err
			}
			data, err := io.ReadAll(part)
			_ = part.Close()
			if err != nil {
				return nil, nil, err
			}
			field := part.FormName()
			if part.FileName() != "" {
				files = append(files, media.Upload{Field: field, Name: filepath.Base(part.FileName()), MIME: strings.Split(part.Header.Get("Content-Type"), ";")[0], Data: data})
			} else {
				if _, exists := p[field]; exists {
					return nil, nil, &media.Error{Status: 400, Code: "duplicate_field", Message: "Duplicate multipart field"}
				}
				text := string(data)
				if strings.HasPrefix(text, "[") || strings.HasPrefix(text, "{") || text == "true" || text == "false" {
					var object any
					if json.Unmarshal(data, &object) == nil {
						p[field] = object
						continue
					}
				}
				p[field] = text
			}
		}
	} else {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err = decoder.Decode(&p); err != nil {
			return nil, nil, err
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			return nil, nil, &media.Error{Status: 400, Code: "invalid_json", Message: "Single JSON object required"}
		}
	}
	if operation == "gemini_image" {
		action := strings.TrimPrefix(c.Param("modelAction"), "/")
		if !strings.HasSuffix(action, ":generateContent") {
			return p, files, nil
		}
		p["model"] = strings.TrimSuffix(action, ":generateContent")
	}
	return p, files, nil
}

// TrySubmit preserves all legacy gateway behavior for unconfigured models.
func (h *MediaHandler) TrySubmit(c *gin.Context, operation string) bool {
	if h == nil {
		return false
	}
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || key.Group == nil {
		return false
	}
	if h.Engine.Config != nil && key.Group.Platform != service.PlatformImageVideo && key.Group.Platform != service.PlatformComposite {
		return false
	}
	p, files, err := mediaPayload(c, operation)
	if err != nil {
		if key.Group.Platform == service.PlatformImageVideo {
			mediaError(c, &media.Error{Status: 400, Code: "invalid_media_request", Message: "Invalid media request body"})
			return true
		}
		return false
	}
	cap, err := h.Engine.SelectCapability(c.Request.Context(), fmt.Sprint(p["model"]), operation, p, files, media.Identity{GroupID: key.Group.ID})
	if err != nil {
		mediaError(c, err)
		return true
	}
	if cap == nil {
		managed, err := h.Engine.ManagedModel(c.Request.Context(), fmt.Sprint(p["model"]), operation, key.Group.ID)
		if err != nil {
			mediaError(c, err)
			return true
		}
		if managed || key.Group.Platform == service.PlatformImageVideo {
			mediaError(c, &media.Error{Status: 422, Code: "media_disabled", Message: "Media model is disabled for new tasks"})
			return true
		}
		return false
	}
	if key.Group.ModelAllowlistEnabled() && !key.Group.ModelAllowlist.Allows(cap.Model) {
		mediaError(c, &media.Error{Status: 403, Code: "model_not_allowed", Message: "Model is not allowed in this group"})
		return true
	}
	identity, err := h.identity(c)
	if err != nil {
		mediaError(c, err)
		return true
	}
	raw, _ := json.Marshal(p)
	if h.openAI != nil {
		key, _ := middleware.GetAPIKeyFromContext(c)
		subject, _ := middleware.GetAuthSubjectFromContext(c)
		protocol := service.ContentModerationProtocolOpenAIImages
		if operation == "gemini_image" {
			protocol = service.ContentModerationProtocolGemini
		}
		if operation == "chat_music" {
			protocol = service.ContentModerationProtocolOpenAIChat
		}
		decision := h.openAI.checkSecurityAudit(c, requestLogger(c, "handler.media.security_audit"), key, subject, protocol, cap.Model, raw)
		if decision != nil && !decision.AllowNextStage {
			h.openAI.openAISecurityAuditError(c, decision)
			return true
		}
	}
	size := int64(len(raw))
	for _, file := range files {
		size += int64(len(file.Data))
	}
	if cap.Limits.MaxRequestBytes == nil || size > *cap.Limits.MaxRequestBytes {
		mediaError(c, &media.Error{Status: 413, Code: "body_too_large", Message: "Request exceeds configured model limit"})
		return true
	}
	task, err := h.Engine.Create(c.Request.Context(), *cap, operation, p, files, identity, c.GetHeader("Idempotency-Key"))
	if err != nil {
		mediaError(c, err)
		return true
	}
	if operation == "video" || p["async"] == true {
		h.writeTask(c, task)
		return true
	}
	// Synchronous protocols wait on the same durable task; disconnecting does not cancel it.
	timer := time.NewTimer(175 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := h.Engine.Store.GetTask(c.Request.Context(), task.ID)
		if err != nil {
			mediaError(c, err)
			return true
		}
		if current.Settlement == "settled" || current.Settlement == "released" {
			h.writeSynchronous(c, current, operation, p)
			return true
		}
		if current.Settlement == "manual_review" {
			h.writeTask(c, current)
			return true
		}
		select {
		case <-c.Request.Context().Done():
			return true
		case <-timer.C:
			h.writeTask(c, current)
			return true
		case <-ticker.C:
		}
	}
}
func (h *MediaHandler) writeTask(c *gin.Context, t media.StoredTask) {
	public, err := h.Engine.Store.PublicTask(c.Request.Context(), t)
	if err != nil {
		mediaError(c, err)
		return
	}
	code := 200
	if c.Request.Method == http.MethodPost {
		code = 202
	}
	c.JSON(code, public)
}
func (h *MediaHandler) writeSynchronous(c *gin.Context, t media.StoredTask, operation string, p map[string]any) {
	if t.State == "failed" {
		c.JSON(502, gin.H{"id": t.ID, "error": gin.H{"type": "generation_failed", "message": t.Diagnostic}})
		return
	}
	artifacts, err := h.Engine.Store.Artifacts(c.Request.Context(), t.ID)
	if err != nil {
		mediaError(c, err)
		return
	}
	data := []gin.H{}
	for _, a := range artifacts {
		item := gin.H{"url": a.ContentURL}
		if a.Duration != nil {
			item["duration"] = a.Duration
		}
		if p["response_format"] == "b64_json" {
			file, _, _, err := h.Engine.Store.ArtifactFile(c.Request.Context(), t.ID, a.Key)
			if err != nil {
				mediaError(c, err)
				return
			}
			raw, err := io.ReadAll(io.LimitReader(file, (30<<20)+1))
			_ = file.Close()
			if err != nil || len(raw) > 30<<20 {
				mediaError(c, &media.Error{Status: 413, Code: "base64_result_limit", Message: "Use URL delivery for files larger than 30 MiB"})
				return
			}
			item = gin.H{"b64_json": base64.StdEncoding.EncodeToString(raw)}
		}
		data = append(data, item)
	}
	result := gin.H{"id": t.ID, "created": t.Created.Unix(), "data": data}
	if operation == "gemini_image" || operation == "chat_music" {
		var upstream map[string]any
		if err = h.Engine.Blobs.ReadJSON(t.ResultPath, &upstream); err != nil {
			mediaError(c, err)
			return
		}
		// Preserve protocol text, finish reasons and real usage. Only documented
		// response fields are exposed; internal task IDs and supplier metadata stay private.
		fields := []string{"candidates", "usageMetadata", "modelVersion", "promptFeedback", "responseId"}
		if operation == "chat_music" {
			fields = []string{"object", "model", "choices", "usage", "created", "system_fingerprint"}
		}
		for _, key := range fields {
			if value, ok := upstream[key]; ok {
				result[key] = value
			}
		}
		result["task_id"] = t.ID
		if operation == "gemini_image" {
			delete(result, "data")
			delete(result, "created")
			delete(result, "id")
		}
		replacements := map[string]string{}
		rows, dbErr := h.Engine.Store.DB.QueryContext(c.Request.Context(), `SELECT key,source_encrypted FROM media_artifacts WHERE task_id=$1`, t.ID)
		if dbErr != nil {
			mediaError(c, dbErr)
			return
		}
		for rows.Next() {
			var key string
			var encrypted []byte
			if err = rows.Scan(&key, &encrypted); err != nil {
				break
			}
			source, openErr := h.Engine.Blobs.Open(encrypted)
			if openErr != nil {
				err = openErr
				break
			}
			if !bytes.HasPrefix(source, []byte("data:")) {
				replacements[string(source)] = fmt.Sprintf("/v1/tasks/%s/artifacts/%s/content", t.ID, key)
			}
		}
		if err == nil {
			err = rows.Err()
		}
		_ = rows.Close()
		if err != nil {
			mediaError(c, err)
			return
		}
		result, _ = sanitizeMediaResult(result, replacements).(gin.H)
	}
	if p["stream"] == true {
		c.Header("Content-Type", "text/event-stream")
		c.Status(200)
		if operation == "chat_music" {
			chunk := gin.H{"id": t.ID, "object": "chat.completion.chunk", "model": t.Snapshot.Capability.Model, "created": t.Created.Unix()}
			choices := []gin.H{}
			if upstreamChoices, ok := result["choices"].([]any); ok {
				for _, value := range upstreamChoices {
					if choice, ok := value.(map[string]any); ok {
						choices = append(choices, gin.H{"index": choice["index"], "delta": choice["message"], "finish_reason": nil})
					}
				}
			}
			chunk["choices"] = choices
			raw, _ := json.Marshal(chunk)
			fmt.Fprintf(c.Writer, "data: %s\n\n", raw)
			for _, choice := range choices {
				choice["delta"] = gin.H{}
				choice["finish_reason"] = "stop"
			}
			chunk["choices"] = choices
			chunk["usage"] = result["usage"]
			raw, _ = json.Marshal(chunk)
			fmt.Fprintf(c.Writer, "data: %s\n\ndata: [DONE]\n\n", raw)
		} else {
			raw, _ := json.Marshal(result)
			fmt.Fprintf(c.Writer, "data: %s\n\ndata: [DONE]\n\n", raw)
		}
		return
	}

	c.JSON(200, result)
}
func (h *MediaHandler) SubmitAudio(c *gin.Context) {
	if !h.TrySubmit(c, "audio") {
		c.JSON(422, gin.H{"error": gin.H{"type": "media_not_configured", "message": "No verified, priced music model is configured for this key"}})
	}
}
func (h *MediaHandler) authorizedTask(c *gin.Context) (media.StoredTask, error) {
	id := c.Param("request_id")
	if id == "" {
		id = c.Param("id")
	}
	t, err := h.Engine.Store.GetTask(c.Request.Context(), id)
	if err != nil {
		return t, err
	}
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || t.Identity.UserID != key.UserID || t.Identity.KeyID != key.ID {
		return t, &media.Error{Status: 404, Code: "task_not_found", Message: "Task does not exist"}
	}
	return t, nil
}
func (h *MediaHandler) Get(c *gin.Context) {
	t, err := h.authorizedTask(c)
	if err != nil {
		mediaError(c, err)
		return
	}
	h.writeTask(c, t)
}
func (h *MediaHandler) Artifacts(c *gin.Context) {
	t, err := h.authorizedTask(c)
	if err != nil {
		mediaError(c, err)
		return
	}
	artifacts, err := h.Engine.Store.Artifacts(c.Request.Context(), t.ID)
	if err != nil {
		mediaError(c, err)
		return
	}
	c.JSON(200, gin.H{"task_id": t.ID, "artifacts": artifacts})
}
func (h *MediaHandler) Content(c *gin.Context) {
	t, err := h.authorizedTask(c)
	if err != nil {
		mediaError(c, err)
		return
	}
	if t.State != "completed" {
		c.JSON(409, gin.H{"error": gin.H{"type": "artifact_not_ready", "message": "Task has not completed"}})
		return
	}
	file, mime, hash, err := h.Engine.Store.ArtifactFile(c.Request.Context(), t.ID, c.Param("key"))
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
	c.Header("Content-Type", mime)
	c.Header("ETag", `"`+hash+`"`)
	c.Header("Cache-Control", "private, no-store")
	http.ServeContent(c.Writer, c.Request, "artifact", info.ModTime(), file)
}
func (h *MediaHandler) Estimate(c *gin.Context) {
	identity, err := h.identity(c)
	if err != nil {
		mediaError(c, err)
		return
	}
	var body struct {
		Operation string         `json:"operation"`
		Request   map[string]any `json:"request"`
	}
	if err = c.ShouldBindJSON(&body); err != nil {
		mediaError(c, &media.Error{Status: 400, Code: "invalid_estimate", Message: "Operation and request required"})
		return
	}
	cap, err := h.Engine.SelectCapability(c.Request.Context(), fmt.Sprint(body.Request["model"]), body.Operation, body.Request, nil, identity)
	if err != nil {
		mediaError(c, err)
		return
	}
	if cap == nil {
		mediaError(c, &media.Error{Status: 422, Code: "media_not_configured", Message: "No configured model"})
		return
	}
	key, _ := middleware.GetAPIKeyFromContext(c)
	if key.Group.ModelAllowlistEnabled() && !key.Group.ModelAllowlist.Allows(cap.Model) {
		mediaError(c, &media.Error{Status: 403, Code: "model_not_allowed", Message: "Model is not allowed in this group"})
		return
	}
	req, err := media.Normalize(*cap, body.Operation, body.Request)
	if err != nil {
		mediaError(c, err)
		return
	}
	snapshot, _, calc, err := h.Engine.Snapshot(c.Request.Context(), *cap, req, identity)
	if err != nil {
		mediaError(c, err)
		return
	}
	c.JSON(200, gin.H{"currency": "USD", "calculation": calc, "maximum_charge": snapshot.Maximum, "price_version": snapshot.Sale.Version, "locks_price": false})
}
func (h *MediaHandler) UserTasks(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		c.Status(401)
		return
	}
	h.listTasks(c, subject.UserID, false)
}

func sanitizeMediaResult(value any, replacements map[string]string) any {
	switch v := value.(type) {
	case string:
		for source, target := range replacements {
			v = strings.ReplaceAll(v, source, target)
		}
		return v
	case gin.H:
		clean := gin.H{}
		for key, item := range v {
			if !sensitiveMediaField(key) {
				clean[key] = sanitizeMediaResult(item, replacements)
			}
		}
		return clean
	case map[string]any:
		clean := map[string]any{}
		for key, item := range v {
			if !sensitiveMediaField(key) {
				clean[key] = sanitizeMediaResult(item, replacements)
			}
		}
		return clean
	case []any:
		for i, item := range v {
			v[i] = sanitizeMediaResult(item, replacements)
		}
		return v
	default:
		return value
	}
}
func sensitiveMediaField(key string) bool {
	switch strings.ToLower(key) {
	case "api_key", "access_token", "authorization", "credentials", "upstream_cost", "supplier_cost":
		return true
	}
	return false
}
