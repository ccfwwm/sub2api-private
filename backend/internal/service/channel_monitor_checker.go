package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/tidwall/gjson"
)

// monitorHTTPClient 共享一个 http.Client，避免每次检测重建 transport。
// 自定义 Transport 在 dial 时强制再次校验 IP，防止 DNS rebinding 绕过 validateEndpoint。
var monitorHTTPClient = newSSRFSafeHTTPClient(monitorRequestTimeout)

// monitorPingHTTPClient 用于 endpoint origin 的 HEAD ping，超时更短。
var monitorPingHTTPClient = newSSRFSafeHTTPClient(monitorPingTimeout)

var monitorImageHTTPClient = newSSRFSafeHTTPClient(monitorImageRequestTimeout)

// newSSRFSafeHTTPClient 返回一个使用 safeDialContext 的 http.Client。
// 仅供监控模块对外发起请求使用——所有目标都应是公网 endpoint。
func newSSRFSafeHTTPClient(timeout time.Duration) *http.Client {
	headerTimeout := monitorResponseHeaderTimeout
	if timeout > monitorRequestTimeout {
		headerTimeout = timeout
	}
	tr := &http.Transport{
		DialContext:           safeDialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       monitorIdleConnTimeout,
		TLSHandshakeTimeout:   monitorTLSHandshakeTimeout,
		ResponseHeaderTimeout: headerTimeout,
	}
	return &http.Client{Timeout: timeout, Transport: servertiming.WrapRoundTripper(tr), CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
}

// CheckOptions 承载一次检测的自定义入参。
type CheckOptions struct {
	// APIMode 仅对 OpenAI provider 生效；空串等同 chat_completions。
	APIMode string
	// ExtraHeaders 用户自定义 HTTP 头（merge 到 adapter 默认 headers，用户优先）。
	ExtraHeaders map[string]string
	// BodyOverrideMode: off | merge | replace
	BodyOverrideMode string
	// BodyOverride 在 merge 模式下做浅合并（key 命中黑名单时静默丢弃），
	// 在 replace 模式下直接当作完整 body。
	BodyOverride                  map[string]any
	ProbeType                     string
	CustomMethod                  string
	CustomPath                    string
	CustomBody                    map[string]any
	CustomResponseJSONPath        string
	CustomResponseRequireNonEmpty bool
}

// runCheckForModel 对单个 (provider, model) 做一次完整检测。
// 不返回 error：所有失败都包装进 CheckResult.Status=error/failed。
//
// opts 承载模板 / 监控快照带来的自定义配置。nil 等同于 "off + 无 extra headers"。
func runCheckForModel(ctx context.Context, provider, endpoint, apiKey, model string, opts *CheckOptions) *CheckResult {
	res := &CheckResult{
		Model:     model,
		Status:    MonitorStatusError,
		CheckedAt: time.Now(),
	}

	challenge := generateChallenge()
	mode := bodyOverrideMode(opts)
	probeType := monitorProbeType(opts)

	start := time.Now()
	respText, rawBody, statusCode, err := callProvider(ctx, provider, endpoint, apiKey, model, probePrompt(probeType, challenge.Prompt), opts)
	latency := time.Since(start)
	latencyMs := int(latency / time.Millisecond)
	res.LatencyMs = &latencyMs

	if err != nil {
		res.Status = MonitorStatusError
		res.Message = truncateMessage(sanitizeErrorMessage(err.Error()))
		return res
	}
	if statusCode < 200 || statusCode >= 300 {
		// 错误路径：用 rawBody 而非 respText（gjson textPath 抽取在错误响应里通常为空，
		// 会丢掉真正的上游错误信息，例如 `{"error":{"message":"No available accounts ..."}}`）。
		res.Status = MonitorStatusError
		bodySnippet := truncateForErrorBody(rawBody)
		res.Message = truncateMessage(sanitizeErrorMessage(fmt.Sprintf("upstream HTTP %d: %s", statusCode, bodySnippet)))
		return res
	}
	if probeType == MonitorProbeTypeImage || probeType == MonitorProbeTypeCustom {
		if probeType == MonitorProbeTypeImage && strings.TrimSpace(respText) == "" {
			res.Status = MonitorStatusFailed
			res.Message = truncateMessage("image probe: upstream returned 2xx without an image")
			return res
		}
		if probeType == MonitorProbeTypeCustom && customResponseMustBeNonEmpty(opts) && strings.TrimSpace(respText) == "" {
			res.Status = MonitorStatusFailed
			res.Message = truncateMessage("custom probe: upstream returned 2xx with an empty extracted response")
			return res
		}
		return finalizeOperationalOrDegraded(res, latency, latencyMs)
	}

	// Replace 模式：跳过 challenge 校验（用户 body 是静态的，challenge 没法嵌入）。
	// 改用「HTTP 2xx + 响应文本（adapter.textPath 抽取）非空」作为 operational 判定。
	// 响应文本为空则降级为 failed（视为上游回了 200 但没实际内容）。
	if mode == MonitorBodyOverrideModeReplace {
		if strings.TrimSpace(respText) == "" {
			res.Status = MonitorStatusFailed
			res.Message = truncateMessage("replace-mode: upstream returned 2xx with empty text")
			return res
		}
		return finalizeOperationalOrDegraded(res, latency, latencyMs)
	}

	if !validateChallenge(respText, challenge.Expected) {
		res.Status = MonitorStatusFailed
		res.Message = truncateMessage(sanitizeErrorMessage(fmt.Sprintf("challenge mismatch (expected %s, got %q)", challenge.Expected, respText)))
		return res
	}

	return finalizeOperationalOrDegraded(res, latency, latencyMs)
}

func probePrompt(probeType, textPrompt string) string {
	if probeType == MonitorProbeTypeImage {
		return "Generate a simple blue circle on a white background."
	}
	return textPrompt
}

func monitorProbeType(opts *CheckOptions) string {
	if opts == nil {
		return MonitorProbeTypeText
	}
	return defaultMonitorProbeType(opts.ProbeType)
}

func customResponseMustBeNonEmpty(opts *CheckOptions) bool {
	return opts == nil || opts.CustomResponseRequireNonEmpty
}

// finalizeOperationalOrDegraded 负责走到最后一步的 operational/degraded 判定。
// 拆出来是为了让 runCheckForModel 不超过 30 行。
func finalizeOperationalOrDegraded(res *CheckResult, latency time.Duration, latencyMs int) *CheckResult {
	if latency >= monitorDegradedThreshold {
		res.Status = MonitorStatusDegraded
		res.Message = truncateMessage(fmt.Sprintf("slow response: %dms", latencyMs))
		return res
	}
	res.Status = MonitorStatusOperational
	return res
}

// bodyOverrideMode 归一取 opts.BodyOverrideMode，nil opts / 空串都视为 off。
func bodyOverrideMode(opts *CheckOptions) string {
	if opts == nil || opts.BodyOverrideMode == "" {
		return MonitorBodyOverrideModeOff
	}
	return opts.BodyOverrideMode
}

// pingEndpointOrigin 对 endpoint 的 origin (scheme://host) 发起 HEAD 请求，返回耗时。
// 失败时返回 nil（不影响主状态判定）。
func pingEndpointOrigin(ctx context.Context, endpoint string) *int {
	origin, err := extractOrigin(endpoint)
	if err != nil || origin == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, origin, nil)
	if err != nil {
		return nil
	}
	start := time.Now()
	resp, err := monitorPingHTTPClient.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, monitorPingDiscardMaxBytes))
	ms := int(time.Since(start) / time.Millisecond)
	return &ms
}

