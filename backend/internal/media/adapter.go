package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Adapter struct{ Client *http.Client }

func (a *Adapter) Submit(ctx context.Context, cap Capability, req Request, account Account) (UpstreamResult, error) {
	if cap.Template != nil {
		return a.submitTemplate(ctx, cap, req, account)
	}
	p := map[string]any{}
	for k, v := range req.Parameters {
		p[k] = v
	}
	delete(p, "async")
	if account.TaskAPI {
		p["async"] = true
	}
	path := "/v1/videos"
	switch req.Operation {
	case "image":
		path = "/v1/images/generations"
	case "image_edit":
		path = "/v1/images/edits"
	case "gemini_image":
		path = "/v1beta/models/" + url.PathEscape(cap.UpstreamModel) + ":generateContent"
		delete(p, "model")
	case "audio", "chat_music":
		path = "/v1/chat/completions"
		if p["messages"] == nil {
			p["messages"] = []any{map[string]any{"role": "user", "content": p["prompt"]}}
		}
		delete(p, "prompt")
		p["stream"] = false
		if refs := req.MaterialRefs["image"]; len(refs) > 0 {
			p["reference_images"] = refs
			delete(p, "images")
		}
	}
	if req.Operation == "video" && (cap.RequestFormat == "yunfei_multimodal" || cap.RequestFormat == "v6k_url") {
		for _, field := range []string{"images", "videos", "audios", "reference_images", "reference_videos", "reference_audios", "reference_image_urls", "reference_video_urls", "reference_audio_urls", "image", "input_reference"} {
			delete(p, field)
		}
		for _, kind := range []string{"image", "video", "audio"} {
			refs := req.MaterialRefs[kind]
			if len(refs) == 0 {
				continue
			}
			field := "reference_" + kind + "s"
			values := []any{}
			if cap.RequestFormat == "v6k_url" {
				field = kind + "s"
			}
			for _, ref := range refs {
				raw, _, err := refString(ref)
				if err != nil {
					return UpstreamResult{}, err
				}
				if cap.RequestFormat == "v6k_url" && !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, "data:") {
					decoded, err := base64.StdEncoding.DecodeString(raw)
					if err != nil {
						return UpstreamResult{}, fail(400, "invalid_reference", "Invalid Base64")
					}
					raw = "data:" + http.DetectContentType(decoded) + ";base64," + raw
				}
				if obj, ok := ref.(map[string]any); ok {
					copy := map[string]any{}
					for k, v := range obj {
						copy[k] = v
					}
					copy["url"] = raw
					delete(copy, "b64_json")
					// V6K accepts Data URL strings. Canonicalize source-only Base64
					// objects while retaining role/weight and future object attributes.
					if cap.RequestFormat == "v6k_url" && strings.HasPrefix(raw, "data:") && len(copy) == 1 {
						values = append(values, raw)
					} else {
						values = append(values, copy)
					}
				} else if cap.RequestFormat == "v6k_url" {
					if strings.HasPrefix(raw, "data:") {
						values = append(values, raw)
					} else {
						values = append(values, map[string]any{"url": raw})
					}
				} else {
					values = append(values, raw)
				}
			}
			p[field] = values
		}
		if cap.RequestFormat == "yunfei_multimodal" {
			if p["seconds"] != nil {
				p["duration"] = p["seconds"]
				delete(p, "seconds")
			}
			if p["ratio"] != nil {
				p["aspect_ratio"] = p["ratio"]
				delete(p, "ratio")
			}
		}
	}
	// Legacy mappings never override an explicitly selected protocol.
	if cap.RequestFormat == "" || cap.RequestFormat == "legacy" {
		for from, to := range cap.FieldMapping {
			if to != "" && !strings.ContainsAny(to, ":.[") && p[from] != nil && from != to {
				p[to] = p[from]
				delete(p, from)
			}
		}
	}
	if cap.Adapter == "ark_native" {
		path = arkTaskPath(account.BaseURL)
		content := []any{}
		if meta, ok := p["metadata"].(map[string]any); ok {
			if native, ok := meta["content"].([]any); ok {
				content = append(content, native...)
			}
		}
		if prompt := stringValue(p["prompt"]); prompt != "" {
			content = append([]any{map[string]any{"type": "text", "text": prompt}}, content...)
		}
		if len(content) == 0 {
			return UpstreamResult{}, fmt.Errorf("ark requires content")
		}
		if meta, ok := p["metadata"].(map[string]any); !ok || meta["content"] == nil {
			for _, kind := range []string{"image", "video", "audio"} {
				for _, ref := range req.MaterialRefs[kind] {
					raw, _, err := refString(ref)
					if err != nil {
						return UpstreamResult{}, err
					}
					part := map[string]any{}
					if obj, ok := ref.(map[string]any); ok {
						for k, v := range obj {
							if k != "url" && k != "b64_json" {
								part[k] = v
							}
						}
					}
					part["type"] = kind + "_url"
					part[kind+"_url"] = map[string]any{"url": raw}
					content = append(content, part)
				}
			}
		}
		native := map[string]any{}
		for k, v := range p {
			native[k] = v
		}
		for _, k := range []string{"prompt", "metadata", "seconds", "images", "videos", "audios"} {
			delete(native, k)
		}
		native["model"] = cap.UpstreamModel
		native["content"] = content
		for _, key := range []string{"resolution", "ratio", "generate_audio", "seed"} {
			if p[key] != nil {
				native[key] = p[key]
			}
		}
		if req.Duration != "" {
			seconds, err := requestSeconds(req.Duration)
			if err != nil {
				return UpstreamResult{}, err
			}
			native["duration"] = seconds
		}
		p = native
	}
	var body io.Reader
	contentType := "application/json"
	if len(req.Uploads) > 0 {
		buf := &bytes.Buffer{}
		writer := multipart.NewWriter(buf)
		for key, value := range p {
			var text string
			if s, ok := value.(string); ok {
				text = s
			} else {
				raw, e := json.Marshal(value)
				if e != nil {
					return UpstreamResult{}, e
				}
				text = string(raw)
			}
			if e := writer.WriteField(key, text); e != nil {
				return UpstreamResult{}, e
			}
		}
		for _, u := range req.Uploads {
			header := textproto.MIMEHeader{}
			field := u.Field
			if cap.Adapter == "yunfei_gpt_images" && len(req.Uploads) > 1 && field == "image" {
				field = "image[]"
			}
			header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, u.Name))
			header.Set("Content-Type", u.MIME)
			part, e := writer.CreatePart(header)
			if e != nil {
				return UpstreamResult{}, e
			}
			if _, e = part.Write(u.Data); e != nil {
				return UpstreamResult{}, e
			}
		}
		if e := writer.Close(); e != nil {
			return UpstreamResult{}, e
		}
		contentType = writer.FormDataContentType()
		body = buf
	} else {
		raw, e := json.Marshal(p)
		if e != nil {
			return UpstreamResult{}, e
		}
		body = bytes.NewReader(raw)
	}
	if account.ObserveSubmission != nil {
		raw, readErr := io.ReadAll(body)
		if readErr != nil {
			return UpstreamResult{}, readErr
		}
		body = bytes.NewReader(raw)
		submission := NewAdminSubmissionWithFiles(path, raw, p, req.Uploads, req.MaterialRefs, account.Key)
		submission.Method = http.MethodPost
		submission.Encoding = "json"
		if len(req.Uploads) > 0 {
			submission.Encoding = "multipart"
		}
		observe := account.ObserveSubmission
		var receiptMu sync.Mutex
		if err := observe(submission); err != nil {
			return UpstreamResult{}, err
		}
		account.ObserveSubmission = func(update Submission) error {
			receiptMu.Lock()
			defer receiptMu.Unlock()
			submission.Stage = update.Stage
			if update.ResponseStatusCode != 0 {
				submission.ResponseStatusCode = update.ResponseStatusCode
			}
			if update.ResponseContentType != "" {
				submission.ResponseContentType = update.ResponseContentType
			}
			if update.Response != nil {
				submission.Response = update.Response
			}
			return observe(submission)
		}
	}
	return a.call(ctx, account, cap, http.MethodPost, path, body, contentType, true)
}
func (a *Adapter) Poll(ctx context.Context, t StoredTask, account Account) (UpstreamResult, error) {
	if t.Snapshot.Capability.Template != nil {
		return a.pollTemplate(ctx, t, account)
	}
	path := "/v1/videos/" + url.PathEscape(t.UpstreamID)
	if account.TaskAPI {
		return a.call(ctx, account, t.Snapshot.Capability, http.MethodGet, "/v1/tasks/"+url.PathEscape(t.UpstreamID), nil, "", false)
	}
	if t.Snapshot.Capability.Adapter == "ark_native" {
		path = arkTaskPath(account.BaseURL) + "/" + url.PathEscape(t.UpstreamID)
	}
	if t.Snapshot.Capability.MediaType == "audio" {
		path = "/v1/audio/generations/" + url.PathEscape(t.UpstreamID)
	}
	return a.call(ctx, account, t.Snapshot.Capability, http.MethodGet, path, nil, "", false)
}

