package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func (h *MediaHandler) AdminProtocolEffective(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	group, err := strconv.ParseInt(c.Query("group_id"), 10, 64)
	if err != nil || group < 1 {
		response.BadRequest(c, "A group ID is required")
		return
	}
	caps, _, err := h.Engine.Config(c.Request.Context(), group)
	if err == nil {
		caps, _, err = h.Engine.Store.ResolveTemplatesIsolated(c.Request.Context(), caps)
	}
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, media.SafeReviewValue(caps, nil, 0))
}

// AdminPricingStatus uses the same price resolver as submissions; media setup
// only displays the result and never writes a separate price card.
func (h *MediaHandler) AdminPricingStatus(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	group, err := strconv.ParseInt(c.Query("group_id"), 10, 64)
	if err != nil || group < 1 {
		response.BadRequest(c, "A group ID is required")
		return
	}
	account, err := strconv.ParseInt(c.Query("account_id"), 10, 64)
	if err != nil || account < 1 {
		response.BadRequest(c, "An account ID is required")
		return
	}
	caps, cards, err := h.Engine.Config(c.Request.Context(), group)
	if err != nil {
		mediaError(c, err)
		return
	}
	rows := []gin.H{}
	for _, cap := range caps {
		selected := false
		for _, id := range cap.AccountIDs {
			selected = selected || id == account
		}
		if !selected {
			continue
		}
		for _, operation := range cap.Operations {
			row := gin.H{"model": cap.Model, "operation": operation, "group_id": group, "matched": false, "reason": "price_not_configured"}
			price, resolveErr := media.ResolvePrice(cards, cap, media.Identity{GroupID: group}, operation, false, account, time.Now())
			if resolveErr == nil && len(price.Rules) > 0 {
				row["matched"], row["reason"] = true, ""
				row["price_id"], row["version"], row["rules"] = price.ID, price.Version, price.Rules
			} else if failure, ok := resolveErr.(*media.Error); ok {
				row["reason"] = failure.Code
			}
			rows = append(rows, row)
		}
	}
	response.Success(c, rows)
}

func protocolConfigKind(c *gin.Context) string {
	if strings.Contains(c.FullPath(), "/bindings") {
		return "binding"
	}
	return "template"
}
func (h *MediaHandler) AdminProtocolList(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	if err := h.Engine.Store.EnsureBuiltinTemplates(c.Request.Context()); err != nil {
		mediaError(c, err)
		return
	}
	rows, err := h.Engine.Store.Configs(c.Request.Context(), protocolConfigKind(c), false)
	if err != nil {
		mediaError(c, err)
		return
	}
	for i := range rows {
		if rows[i].PublishedVersion != nil {
			published, e := h.Engine.Store.ConfigVersion(c.Request.Context(), protocolConfigKind(c), rows[i].ID, *rows[i].PublishedVersion)
			if e != nil {
				mediaError(c, e)
				return
			}
			rows[i].PublishedConfig = published.Config
		}
		if protocolConfigKind(c) == "template" {
			for _, builtin := range media.BuiltinTemplates() {
				if rows[i].ID == builtin.ID {
					rows[i].BuiltinCurrent, _ = json.Marshal(builtin)
				}
			}
		}
	}
	response.Success(c, rows)
}
func (h *MediaHandler) AdminProtocolHistory(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	rows, err := h.Engine.Store.History(c.Request.Context(), protocolConfigKind(c), c.Param("id"))
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, rows)
}

type protocolSaveBody struct {
	ID              string          `json:"id"`
	Expected        *int64          `json:"expected_version"`
	Publish         bool            `json:"publish"`
	Config          json.RawMessage `json:"config"`
	UpgradeBindings []string        `json:"upgrade_bindings"`
	SourceVersion   *int64          `json:"source_version"`
}