// providerAdapter 描述某个 provider 在 challenge 检测中需要的几件事：
//   - 拼出请求路径（含 model 占位）
//   - 序列化请求体
//   - 构造鉴权头
//   - 从响应 JSON 中提取文本（默认按 gjson path；需要时可自定义）
//
// 加新 provider 只需要在 providerAdapters 里增加一个条目，无需触碰 callProvider / validateProvider。
type providerAdapter struct {
	buildPath    func(model string) string
	buildBody    func(model, prompt string) ([]byte, error)
	buildHeaders func(apiKey string) map[string]string
	textPath     string // gjson 提取响应文本的 path
	extractText  func([]byte) string
}

// providerAdapters 全部已支持的 provider。键值即 MonitorProvider* 字符串。
//
//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerAdapters = map[string]providerAdapter{
	MonitorProviderOpenAI: providerOpenAIChatAdapter,
	MonitorProviderGrok:   providerGrokChatAdapter,
	// 国产 3 家（配额模式引入）：均为 OpenAI 兼容 Chat Completions，
	// 仅智谱路径前缀不同（/api/paas/v4/chat/completions）。
	MonitorProviderKimi:     providerKimiChatAdapter,
	MonitorProviderZhipu:    providerZhipuChatAdapter,
	MonitorProviderDeepseek: providerDeepseekChatAdapter,
	MonitorProviderMiniMax:  providerMiniMaxChatAdapter,
	MonitorProviderAnthropic: {
		buildPath: func(string) string { return providerAnthropicPath },
		buildBody: func(model, prompt string) ([]byte, error) {
			return json.Marshal(map[string]any{
				"model":      model,
				"messages":   []map[string]string{{"role": "user", "content": prompt}},
				"max_tokens": monitorChallengeMaxTokens,
			})
		},
		buildHeaders: func(apiKey string) map[string]string {
			return map[string]string{
				"x-api-key":         apiKey,
				"anthropic-version": monitorAnthropicAPIVersion,
			}
		},
		extractText: extractAnthropicMonitorText,
	},
	MonitorProviderGemini: {
		// Gemini 把 model 名写在 URL path 上：/v1beta/models/{model}:generateContent
		buildPath: func(model string) string { return fmt.Sprintf(providerGeminiPathTemplate, model) },
		buildBody: func(_, prompt string) ([]byte, error) {
			return json.Marshal(map[string]any{
				"contents": []map[string]any{
					{"role": "user", "parts": []map[string]any{{"text": prompt}}},
				},
				"generationConfig": map[string]any{"maxOutputTokens": monitorChallengeMaxTokens},
			})
		},
		// 使用 x-goog-api-key header 而不是 ?key= query，避免 *url.Error 把 key 回填到错误日志。
		buildHeaders: func(apiKey string) map[string]string {
			return map[string]string{"x-goog-api-key": apiKey}
		},
		textPath: "candidates.0.content.parts.0.text",
	},
}

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerOpenAIChatAdapter = newOpenAICompatibleChatAdapter(providerOpenAIPath)

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerGrokChatAdapter = newOpenAICompatibleChatAdapter(providerGrokPath)

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerKimiChatAdapter = newOpenAICompatibleChatAdapter(providerOpenAIPath)

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerZhipuChatAdapter = newOpenAICompatibleChatAdapter(providerZhipuPath)

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerDeepseekChatAdapter = newOpenAICompatibleChatAdapter(providerOpenAIPath)

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerMiniMaxChatAdapter = newOpenAICompatibleChatAdapter(providerOpenAIPath)

