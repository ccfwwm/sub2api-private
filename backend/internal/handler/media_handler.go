package handler

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
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
	db      *sql.DB
	keys    *service.APIKeyService
	groups  mediaGroupAuthorizer
	Engine  *media.Engine
	gateway *service.OpenAIGatewayService
	openAI  *OpenAIGatewayHandler
	cache   *service.BillingCacheService
}

func NewMediaHandler(db *sql.DB, cfg *config.Config, accounts service.AccountRepository, keys *service.APIKeyService, cache *service.BillingCacheService, gateway *service.OpenAIGatewayService, openAI *OpenAIGatewayHandler, groups service.GroupRepository, channels *service.ChannelService, accountTests *service.AccountTestService, monitors *service.ChannelMonitorService) (*MediaHandler, error) {
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
		concurrency := a.Concurrency
		if concurrency < 1 {
			concurrency = 1
		}
		return media.Account{ID: id, Concurrency: concurrency, BaseURL: base, Key: key, Platform: a.Platform, Allowed: allowed && a.IsSchedulable(), CostMultiplier: strconv.FormatFloat(a.BillingRateMultiplier(), 'f', -1, 64)}, nil
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
	accountTests.SetMediaTemplateStore(engine.Store)
	monitors.SetMediaTemplateStore(engine.Store)
	engine.Config = service.UnifiedMediaConfig(accounts, groups, channels)
	engine.Store.RuntimeCapabilities = func(ctx context.Context, groupID int64) ([]media.Capability, error) {
		capabilities, _, err := engine.Config(ctx, groupID)
		return capabilities, err
	}
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
	if err = engine.Store.EnsureBuiltinTemplates(context.Background()); err != nil {
		return nil, err
	}
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
	return &MediaHandler{db: db, keys: keys, Engine: engine, gateway: gateway, openAI: openAI, cache: cache, groups: keys}, nil
}
func (h *MediaHandler) Stop() {
	if h != nil && h.Engine != nil {
		h.Engine.Stop()
	}
}
func mediaError(c *gin.Context, err error) {
	var problem *media.Error
	if errors.As(err, &problem) {
		if problem.RetryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(problem.RetryAfter))
		}
		if problem.Details != nil {
			setOpsMediaAdmission(c, problem.Details)
			if accountID, ok := problem.Details["account_id"].(int64); ok {
				setOpsSelectedAccount(c, accountID, service.PlatformImageVideo)
			}
		}
		errorBody := gin.H{"type": problem.Code, "code": problem.Code, "message": problem.Message}
		if problem.Details != nil {
			errorBody["details"] = problem.Details
		}
		c.JSON(problem.Status, gin.H{"error": errorBody})
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
	if mimeType == "application/x-www-form-urlencoded" {
		values, err := url.ParseQuery(string(raw))
		if err != nil {
			return nil, nil, err
		}
		for field, values := range values {
			for _, value := range values {
				if err := mediaFormField(p, field, value); err != nil {
					return nil, nil, err
				}
			}
		}
		return p, files, nil
	}
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
				if err := mediaFormField(p, field, string(data)); err != nil {
					return nil, nil, err
				}
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

func mediaFormField(p map[string]any, field, text string) error {
	field = strings.TrimSuffix(field, "[]")
	arrays := map[string]bool{"images": true, "videos": true, "audios": true, "reference_images": true, "reference_videos": true, "reference_audios": true, "reference_image_urls": true, "reference_video_urls": true, "reference_audio_urls": true}
	var value any = text
	if strings.HasPrefix(text, "[") || strings.HasPrefix(text, "{") || text == "true" || text == "false" {
		var parsed any
		dec := json.NewDecoder(strings.NewReader(text))
		dec.UseNumber()
		if dec.Decode(&parsed) == nil {
			value = parsed
		}
	}
	if arrays[field] {
		list, _ := p[field].([]any)
		if values, ok := value.([]any); ok {
			list = append(list, values...)
		} else {
			list = append(list, value)
		}
		p[field] = list
		return nil
	}
	if _, exists := p[field]; exists {
		return &media.Error{Status: 400, Code: "duplicate_field", Message: "Duplicate scalar field: " + field}
	}
	p[field] = value
	return nil
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
	setOpsRequestContext(c, cap.Model, false)
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
	setOpsSelectedAccount(c, task.AccountID, service.PlatformImageVideo)
	// Managed media operations always use the standard durable task protocol.
	// HTTP 202 means this service has persisted and reserved the task; only a
	// later status query can report whether the upstream accepted it.
	h.writeTask(c, task)
	return true
}
func (h *MediaHandler) writeTask(c *gin.Context, t media.StoredTask) {
	public, err := h.Engine.Store.PublicTask(c.Request.Context(), t)
	if err != nil {
		mediaError(c, err)
		return
	}
	public.Inputs = h.Engine.PublicInputs(t)
	code := 200
	if c.Request.Method == http.MethodPost {
		code = 202
	}
	c.JSON(code, public)
}
func (h *MediaHandler) SubmitSpeech(c *gin.Context) {
	if !h.TrySubmit(c, "speech") {
		c.JSON(422, gin.H{"error": gin.H{"type": "media_not_configured", "message": "Configure a priced speech model and template"}})
	}
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
	h.serveArtifact(c, t)
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
	req, err := h.Engine.PrepareRequest(c.Request.Context(), *cap, body.Operation, body.Request, nil)
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
