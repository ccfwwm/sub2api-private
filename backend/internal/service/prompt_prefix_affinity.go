package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const prefixAffinityTTL = 30 * time.Minute

// PrefixAffinityRule scopes each experiment to a single request and upstream model.
type PrefixAffinityRule struct {
	GroupID          int64  `json:"group_id"`
	Platform         string `json:"platform"`
	Endpoint         string `json:"endpoint"`
	Model            string `json:"model"`
	UpstreamModel    string `json:"upstream_model"`
	UpstreamEndpoint string `json:"upstream_endpoint"`
	Mode             string `json:"mode"`
	Percent          int    `json:"percent"`
	PromptKey        bool   `json:"prompt_key"`
	Route            bool   `json:"route"`
}

var prefixAffinityRules struct {
	sync.Mutex
	refreshAt time.Time
	rules     []PrefixAffinityRule
}

func loadPrefixAffinityRules() []PrefixAffinityRule {
	path := os.Getenv("SUB2API_PREFIX_AFFINITY_FILE")
	if path == "" {
		return nil
	}
	prefixAffinityRules.Lock()
	defer prefixAffinityRules.Unlock()
	now := time.Now()
	if now.Before(prefixAffinityRules.refreshAt) {
		return prefixAffinityRules.rules
	}
	prefixAffinityRules.refreshAt = now.Add(2 * time.Second)
	raw, err := os.ReadFile(path)
	var rules []PrefixAffinityRule
	if err != nil || json.Unmarshal(raw, &rules) != nil {
		prefixAffinityRules.rules = nil
		return nil
	}
	prefixAffinityRules.rules = rules
	return rules
}

type prefixAffinityContextKey struct{}

type PrefixAffinity struct {
	rule      PrefixAffinityRule
	key       string
	bucket    uint32
	preferred int64
	replace   int64
}

func prefixAffinityInContext(ctx context.Context) *PrefixAffinity {
	value, _ := ctx.Value(prefixAffinityContextKey{}).(*PrefixAffinity)
	return value
}

func prefixAffinitySeed(body []byte, endpoint string) string {
	view := gjson.ParseBytes(body)
	if !view.Get("messages").IsArray() || view.Get("previous_response_id").Exists() || view.Get("cachedContent").Exists() {
		return ""
	}
	var parts []string
	add := func(name string, field gjson.Result) {
		if normalized, ok := normalizeNonEmptyCompatSeedJSON(field); ok {
			parts = append(parts, name+"="+normalized)
		}
	}
	add("tools", view.Get("tools"))
	if endpoint == "/v1/messages" {
		add("system", view.Get("system"))
		add("cache_control", view.Get("cache_control"))
	} else {
		add("functions", view.Get("functions"))
		view.Get("messages").ForEach(func(_, message gjson.Result) bool {
			role := message.Get("role").String()
			if role != "system" && role != "developer" {
				return false
			}
			add(role, message.Get("content"))
			return true
		})
	}
	if len(parts) == 0 {
		return ""
	}
	encoded, _ := json.Marshal(parts)
	return string(encoded)
}

// WithPrefixAffinity installs a request-local experiment after authentication and model resolution.
func WithPrefixAffinity(ctx context.Context, groupID, apiKeyID int64, platform, endpoint, model string, body []byte, explicit bool) context.Context {
	if explicit || groupID <= 0 || apiKeyID <= 0 {
		return ctx
	}
	if platform != PlatformOpenAI && platform != PlatformDeepseek && platform != PlatformAnthropic {
		return ctx
	}
	for _, rule := range loadPrefixAffinityRules() {
		if rule.GroupID != groupID || rule.Platform != platform || rule.Endpoint != endpoint || rule.Model != model || rule.UpstreamModel == "" || !validPrefixAffinityPath(rule) {
			continue
		}
		if (rule.Mode != "shadow" && rule.Mode != "enforce") || rule.Percent < 1 || rule.Percent > 100 || !rule.Route && !rule.PromptKey {
			return ctx
		}
		seed := prefixAffinitySeed(body, endpoint)
		if seed == "" {
			return ctx
		}
		scope, _ := json.Marshal([]string{strconv.FormatInt(groupID, 10), strconv.FormatInt(apiKeyID, 10), platform, endpoint, rule.UpstreamEndpoint, rule.UpstreamModel, seed})
		sum := sha256.Sum256(scope)
		bucket := binary.BigEndian.Uint32(sum[:4]) % 100
		if rule.Mode == "enforce" && bucket >= uint32(rule.Percent) {
			return ctx
		}
		return context.WithValue(ctx, prefixAffinityContextKey{}, &PrefixAffinity{
			rule: rule, key: "prompt-prefix:v1:" + hex.EncodeToString(sum[:]), bucket: bucket,
		})
	}
	return ctx
}

func validPrefixAffinityPath(rule PrefixAffinityRule) bool {
	switch rule.Platform {
	case PlatformOpenAI:
		return rule.Endpoint == "/v1/chat/completions" && rule.UpstreamEndpoint == "/v1/responses"
	case PlatformDeepseek:
		return rule.Endpoint == "/v1/chat/completions" && rule.UpstreamEndpoint == "/v1/chat/completions" && !rule.PromptKey
	case PlatformAnthropic:
		return rule.Endpoint == "/v1/messages" && rule.UpstreamEndpoint == "/v1/messages" && !rule.PromptKey
	}
	return false
}

func (p *PrefixAffinity) usable(groupID *int64, platform, model string) bool {
	return p != nil && p.rule.GroupID == derefGroupID(groupID) && p.rule.Platform == platform && p.rule.Model == model
}