func newOpenAICompatibleChatAdapter(path string) providerAdapter {
	return providerAdapter{
		buildPath: func(string) string { return path },
		buildBody: func(model, prompt string) ([]byte, error) {
			return json.Marshal(map[string]any{
				"model":      model,
				"messages":   []map[string]string{{"role": "user", "content": prompt}},
				"max_tokens": monitorChallengeMaxTokens,
				"stream":     false,
			})
		},
		buildHeaders: func(apiKey string) map[string]string {
			return map[string]string{"Authorization": "Bearer " + apiKey}
		},
		textPath: "choices.0.message.content",
	}
}

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerOpenAIResponsesAdapter = providerAdapter{
	buildPath: func(string) string { return providerOpenAIResponsesPath },
	buildBody: func(model, prompt string) ([]byte, error) {
		return json.Marshal(map[string]any{
			"model":             model,
			"instructions":      "You are a channel health-check endpoint. Answer the arithmetic challenge exactly and briefly.",
			"input":             prompt,
			"max_output_tokens": monitorChallengeMaxTokens,
			"stream":            false,
		})
	},
	buildHeaders: func(apiKey string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + apiKey}
	},
	textPath: "output.0.content.0.text",
}

// providerAdapterFor 按 provider + api_mode 选择具体 adapter。
func providerAdapterFor(provider, apiMode string) (providerAdapter, string, bool) {
	if provider == MonitorProviderOpenAI && defaultAPIMode(apiMode) == MonitorAPIModeResponses {
		return providerOpenAIResponsesAdapter, MonitorAPIModeResponses, true
	}
	adapter, ok := providerAdapters[provider]
	return adapter, MonitorAPIModeChatCompletions, ok
}