func arkTaskPath(base string) string {
	u, err := url.Parse(base)
	if err == nil && (strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/api/v3") || strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/v3")) {
		return "/contents/generations/tasks"
	}
	return "/api/v3/contents/generations/tasks"
}
func (a *Adapter) call(ctx context.Context, account Account, cap Capability, method, path string, body io.Reader, mime string, submission bool) (UpstreamResult, error) {
	limit := 30 * time.Second
	if submission {
		limit = 180 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	var stage atomic.Value
	stage.Store("连接")
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		DNSStart:          func(httptrace.DNSStartInfo) { stage.Store("域名解析") },
		ConnectStart:      func(string, string) { stage.Store("连接") },
		TLSHandshakeStart: func() { stage.Store("TLS 握手") },
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			stage.Store("等待响应头")
			if info.Err == nil && account.ObserveSubmission != nil {
				_ = account.ObserveSubmission(Submission{Stage: "attempted"})
			}
		},
		GotFirstResponseByte: func() { stage.Store("读取响应") },
	})
	endpoint, e := url.Parse(path)
	if e != nil {
		return UpstreamResult{}, e
	}
	target, e := originURL(account, endpoint.EscapedPath())
	if e != nil {
		return UpstreamResult{}, e
	}
	if endpoint.RawQuery != "" {
		target += "?" + endpoint.RawQuery
	}
	req, e := http.NewRequestWithContext(ctx, method, target, body)
	if e != nil {
		return UpstreamResult{}, e
	}
	if cap.Template != nil {
		applyTemplateAuth(req, cap.Template.Auth, account.Key)
	} else {
		req.Header.Set("Authorization", "Bearer "+account.Key)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sub2API-media/1")
	if submission && account.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", account.IdempotencyKey)
	}
	if mime != "" {
		req.Header.Set("Content-Type", mime)
	}
	response, e := a.Client.Do(protectMediaCredentials(req, account.Key))
	if e != nil {
		if submission {
			reason := "传输中断"
			var network net.Error
			if errors.Is(e, context.Canceled) {
				reason = "调用取消"
			} else if errors.Is(e, context.DeadlineExceeded) || (errors.As(e, &network) && network.Timeout()) {
				reason = "超时"
			}
			currentStage, _ := stage.Load().(string)
			return UpstreamResult{Status: "unknown", Diagnostic: "提交失败：" + currentStage + reason + "；受理状态待核对，请勿重复提交"}, nil
		}
		return UpstreamResult{}, fmt.Errorf("upstream query unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	raw, e := io.ReadAll(io.LimitReader(response.Body, (256<<20)+1))
	if e != nil || len(raw) > 256<<20 {
		if submission {
			return UpstreamResult{Status: "unknown", Diagnostic: "Submission response incomplete; reconcile acceptance"}, nil
		}
		return UpstreamResult{}, fmt.Errorf("upstream query response incomplete")
	}
	if submission && account.ObserveSubmission != nil {
		state := "attempted"

		var object any
		_ = json.Unmarshal(raw, &object)
		if err := account.ObserveSubmission(Submission{Stage: state, ResponseStatusCode: response.StatusCode, ResponseContentType: response.Header.Get("Content-Type"), Response: SafeAdminReviewValue(object, []string{account.Key}, 0)}); err != nil {
			return UpstreamResult{Status: "unknown", Diagnostic: "Submission receipt could not be persisted; reconcile original task"}, nil
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if !submission {
			return UpstreamResult{}, fmt.Errorf("upstream query HTTP %d", response.StatusCode)
		}
		// A complete 503 response is an explicit rejection, not a lost response.
		// Gateway timeouts can still conceal acceptance and require recovery.
		if (response.StatusCode >= 500 && response.StatusCode != http.StatusServiceUnavailable) || response.StatusCode == 408 {
			return UpstreamResult{Status: "unknown", Response: raw, Diagnostic: fmt.Sprintf("Upstream HTTP %d; acceptance not confirmed", response.StatusCode)}, nil
		}
		diagnostic := fmt.Sprintf("Upstream rejected request: HTTP %d", response.StatusCode)
		var rejectedDetail map[string]any
		if len(raw) <= 8192 && json.Unmarshal(raw, &rejectedDetail) == nil {
			if detail, ok := rejectedDetail["error"].(map[string]any); ok {
				rejectedDetail = detail
			}
			code, _ := SafeReviewValue(rejectedDetail["code"], []string{account.Key}, 0).(string)
			message, _ := SafeReviewValue(rejectedDetail["message"], []string{account.Key}, 0).(string)
			diagnostic += ": " + code + " " + message
		}
		if response.StatusCode == 400 || response.StatusCode == 415 || response.StatusCode == 422 {
			var rejected map[string]any
			if len(raw) <= 8192 && json.Unmarshal(raw, &rejected) == nil {
				code := stringValue(rejected["code"])
				if detail, ok := rejected["error"].(map[string]any); ok {
					code = stringValue(detail["code"])
				}
				if contains([]string{"unsupported_reference_format", "base64_not_supported", "reference_url_required"}, code) {
					diagnostic = "reference_url_required"
				}
			}
		}
		return UpstreamResult{Status: "failed", Response: raw, Diagnostic: diagnostic}, nil
	}
	if cap.Template != nil && cap.Template.Mode == "sync_binary" {
		mime := strings.Split(response.Header.Get("Content-Type"), ";")[0]
		if pcmMIMEConfigured(cap.Template.Response.PCMToWAV, response.Header.Get("Content-Type")) {
			mime = response.Header.Get("Content-Type")
		}
		if !strings.HasPrefix(mime, cap.MediaType+"/") || len(raw) == 0 {
			return UpstreamResult{Status: "unknown", Diagnostic: "Successful response was not the configured media type; do not resubmit"}, nil
		}
		payload, _ := json.Marshal(map[string]any{"_media_binary": map[string]any{"mime": mime, "data": base64String(raw)}})
		return UpstreamResult{Status: "completed", Accepted: true, Response: payload, Artifacts: []Artifact{{Key: cap.MediaType + "1", Type: cap.MediaType, MIME: mime, Source: "data:" + mime + ";base64," + base64String(raw)}}}, nil
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if e = decoder.Decode(&object); e != nil {
		return UpstreamResult{Status: "unknown", Diagnostic: "Non-JSON successful response; acceptance requires reconciliation"}, nil
	}
	result := parseResult(object, cap)
	result.Diagnostic, _ = SafeReviewValue(result.Diagnostic, []string{account.Key}, 0).(string)
	if submission && result.Accepted && account.ObserveSubmission != nil {
		if err := account.ObserveSubmission(Submission{Stage: "accepted"}); err != nil {
			return UpstreamResult{Status: "unknown", Response: raw, Diagnostic: "Acceptance receipt unavailable; reconcile original task"}, nil
		}
	}
	result.Response = raw
	if result.Status == "unknown" {
		result.Diagnostic = "Unrecognized successful response; do not resubmit"
	}
	return result, nil
}

// Recover performs a read only lookup; a missing response never triggers POST.
func (a *Adapter) Recover(ctx context.Context, account Account, cap Capability, operation string) (UpstreamResult, error) {
	if cap.Template == nil || cap.Template.Recover == nil {
		return UpstreamResult{Status: "unknown", Diagnostic: "No read-only recovery endpoint configured; do not resubmit"}, nil
	}
	path, err := expandTemplateEndpoint(*cap.Template.Recover, map[string]string{"model": cap.UpstreamModel, "idempotency_key": account.IdempotencyKey})
	if err != nil {
		return UpstreamResult{}, err
	}
	return a.call(ctx, account, cap, http.MethodGet, path, nil, "", false)
}
func parseResult(p map[string]any, cap Capability) UpstreamResult {
	if cap.Template != nil {
		return ParseTemplateResponse(p, cap)
	}
	result := UpstreamResult{ID: stringValue(p["id"]), Status: "unknown", Quantities: map[string][]string{}}
	if result.ID == "" {
		result.ID = stringValue(p["task_id"])
	}
	status := strings.ToLower(stringValue(p["status"]))
	if status == "" {
		status = strings.ToLower(stringValue(p["state"]))
	}
	switch status {
	case "queued", "pending", "submitting":
		result.Status = "in_progress"
	case "running", "processing", "in_progress":
		result.Status = "in_progress"
	case "succeeded", "success", "completed":
		result.Status = "completed"
	case "failed", "error", "cancelled", "canceled":
		result.Status = "failed"
	}
	seen := map[string]bool{}
	add := func(raw string, mime string) {
		if raw == "" || seen[raw] {
			return
		}
		if !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "data:") {
			return
		}
		seen[raw] = true
		result.Artifacts = append(result.Artifacts, Artifact{Key: fmt.Sprintf("%s%d", cap.MediaType, len(result.Artifacts)+1), Type: cap.MediaType, MIME: mime, Source: raw})
	}
	for _, field := range []string{"url", "download_url", "video_url", "audio_url"} {
		add(stringValue(p[field]), "")
	}
	for _, field := range []string{"data", "videos", "artifacts", "content", "output"} {
		if item, ok := p[field].(map[string]any); ok {
			for _, key := range []string{"url", "download_url", "content_url", cap.MediaType + "_url"} {
				add(stringValue(item[key]), stringValue(item["mime_type"]))
			}
		}
		if list, ok := p[field].([]any); ok {
			for _, v := range list {
				switch item := v.(type) {
				case string:
					add(item, "")
				case map[string]any:
					if value := stringValue(item["b64_json"]); value != "" {
						mime := "image/png"
						if cap.MediaType == "audio" {
							mime = "audio/mpeg"
						}
						add("data:"+mime+";base64,"+value, mime)
					}
					for _, key := range []string{"url", "download_url", "content_url", cap.MediaType + "_url"} {
						add(stringValue(item[key]), stringValue(item["mime_type"]))
					}
				}
			}
		}
	}
	if choices, ok := p["choices"].([]any); ok {
		for _, choice := range choices {
			if obj, ok := choice.(map[string]any); ok {
				if message, ok := obj["message"].(map[string]any); ok {
					links := regexp.MustCompile(`https://[^\s)<>"\]]+`).FindAllString(stringValue(message["content"]), -1)
					for _, link := range links {
						add(link, "")
					}
				}
			}
		}
	}
	if candidates, ok := p["candidates"].([]any); ok {
		for _, candidate := range candidates {
			if obj, ok := candidate.(map[string]any); ok {
				content, _ := obj["content"].(map[string]any)
				parts, _ := content["parts"].([]any)
				for _, part := range parts {
					if object, ok := part.(map[string]any); ok {
						inline := object["inlineData"]
						if inline == nil {
							inline = object["inline_data"]
						}
						if v, ok := inline.(map[string]any); ok {
							mime := stringValue(v["mimeType"])
							if mime == "" {
								mime = stringValue(v["mime_type"])
							}
							add("data:"+mime+";base64,"+stringValue(v["data"]), mime)
						}
						for _, link := range regexp.MustCompile(`https://[^\s)<>"\]]+`).FindAllString(stringValue(object["text"]), -1) {
							add(link, "")
						}
					}
				}
			}
		}
	}
	if len(result.Artifacts) > 0 && result.Status != "failed" {
		result.Status = "completed"
	}
	if result.Status == "completed" && len(result.Artifacts) == 0 && cap.MediaType == "video" && result.ID != "" && cap.Adapter != "ark_native" {
		add("/v1/videos/"+url.PathEscape(result.ID)+"/content", "video/mp4")
	}
	if result.ID != "" && result.Status == "unknown" {
		result.Status = "in_progress"
	}
	if (result.Status == "in_progress" || result.Status == "queued") && result.ID == "" {
		result.Status = "unknown"
	}
	result.Accepted = result.ID != "" || len(result.Artifacts) > 0
	// Quantity names and JSON paths are administrator-configured, adapter-whitelisted.
	for key, path := range cap.UsageFields {
		if !strings.HasPrefix(key, "provider_tokens.") && key != "provider_duration" {
			continue
		}
		var value any = p
		for _, segment := range strings.Split(path, ".") {
			obj, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = obj[segment]
		}
		if value != nil {
			if _, e := number(stringValue(value)); e == nil {
				result.Quantities[key] = []string{stringValue(value)}
			}
		}
	}
	return result
}
func decodeArtifact(source string) ([]byte, string, error) {
	pos := strings.Index(source, ",")
	if pos < 0 || !strings.Contains(source[:pos], ";base64") {
		return nil, "", fmt.Errorf("invalid inline artifact")
	}
	raw, e := base64.StdEncoding.DecodeString(source[pos+1:])
	return raw, strings.TrimSuffix(strings.TrimPrefix(source[:pos], "data:"), ";base64"), e
}