func (p *PrefixAffinity) read(ctx context.Context, cache GatewayCache, groupID *int64, platform, model string, lookup func(context.Context, int64) (*Account, error), matchesUpstream func(*Account, PrefixAffinityRule) bool) int64 {
	if !p.usable(groupID, platform, model) || !p.rule.Route || cache == nil {
		return 0
	}
	if p.rule.Mode == "shadow" {
		// Shadow traffic must never create or update production affinity.
		return 0
	}
	id, err := cache.GetSessionAccountID(ctx, p.rule.GroupID, p.key)
	if err != nil || id <= 0 {
		return 0
	}
	p.preferred = id
	a, err := lookup(ctx, id)
	if errors.Is(err, ErrAccountNotFound) || err == nil && a == nil {
		p.replace = id
		return 0
	}
	if err != nil {
		return 0
	}
	if a.Platform != platform || a.Status != StatusActive || !a.Schedulable ||
		!containsPrefixGroup(a.GroupIDs, p.rule.GroupID) || !matchesUpstream(a, p.rule) {
		p.replace = id
		return 0
	}
	return id
}

func containsPrefixGroup(groups []int64, groupID int64) bool {
	for _, id := range groups {
		if id == groupID {
			return true
		}
	}
	return false
}

type prefixAffinityWriter interface {
	CompareAndSetPrefixAffinity(ctx context.Context, groupID int64, key string, previous, accountID int64, ttl time.Duration) error
}

func recordPrefixAffinity(ctx context.Context, cache GatewayCache, groupID *int64, account *Account, actualModel, actualEndpoint string, reads, writes int) {
	p := prefixAffinityInContext(ctx)
	if p == nil || account == nil || p.rule.GroupID != derefGroupID(groupID) || p.rule.Platform != account.Platform || actualModel != p.rule.UpstreamModel || actualEndpoint != p.rule.UpstreamEndpoint {
		return
	}
	if p.bucket < 5 {
		slog.Info("prompt_prefix_affinity", "group_id", p.rule.GroupID, "platform", p.rule.Platform,
			"endpoint", p.rule.Endpoint, "model", actualModel, "mode", p.rule.Mode, "prefix", p.key,
			"preferred_account_id", p.preferred, "account_id", account.ID, "cache_read_tokens", reads, "cache_creation_tokens", writes)
	}
	if !p.rule.Route || p.rule.Mode != "enforce" || p.preferred != 0 && p.preferred != account.ID && p.replace == 0 {
		return
	}
	writer, ok := cache.(prefixAffinityWriter)
	if !ok {
		return
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 100*time.Millisecond)
	defer cancel()
	_ = writer.CompareAndSetPrefixAffinity(writeCtx, p.rule.GroupID, p.key, p.replace, account.ID, prefixAffinityTTL)
}

func prefixAffinityPromptKey(ctx context.Context, account *Account, upstreamModel, fallback string) string {
	p := prefixAffinityInContext(ctx)
	if p == nil || p.rule.Platform != PlatformOpenAI || account.Platform != PlatformOpenAI ||
		p.rule.UpstreamModel != upstreamModel || !p.rule.PromptKey {
		return fallback
	}
	key := strings.TrimPrefix(p.key, "prompt-prefix:v1:")
	if p.rule.Mode != "enforce" {
		if p.bucket < 5 {
			slog.Info("prompt_prefix_cache_key_shadow", "group_id", p.rule.GroupID, "model", upstreamModel,
				"prefix", key, "current_key_hash", hashSensitiveValueForLog(fallback))
		}
		return fallback
	}
	return "compat_prefix_" + key
}

func (s *OpenAIGatewayService) RecordChatPrefixAffinity(ctx context.Context, groupID *int64, account *Account, result *OpenAIForwardResult, endpoint string) {
	if result != nil {
		model := result.UpstreamModel
		if model == "" {
			model = result.Model
		}
		recordPrefixAffinity(ctx, s.cache, groupID, account, model, endpoint, result.Usage.CacheReadInputTokens, result.Usage.CacheCreationInputTokens)
	}
}

func (s *GatewayService) RecordMessagesPrefixAffinity(ctx context.Context, groupID *int64, account *Account, result *ForwardResult) {
	if result != nil {
		model := result.UpstreamModel
		if model == "" {
			model = result.Model
		}
		recordPrefixAffinity(ctx, s.cache, groupID, account, model, "/v1/messages", result.Usage.CacheReadInputTokens, result.Usage.CacheCreationInputTokens)
	}
}

func (s *GatewayService) ResolveNativeMessagesSessionHash(ctx context.Context, c *gin.Context, existing string, apiKey *APIKey, model string) string {
	if s.cache == nil || apiKey.GroupID == nil {
		return existing
	}
	session := ClaudeCodeSessionIDFromHeader(c)
	if session == "" {
		return existing
	}
	for _, rule := range loadPrefixAffinityRules() {
		if rule.GroupID != *apiKey.GroupID || rule.Platform != PlatformAnthropic || rule.Endpoint != "/v1/messages" ||
			rule.Model != model || !validPrefixAffinityPath(rule) || rule.Mode != "enforce" || !rule.Route || rule.Percent < 1 || rule.Percent > 100 {
			continue
		}
		scope, _ := json.Marshal([]string{strconv.FormatInt(*apiKey.GroupID, 10), strconv.FormatInt(apiKey.ID, 10), session})
		digest := sha256.Sum256(scope)
		if binary.BigEndian.Uint32(digest[:4])%100 >= uint32(rule.Percent) {
			return existing
		}
		if existing != "" {
			account, err := s.cache.GetSessionAccountID(ctx, *apiKey.GroupID, existing)
			if account > 0 || err != nil && !errors.Is(err, ErrStickySessionNotFound) {
				return existing
			}
		}
		return "claude-code-session:v1:" + hex.EncodeToString(digest[:])
	}
	return existing
}