// callProvider 通过 providerAdapters 分发到具体实现。
// opts 承载用户的自定义 headers / body 覆盖（可为 nil）。
//
// 返回值：
//   - extractedText: 按 textPath 抽出的成功文本，仅在 status 2xx 时有意义；非 2xx 时通常为空串
//   - rawBody: 完整响应体的字符串形式（已被 monitorResponseMaxBytes 截断），用于错误路径保留上游真实回包
//   - status: HTTP 状态码
//   - err: 网络 / 序列化错误
func callProvider(ctx context.Context, provider, endpoint, apiKey, model, prompt string, opts *CheckOptions) (extractedText, rawBody string, status int, err error) {
	switch monitorProbeType(opts) {
	case MonitorProbeTypeImage:
		return callImageProvider(ctx, provider, endpoint, apiKey, model, prompt, opts)
	case MonitorProbeTypeCustom:
		return callCustomProvider(ctx, endpoint, apiKey, opts)
	case MonitorProbeTypeText:
	default:
		return "", "", 0, fmt.Errorf("unsupported probe type %q", monitorProbeType(opts))
	}
	requestedAPIMode := checkAPIMode(opts)
	if err := validateAPIMode(provider, requestedAPIMode); err != nil {
		return "", "", 0, err
	}
	adapter, apiMode, ok := providerAdapterFor(provider, requestedAPIMode)
	if !ok {
		return "", "", 0, fmt.Errorf("unsupported provider %q", provider)
	}
	body, err := buildRequestBody(adapter, provider, apiMode, model, prompt, opts)
	if err != nil {
		return "", "", 0, err
	}
	headers := mergeHeaders(adapter.buildHeaders(apiKey), opts)
	full := joinURL(endpoint, monitorRequestPath(provider, endpoint, adapter, model))
	respBytes, status, err := doMonitorRequest(ctx, http.MethodPost, full, body, headers)
	if err != nil {
		return "", "", status, err
	}
	if provider == MonitorProviderOpenAI && apiMode == MonitorAPIModeResponses {
		return extractOpenAIResponsesText(respBytes), string(respBytes), status, nil
	}
	return extractMonitorResponseText(adapter, respBytes), string(respBytes), status, nil
}

func callImageProvider(ctx context.Context, provider, endpoint, apiKey, model, prompt string, opts *CheckOptions) (string, string, int, error) {
	var path string
	var body map[string]any
	headers := map[string]string{}
	switch provider {
	case MonitorProviderOpenAI, MonitorProviderGrok:
		path = "/v1/images/generations"
		headers["Authorization"] = "Bearer " + apiKey
		body = map[string]any{"model": model, "prompt": prompt, "n": 1}
	case MonitorProviderGemini:
		path = fmt.Sprintf("/v1beta/models/%s:generateContent", url.PathEscape(model))
		headers["x-goog-api-key"] = apiKey
		body = map[string]any{
			"contents":         []map[string]any{{"role": "user", "parts": []map[string]string{{"text": prompt}}}},
			"generationConfig": map[string]any{"responseModalities": []string{"IMAGE"}},
		}
	default:
		return "", "", 0, fmt.Errorf("image probes are not supported for provider %q", provider)
	}
	if opts != nil {
		if err := validateBodyModeParams(bodyOverrideMode(opts), opts.BodyOverride); err != nil {
			return "", "", 0, err
		}
		switch bodyOverrideMode(opts) {
		case MonitorBodyOverrideModeReplace:
			body = opts.BodyOverride
		case MonitorBodyOverrideModeMerge:
			for key, value := range opts.BodyOverride {
				if key != "model" && key != "prompt" && key != "contents" && key != "n" {
					body[key] = value
				}
			}
		}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", "", 0, err
	}
	full := joinURL(endpoint, path)
	response, status, err := doMonitorRequestWithClient(ctx, monitorImageHTTPClient, http.MethodPost, full, payload, mergeHeaders(headers, opts), monitorImageResponseMaxBytes)
	if err != nil {
		return "", "", status, err
	}
	if imageProbeResponseHasImage(provider, response) {
		return "image", string(response), status, nil
	}
	return "", string(response), status, nil
}