func (h *MediaHandler) AdminProtocolSave(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		c.Status(401)
		return
	}
	var body protocolSaveBody
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "Invalid versioned configuration")
		return
	}
	kind := protocolConfigKind(c)
	var expected int64
	if body.Expected != nil {
		expected = *body.Expected
	}
	if strings.TrimSpace(body.ID) == "" || expected < 0 {
		response.BadRequest(c, "A configuration ID and a non-negative expected version are required")
		return
	}
	switch {
	case strings.HasSuffix(c.FullPath(), "/import"):
		if body.Expected == nil || expected != 0 || body.Publish || body.SourceVersion != nil || body.UpgradeBindings != nil {
			response.BadRequest(c, "Import requires a new configuration ID, expected_version 0 and an unpublished configuration")
			return
		}
		// Expected version zero is checked under the store's transaction lock:
		// importing an existing ID must never overwrite its draft or publication.
		body.Publish = false
	case strings.HasSuffix(c.FullPath(), "/publish"):
		if expected <= 0 || body.SourceVersion != nil {
			response.BadRequest(c, "Publish requires an existing expected version; use rollback for a historical source")
			return
		}
		body.Publish = true
		if len(body.Config) == 0 {
			current, err := h.Engine.Store.ConfigVersion(c.Request.Context(), kind, body.ID, expected)
			if err != nil {
				mediaError(c, err)
				return
			}
			body.Config = current.Config
		}
	case strings.HasSuffix(c.FullPath(), "/rollback"):
		if expected <= 0 || body.SourceVersion == nil || *body.SourceVersion <= 0 || len(body.Config) != 0 {
			response.BadRequest(c, "Rollback requires expected_version and source_version, without a configuration override")
			return
		}
		old, err := h.Engine.Store.ConfigVersion(c.Request.Context(), kind, body.ID, *body.SourceVersion)
		if err != nil {
			mediaError(c, err)
			return
		}
		body.Config = old.Config
		// Rollback publishes historical bytes as a new version rather than moving
		// the head backwards, preserving pinned versions and task snapshots.
		body.Publish = true
	default:
		if body.SourceVersion != nil {
			response.BadRequest(c, "Historical sources require the rollback endpoint")
			return
		}
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body.Config, &object) != nil || object == nil {
		response.BadRequest(c, "A configuration object is required")
		return
	}
	if kind == "binding" && body.Publish {
		var binding media.ModelBinding
		if json.Unmarshal(body.Config, &binding) != nil {
			response.BadRequest(c, "Invalid binding")
			return
		}
		if binding.AccountID > 0 {
			a, err := h.Engine.Lookup(binding.AccountID, binding.GroupID)
			if err != nil || !a.Allowed {
				response.BadRequest(c, "Account is not eligible for this group")
				return
			}
		}
	}
	row, err := h.Engine.Store.SaveProtocolConfig(c.Request.Context(), kind, body.ID, expected, subject.UserID, body.Config, body.Publish, body.UpgradeBindings)
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, row)
}
func (h *MediaHandler) AdminProtocolExport(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	rows, err := h.Engine.Store.History(c.Request.Context(), protocolConfigKind(c), c.Param("id"))
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, gin.H{"schema_version": 1, "kind": protocolConfigKind(c), "versions": rows})
}
func (h *MediaHandler) AdminProtocolArchive(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		c.Status(401)
		return
	}
	var body struct {
		ID       string `json:"id"`
		Expected int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "Invalid archive request")
		return
	}
	// Archiving advances the head; pinned versions and historical tasks survive.
	row, err := h.Engine.Store.ArchiveProtocol(c.Request.Context(), protocolConfigKind(c), body.ID, body.Expected, subject.UserID)
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, row)
}
func (h *MediaHandler) AdminProtocolPreview(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	var body struct {
		Mode          string                 `json:"mode"`
		KeyID         int64                  `json:"api_key_id"`
		GroupID       int64                  `json:"group_id"`
		AccountID     int64                  `json:"account_id"`
		Template      media.ProtocolTemplate `json:"template"`
		Operation     string                 `json:"operation"`
		Model         string                 `json:"model"`
		UpstreamModel string                 `json:"upstream_model"`
		Request       map[string]any         `json:"request"`
		Rules         []media.ParameterRule  `json:"rules"`
		Sample        map[string]any         `json:"sample_response"`
		Measure       bool                   `json:"measure_media"`
	}
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "Invalid preview input")
		return
	}
	if body.Mode == "effective" {
		if body.KeyID <= 0 || body.GroupID < 0 || body.AccountID < 0 || body.Request == nil || body.Operation == "" || body.Template.ID != "" || len(body.Rules) > 0 || body.UpstreamModel != "" {
			response.BadRequest(c, "Effective preview requires an existing API key and request; draft overrides are not accepted")
			return
		}
		h.adminEffectiveProtocolPreview(c, body.KeyID, body.GroupID, body.AccountID, body.Operation, body.Request, body.Measure)
		return
	}
	if body.Mode != "" && body.Mode != "draft" {
		response.BadRequest(c, "Preview mode must be draft or effective")
		return
	}
	if err := media.ValidateTemplate(body.Template, false); err != nil {
		mediaError(c, err)
		return
	}
	if body.Request == nil {
		response.BadRequest(c, "Preview requires a request object")
		return
	}
	if err := media.ValidateParameterRules(body.Rules); err != nil {
		mediaError(c, err)
		return
	}
	if body.Model == "" {
		body.Model = "preview"
	}
	if body.UpstreamModel == "" {
		body.UpstreamModel = body.Model
	}
	cap := media.Capability{Model: body.Model, UpstreamModel: body.UpstreamModel, Template: &body.Template, TemplateRules: body.Rules}
	if len(body.Template.MediaTypes) > 0 {
		cap.MediaType = body.Template.MediaTypes[0]
	}
	req, err := media.Normalize(cap, body.Operation, body.Request)
	if err != nil {
		mediaError(c, err)
		return
	}
	var wire media.PreparedMediaRequest
	if body.Measure {
		wire, err = media.BuildTemplateRequest(c.Request.Context(), h.Engine.Adapter.Client, cap, req)
	} else {
		wire, err = media.BuildTemplateRequest(c.Request.Context(), nil, cap, req)
	}
	if err != nil {
		mediaError(c, err)
		return
	}
	raw, _ := json.Marshal(wire.Body)
	receipt := media.NewSubmission(wire.Path, raw, wire.Body, req.MaterialRefs, "")
	result := gin.H{"normalized": media.SafeReviewValue(req.Parameters, nil, 0), "request": receipt, "quantities": req.Quantities, "online_media_get": body.Measure, "paid_submission": false}
	if body.Sample != nil {
		parsed := media.ParseTemplateResponse(body.Sample, cap)
		result["parsed_response"] = media.SafeReviewValue(gin.H{"task_id": parsed.ID, "status": parsed.Status, "artifacts": parsed.Artifacts, "error": parsed.Diagnostic}, nil, 0)
	}
	response.Success(c, result)
}
func (h *MediaHandler) AdminProtocolDiscover(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	var body struct {
		Template  media.ProtocolTemplate `json:"template"`
		AccountID int64                  `json:"account_id"`
		GroupID   int64                  `json:"group_id"`
	}
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "Invalid discovery input")
		return
	}
	if err := media.ValidateTemplate(body.Template, false); err != nil {
		mediaError(c, err)
		return
	}
	a, err := h.Engine.Lookup(body.AccountID, body.GroupID)
	if err != nil || !a.Allowed {
		response.BadRequest(c, "Account is not eligible")
		return
	}
	models, err := h.Engine.Adapter.DiscoverTemplate(c.Request.Context(), body.Template, a)
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, gin.H{"models": models, "enabled": false})
}
func (h *MediaHandler) AdminProtocolTest(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	var body struct {
		KeyID     int64          `json:"api_key_id"`
		Operation string         `json:"operation"`
		Request   map[string]any `json:"request"`
		Confirm   bool           `json:"confirm_paid_submission"`
	}
	if c.ShouldBindJSON(&body) != nil || body.Request == nil || !body.Confirm || body.KeyID <= 0 || c.GetHeader("Idempotency-Key") == "" {
		response.BadRequest(c, "Paid test requires explicit confirmation, an existing API key ID and an idempotency key")
		return
	}
	if h.keys == nil {
		response.Error(c, 503, "Media test identity is unavailable")
		return
	}
	key, err := h.keys.GetByID(c.Request.Context(), body.KeyID)
	if err != nil || !activeMediaAdminTestKey(key) {
		response.BadRequest(c, "An active assigned API key is required")
		return
	}
	if key.Group.IsSubscriptionType() {
		response.BadRequest(c, "Use the public authenticated API for subscription tests")
		return
	}
	c.Set(string(middleware.ContextKeyAPIKey), key)
	body.Request["async"] = true
	raw, _ := json.Marshal(body.Request)
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	if !h.TrySubmit(c, body.Operation) {
		response.BadRequest(c, "Publish a matching priced template binding before a paid test")
	}
}