func imageProbeResponseHasImage(provider string, response []byte) bool {
	if !gjson.ValidBytes(response) {
		return false
	}
	nonEmpty := func(value gjson.Result) bool {
		return value.Type == gjson.String && strings.TrimSpace(value.String()) != ""
	}
	switch provider {
	case MonitorProviderOpenAI, MonitorProviderGrok:
		data := gjson.GetBytes(response, "data")
		if !data.IsArray() {
			return false
		}
		found := false
		data.ForEach(func(_, item gjson.Result) bool {
			found = nonEmpty(item.Get("b64_json")) || nonEmpty(item.Get("url"))
			return !found
		})
		return found
	case MonitorProviderGemini:
		candidates := gjson.GetBytes(response, "candidates")
		if !candidates.IsArray() {
			return false
		}
		found := false
		candidates.ForEach(func(_, candidate gjson.Result) bool {
			candidate.Get("content.parts").ForEach(func(_, part gjson.Result) bool {
				for _, media := range []struct{ data, mime string }{
					{"inlineData.data", "inlineData.mimeType"},
					{"fileData.fileUri", "fileData.mimeType"},
				} {
					mimeType := part.Get(media.mime).String()
					if nonEmpty(part.Get(media.data)) && (mimeType == "" || strings.HasPrefix(mimeType, "image/")) {
						found = true
						break
					}
				}
				return !found
			})
			return !found
		})
		return found
	default:
		return false
	}
}

func callCustomProvider(ctx context.Context, endpoint, apiKey string, opts *CheckOptions) (string, string, int, error) {
	if opts == nil {
		return "", "", 0, fmt.Errorf("custom probe configuration is required")
	}
	if err := validateCustomProbePath(opts.CustomPath); err != nil {
		return "", "", 0, err
	}
	method := defaultCustomProbeMethod(opts.CustomMethod)
	if !validCustomProbeMethod(method) {
		return "", "", 0, fmt.Errorf("unsupported custom probe method %q", method)
	}
	var payload []byte
	var err error
	if opts.CustomBody != nil {
		payload, err = json.Marshal(opts.CustomBody)
		if err != nil {
			return "", "", 0, fmt.Errorf("marshal custom probe body: %w", err)
		}
	}
	full := joinURL(endpoint, opts.CustomPath)
	headers := mergeHeaders(map[string]string{"Authorization": "Bearer " + apiKey}, opts)
	response, status, err := doMonitorRequest(ctx, method, full, payload, headers)
	if err != nil {
		return "", "", status, err
	}
	// Error bodies may be HTML or plain text even when successful responses are
	// JSON. Preserve the HTTP status and upstream message before extraction.
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", string(response), status, nil
	}
	path := strings.TrimSpace(opts.CustomResponseJSONPath)
	if path == "" {
		return string(response), string(response), status, nil
	}
	if !gjson.ValidBytes(response) {
		return "", string(response), status, fmt.Errorf("custom probe response is not valid JSON")
	}
	value := gjson.GetBytes(response, path)
	if !value.Exists() || value.Type == gjson.Null {
		return "", string(response), status, nil
	}
	if value.Type == gjson.String {
		return value.String(), string(response), status, nil
	}
	if value.IsArray() && len(value.Array()) == 0 || value.IsObject() && len(value.Map()) == 0 {
		return "", string(response), status, nil
	}
	return value.Raw, string(response), status, nil
}

func validCustomProbeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func validateCustomProbePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "\\") || strings.ContainsAny(path, "?#") || strings.HasPrefix(path, "//") {
		return fmt.Errorf("custom probe path must be a relative URL path")
	}
	parsed, err := url.Parse(path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return fmt.Errorf("custom probe path must be a relative URL path")
	}
	decoded, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		return fmt.Errorf("invalid custom probe path")
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == ".." || segment == "." {
			return fmt.Errorf("custom probe path cannot traverse directories")
		}
	}
	return nil
}

func extractMonitorResponseText(adapter providerAdapter, respBytes []byte) string {
	if adapter.extractText != nil {
		return adapter.extractText(respBytes)
	}
	return gjson.GetBytes(respBytes, adapter.textPath).String()
}

func extractAnthropicMonitorText(respBytes []byte) string {
	content := gjson.GetBytes(respBytes, "content")
	if !content.IsArray() {
		return ""
	}

	parts := make([]string, 0, 1)
	content.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() != "text" {
			return true
		}
		text := strings.TrimSpace(item.Get("text").String())
		if text != "" {
			parts = append(parts, text)
		}
		return true
	})
	return strings.Join(parts, "\n")
}

// extractOpenAIResponsesText 聚合 Responses API 的最终 assistant 文本。
// Responses 的 output 数组顺序由模型决定：reasoning / tool-call item 可能排在 message 前面，
// 因此不能假设文本永远在 output.0.content.0.text。
func extractOpenAIResponsesText(respBytes []byte) string {
	if text := gjson.GetBytes(respBytes, "output_text").String(); strings.TrimSpace(text) != "" {
		return text
	}

	var texts []string
	outputs := gjson.GetBytes(respBytes, "output")
	if outputs.IsArray() {
		outputs.ForEach(func(_, output gjson.Result) bool {
			outputType := output.Get("type").String()
			if outputType != "" && outputType != "message" {
				return true
			}

			content := output.Get("content")
			if !content.IsArray() {
				return true
			}

			content.ForEach(func(_, block gjson.Result) bool {
				blockType := block.Get("type").String()
				if blockType != "" && blockType != "output_text" {
					return true
				}
				if text := block.Get("text").String(); strings.TrimSpace(text) != "" {
					texts = append(texts, text)
				}
				return true
			})
			return true
		})
	}

	if len(texts) > 0 {
		return strings.Join(texts, "")
	}
	return gjson.GetBytes(respBytes, providerOpenAIResponsesAdapter.textPath).String()
}

// mergeHeaders 把用户自定义 headers 合并到 adapter 默认 headers 上。
// 用户值覆盖默认；命中黑名单（hop-by-hop / 由 http.Client 自管的）的 key 静默丢弃。
func mergeHeaders(base map[string]string, opts *CheckOptions) map[string]string {
	if opts == nil || len(opts.ExtraHeaders) == 0 {
		return base
	}
	out := make(map[string]string, len(base)+len(opts.ExtraHeaders))
	for k, v := range base {
		out[http.CanonicalHeaderKey(k)] = v
	}
	for k, v := range opts.ExtraHeaders {
		if IsForbiddenHeaderName(k) {
			continue
		}
		out[http.CanonicalHeaderKey(k)] = v
	}
	return out
}

// buildRequestBody 根据 body_override_mode 构造请求 body。
//
//   - off:     adapter 默认 body
//   - merge:   adapter 默认 body 与 BodyOverride 浅合并；BodyOverride 中命中
//     bodyMergeKeyDenyList[provider] 的 key 会被静默丢弃，避免破坏 challenge / model 路由
//   - replace: 直接 marshal BodyOverride 作为完整 body
//
// 任何 mode 返回的 []byte 都已经是合法 JSON，可直接送入 doMonitorRequest。
func buildRequestBody(adapter providerAdapter, provider, apiMode, model, prompt string, opts *CheckOptions) ([]byte, error) {
	mode := bodyOverrideMode(opts)

	if mode == MonitorBodyOverrideModeReplace {
		if opts == nil || len(opts.BodyOverride) == 0 {
			return nil, fmt.Errorf("replace mode: body_override is empty")
		}
		if err := validateReplaceRequestBody(provider, apiMode, opts.BodyOverride); err != nil {
			return nil, err
		}
		body, err := json.Marshal(opts.BodyOverride)
		if err != nil {
			return nil, fmt.Errorf("marshal body_override (replace): %w", err)
		}
		return body, nil
	}

	defaultBody, err := adapter.buildBody(model, prompt)
	if err != nil {
		return nil, fmt.Errorf("marshal default body: %w", err)
	}
	if mode != MonitorBodyOverrideModeMerge || opts == nil || len(opts.BodyOverride) == 0 {
		return defaultBody, nil
	}

	var defaultMap map[string]any
	if err := json.Unmarshal(defaultBody, &defaultMap); err != nil {
		return nil, fmt.Errorf("unmarshal default body for merge: %w", err)
	}
	deny := bodyMergeKeyDenyList[bodyMergeDenyKey(provider, apiMode)]
	for k, v := range opts.BodyOverride {
		if deny[k] {
			continue
		}
		defaultMap[k] = v
	}
	merged, err := json.Marshal(defaultMap)
	if err != nil {
		return nil, fmt.Errorf("marshal merged body: %w", err)
	}
	return merged, nil
}

// bodyMergeKeyDenyList 在 merge 模式下，禁止用户覆盖这些 provider-specific 的关键字段。
// 思路抄 check-cx 的 EXCLUDED_METADATA_KEYS：保护 challenge / model 路由不被用户误伤。
// 用户想动这些字段就用 replace 模式（已知会跳 challenge 校验）。
//
//nolint:gochecknoglobals // 静态查表，初始化后不变。
var bodyMergeKeyDenyList = map[string]map[string]bool{
	MonitorProviderOpenAI + ":" + MonitorAPIModeChatCompletions: {"model": true, "messages": true, "stream": true},
	MonitorProviderOpenAI + ":" + MonitorAPIModeResponses:       {"model": true, "instructions": true, "input": true, "stream": true},
	MonitorProviderGrok:      {"model": true, "messages": true, "stream": true},
	MonitorProviderAnthropic: {"model": true, "messages": true},
	MonitorProviderGemini:    {"contents": true},
	// 国产 3 家与 OpenAI Chat Completions 同构。
	MonitorProviderKimi:     {"model": true, "messages": true, "stream": true},
	MonitorProviderZhipu:    {"model": true, "messages": true, "stream": true},
	MonitorProviderDeepseek: {"model": true, "messages": true, "stream": true},
	MonitorProviderMiniMax:  {"model": true, "messages": true, "stream": true},
}

func checkAPIMode(opts *CheckOptions) string {
	if opts == nil {
		return MonitorAPIModeChatCompletions
	}
	return defaultAPIMode(opts.APIMode)
}

func bodyMergeDenyKey(provider, apiMode string) string {
	if provider == MonitorProviderOpenAI {
		return provider + ":" + defaultAPIMode(apiMode)
	}
	return provider
}

// isOpenAICompatibleChatProvider 该 provider 的探活请求是否为 OpenAI Chat
// Completions 同构（replace 模式的 body 校验按 messages 必填处理）。
func isOpenAICompatibleChatProvider(provider string) bool {
	switch provider {
	case MonitorProviderOpenAI, MonitorProviderGrok,
		MonitorProviderKimi, MonitorProviderZhipu, MonitorProviderDeepseek, MonitorProviderMiniMax:
		return true
	default:
		return false
	}
}

func validateReplaceRequestBody(provider, apiMode string, body map[string]any) error {
	if !isOpenAICompatibleChatProvider(provider) {
		return nil
	}
	switch defaultAPIMode(apiMode) {
	case MonitorAPIModeResponses:
		if strings.TrimSpace(stringFromAny(body["instructions"])) == "" || !hasNonEmptyBodyValue(body["input"]) {
			return fmt.Errorf("replace mode responses body: instructions and input are required")
		}
	case MonitorAPIModeChatCompletions:
		if !hasNonEmptyBodyValue(body["messages"]) {
			return fmt.Errorf("replace mode chat_completions body: messages are required")
		}
	}
	return nil
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func hasNonEmptyBodyValue(v any) bool {
	switch val := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(val) != ""
	case []any:
		return len(val) > 0
	case []map[string]any:
		return len(val) > 0
	case []map[string]string:
		return len(val) > 0
	default:
		return true
	}
}

func doMonitorRequest(ctx context.Context, method, fullURL string, payload []byte, headers map[string]string) ([]byte, int, error) {
	return doMonitorRequestWithClient(ctx, monitorHTTPClient, method, fullURL, payload, headers, monitorResponseMaxBytes)
}

func doMonitorRequestWithClient(ctx context.Context, client *http.Client, method, fullURL string, payload []byte, headers map[string]string, maxBytes int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	if int64(len(respBody)) > maxBytes {
		return nil, resp.StatusCode, fmt.Errorf("probe response exceeds size limit")
	}
	return respBody, resp.StatusCode, nil
}

// monitorRequestPath 返回探测请求路径。智谱按 endpoint 区分：
//   - 已带 /paas/v4（含 Coding Plan 的 /api/coding/paas/v4）：只追加 /chat/completions
//   - 官方域名根地址：/api/paas/v4/chat/completions
//   - 中转站 / 本站网关：只暴露 OpenAI 兼容的 /v1/chat/completions，与 Kimi / DeepSeek 一致
func monitorRequestPath(provider, endpoint string, adapter providerAdapter, model string) string {
	if provider != MonitorProviderZhipu {
		return adapter.buildPath(model)
	}
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return providerOpenAIPath
	}
	if strings.Contains(u.EscapedPath(), "/paas/v4") {
		return "/chat/completions"
	}
	if isZhipuOfficialHost(u) {
		return adapter.buildPath(model)
	}
	return providerOpenAIPath
}

func isZhipuOfficialHost(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	for _, official := range []string{"bigmodel.cn", "z.ai"} {
		if host == official || strings.HasSuffix(host, "."+official) {
			return true
		}
	}
	return false
}

// joinURL 保留 base 的上游路径前缀，并避免重复追加已有的 API 路径前缀。
// 使用 EscapedPath 匹配完整路径段，避免把 hostname 或编码斜杠当作路径。
func joinURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if u, err := url.Parse(base); err == nil {
		basePath := u.EscapedPath()
		for end := strings.LastIndex(path, "/"); end > 0; end = strings.LastIndex(path[:end], "/") {
			if strings.HasSuffix(basePath, path[:end]) {
				return base + path[end:]
			}
		}
	}
	return base + path
}

// extractOrigin 从一个 endpoint URL 中提取 scheme://host[:port] 部分。
func extractOrigin(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", errors.New("endpoint missing scheme or host")
	}
	return u.Scheme + "://" + u.Host, nil
}

// monitorSensitiveQueryParamRegex 匹配 URL query 中可能泄露凭证的参数：
// key / api_key / api-key / access_token / token / authorization / x-api-key。
// 大小写不敏感，匹配 `?name=value` 或 `&name=value` 形式（value 截到 & 或字符串末尾）。
var monitorSensitiveQueryParamRegex = regexp.MustCompile(`(?i)([?&](?:key|api[_-]?key|access[_-]?token|token|authorization|x-api-key)=)[^&\s"']+`)

// monitorAPIKeyPatterns 匹配常见 provider 的 API key 字面量。
// 顺序敏感：sk-ant- 必须放在 sk- 之前，否则会被通用 sk- 模式先消费。
var monitorAPIKeyPatterns = []struct {
	pattern *regexp.Regexp
	replace string
}{
	// Anthropic（带前缀，必须先匹配）：sk-ant-xxxxxxx
	{regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{20,}`), "sk-ant-***REDACTED***"},
	// OpenAI / Anthropic 通用 sk-: sk-xxxxxxx
	{regexp.MustCompile(`sk-[A-Za-z0-9-]{20,}`), "sk-***REDACTED***"},
	// xAI API Key：xai-xxxxxxx
	{regexp.MustCompile(`xai-[A-Za-z0-9_-]{6,}`), "xai-***REDACTED***"},
	// Gemini / Google API Key：固定前缀 + 35 位
	{regexp.MustCompile(`AIza[A-Za-z0-9_-]{35}`), "AIza***REDACTED***"},
	// Gemini auth key：AQ.Ab 前缀 + 48 位，长度放宽以覆盖格式变体
	{regexp.MustCompile(`AQ\.Ab[A-Za-z0-9_-]{40,}`), "AQ.***REDACTED***"},
	// JWT 三段式（Bearer 后常出现）：eyJxxx.eyJxxx.signature
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), "eyJ***REDACTED.JWT***"},
}

// sanitizeErrorMessage 擦除错误/响应文本中可能泄露的 API key。
// 处理两类来源：
//  1. URL query 中的 ?key= / ?api_key= 等（Go *url.Error 会回填完整 URL）
//  2. 上游 HTTP body 文本里直接出现的 sk-* / xai-* / AIza* / AQ.* / JWT 等密钥碎片
//
// 注意：与 gemini_messages_compat_service.go 的 sanitizeUpstreamErrorMessage 关注点类似但参数集更广，
// 监控模块独立维护，避免互相耦合。
func sanitizeErrorMessage(msg string) string {
	if msg == "" {
		return msg
	}
	msg = monitorSensitiveQueryParamRegex.ReplaceAllString(msg, `${1}REDACTED`)
	for _, p := range monitorAPIKeyPatterns {
		msg = p.pattern.ReplaceAllString(msg, p.replace)
	}
	return msg
}

// truncateMessage 把消息按 monitorMessageMaxBytes 截断，避免 DB 列溢出与日志过长。
func truncateMessage(msg string) string {
	if len(msg) <= monitorMessageMaxBytes {
		return msg
	}
	const ellipsis = "...(truncated)"
	cutoff := monitorMessageMaxBytes - len(ellipsis)
	if cutoff < 0 {
		cutoff = 0
	}
	return msg[:cutoff] + ellipsis
}

// truncateForErrorBody 把上游错误响应 body 压到 monitorErrorBodySnippetMaxBytes 以内，
// 并顺手把连续空白折成一个空格：上游 HTML 错误页常含大量缩进/换行，保留会浪费预算。
// 被 truncateMessage 做最终总截断兜底，所以这里只负责 body 自身的精简。
func truncateForErrorBody(body string) string {
	body = strings.Join(strings.Fields(body), " ")
	if len(body) <= monitorErrorBodySnippetMaxBytes {
		return body
	}
	const ellipsis = "...(body truncated)"
	cutoff := monitorErrorBodySnippetMaxBytes - len(ellipsis)
	if cutoff < 0 {
		cutoff = 0
	}
	return body[:cutoff] + ellipsis
}
