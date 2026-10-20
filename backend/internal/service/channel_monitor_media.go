package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

// Media configuration lives in the existing request snapshot. Old clients,
// duplication and request templates can preserve it without a new API or column.
type MonitorMediaConfig struct {
	Mode     string                       `json:"mode"`
	Revision string                       `json:"revision,omitempty"`
	Models   map[string]MonitorMediaModel `json:"models"`
	TaskAPI  bool                         `json:"task_api,omitempty"`
}

type MonitorMediaModel struct {
	Capability *media.Capability `json:"capability,omitempty"`
	Operation  string            `json:"operation,omitempty"`
	Parameters map[string]any    `json:"parameters,omitempty"`
}

func monitorMediaConfig(body map[string]any) (MonitorMediaConfig, error) {
	c := MonitorMediaConfig{Mode: "connection", Models: map[string]MonitorMediaModel{}}
	if len(body) == 0 {
		return c, nil
	}
	if len(body) != 1 || body["media_monitor"] == nil {
		return c, ErrChannelMonitorInvalidRequestBody
	}
	raw, err := json.Marshal(body["media_monitor"])
	if err != nil || len(raw) > 64<<10 {
		return c, ErrChannelMonitorInvalidRequestBody
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil || (c.Mode != "connection" && c.Mode != "generation") || len(c.Models) > 20 || len(c.Revision) > 100 {
		return c, ErrChannelMonitorInvalidRequestBody
	}
	for model, entry := range c.Models {
		if c.Mode == "connection" {
			if entry.Capability != nil && (entry.Capability.Model != model || strings.TrimSpace(entry.Capability.UpstreamModel) == "") {
				return c, ErrChannelMonitorInvalidRequestBody
			}
			continue
		}
		if _, _, err := normalizeMonitorMediaModel(model, entry); err != nil {
			return c, monitorMediaConfigError(model, err)
		}
	}
	return c, nil
}

func monitorMediaConfigError(model string, err error) error {
	return infraerrors.BadRequest("CHANNEL_MONITOR_INVALID_REQUEST_BODY", truncateMessage("Media model "+model+": "+err.Error()))
}

func normalizeMonitorMediaModel(model string, entry MonitorMediaModel) (media.Capability, media.Request, error) {
	cap, known := media.ModelPreset(model)
	if entry.Capability != nil {
		cap, known = media.CapabilityDefaults(*entry.Capability), true
	}
	if !known || cap.Model != model {
		return cap, media.Request{}, fmt.Errorf("configure an exact media capability for model %s", model)
	}
	if err := media.ValidateCapability(cap, false); err != nil {
		return cap, media.Request{}, err
	}
	if cap.Limits.ObservationSeconds == nil || *cap.Limits.ObservationSeconds < 15 || *cap.Limits.ObservationSeconds > 86400 {
		return cap, media.Request{}, fmt.Errorf("observation limit must be 15-86400 seconds")
	}
	op := entry.Operation
	if op == "" && len(cap.Operations) > 0 {
		op = cap.Operations[0]
	}
	allowed := false
	for _, value := range cap.Operations {
		allowed = allowed || value == op
	}
	if !allowed {
		return cap, media.Request{}, fmt.Errorf("operation is not supported by this model")
	}
	p := minimalTestParameters(cap, "")
	for key, value := range entry.Parameters {
		p[key] = value
	}
	// One output, no materials or streaming in scheduled probes. Generation
	// parameters still go through exactly the same normalizer as business calls.
	if fmt.Sprint(p["n"]) != "<nil>" && fmt.Sprint(p["n"]) != "1" {
		return cap, media.Request{}, fmt.Errorf("monitor probes require one output")
	}
	if cap.MediaType == "audio" {
		var seconds int
		if _, err := fmt.Sscanf(fmt.Sprint(p["seconds"]), "%d", &seconds); err != nil || seconds < 1 || seconds > 600 || fmt.Sprint(seconds) != fmt.Sprint(p["seconds"]) {
			return cap, media.Request{}, fmt.Errorf("audio monitor duration must be an integer between 1 and 600")
		}
	}
	for _, key := range []string{"image", "images", "image_url", "image_urls", "input_reference", "videos", "audios", "reference_image_urls", "reference_video_urls", "reference_audio_urls", "reference_images", "reference_videos", "reference_audios", "first_frame", "last_frame", "messages", "contents", "metadata", "generationConfig", "async", "stream"} {
		if _, ok := entry.Parameters[key]; ok {
			return cap, media.Request{}, fmt.Errorf("monitor parameter %s is not supported", key)
		}
	}
	if op == "gemini_image" {
		p["contents"] = []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": p["prompt"]}}}}
		delete(p, "prompt")
		delete(p, "n")
	}
	if op == "chat_music" {
		p["messages"] = []any{map[string]any{"role": "user", "content": p["prompt"]}}
	}
	raw, _ := json.Marshal(p)
	if len(raw) > 64<<10 {
		return cap, media.Request{}, fmt.Errorf("monitor request exceeds 64 KiB")
	}
	req, err := media.Normalize(cap, op, p)
	return cap, req, err
}

func validateMonitorMediaModels(provider, primary string, extra []string, body map[string]any) error {
	if provider != MonitorProviderImageVideo {
		return nil
	}
	c, err := monitorMediaConfig(body)
	if err != nil {
		return err
	}
	if len(extra) > 19 {
		return ErrChannelMonitorInvalidRequestBody
	}
	if c.Mode == "generation" {
		for _, model := range normalizeModels(append([]string{primary}, extra...)) {
			if _, _, err := normalizeMonitorMediaModel(model, c.Models[model]); err != nil {
				return monitorMediaConfigError(model, err)
			}
		}
	}
	return nil
}

type monitorMediaHeaders struct {
	base    http.RoundTripper
	headers map[string]string
	origin  string
}

func (t monitorMediaHeaders) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme+"://"+req.URL.Host == t.origin {
		req = req.Clone(req.Context())
		for key, value := range t.headers {
			if !IsForbiddenHeaderName(key) {
				req.Header.Set(key, value)
			}
		}
	}
	return t.base.RoundTrip(req)
}

func monitorMediaClient(m *ChannelMonitor) *http.Client {
	client := *monitorHTTPClient
	client.Timeout = 180 * time.Second
	// Keep safe dialing while giving synchronous media enough time for headers.
	// A custom test transport is preserved.
	if monitorHTTPClient.Timeout == monitorRequestTimeout {
		client = *newSSRFSafeHTTPClient(180 * time.Second)
		client.Transport = &http.Transport{DialContext: safeDialContext, ForceAttemptHTTP2: true,
			MaxIdleConns: 16, IdleConnTimeout: monitorIdleConnTimeout,
			TLSHandshakeTimeout: monitorTLSHandshakeTimeout, ResponseHeaderTimeout: 180 * time.Second}
	}
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}
	origin, _ := extractOrigin(m.Endpoint)
	client.Transport = monitorMediaHeaders{base: client.Transport, headers: m.ExtraHeaders, origin: origin}
	// Redirects must never forward custom credentials to another origin.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}

func runMonitorMediaConnection(ctx context.Context, m *ChannelMonitor, c MonitorMediaConfig, client *http.Client) []*CheckResult {
	models := normalizeModels(append([]string{m.PrimaryModel}, m.ExtraModels...))
	start := time.Now()
	endpoint := strings.TrimRight(m.Endpoint, "/")
	if !strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/v1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/models", nil)
	available := map[string]bool{}
	message := ""
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+m.APIKey)
		var resp *http.Response
		resp, err = client.Do(req)
		if err == nil {
			defer func() { _ = resp.Body.Close() }()
			var payload struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			raw, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				message = fmt.Sprintf("connection: upstream HTTP %d; no generation", resp.StatusCode)
			} else if readErr != nil || len(raw) > 1<<20 || json.Unmarshal(raw, &payload) != nil || payload.Data == nil {
				message = "connection: invalid model-list response; no generation"
			} else {
				for _, item := range payload.Data {
					available[item.ID] = true
				}
			}
		}
	}
	if err != nil {
		message = "connection: request unavailable; no generation"
	}
	latency := int(time.Since(start).Milliseconds())
	results := make([]*CheckResult, 0, len(models))
	for _, model := range models {
		res := &CheckResult{Model: model, Status: MonitorStatusError, Message: message, LatencyMs: &latency, CheckedAt: start}
		upstream := model
		if entry := c.Models[model]; entry.Capability != nil {
			upstream = entry.Capability.UpstreamModel
		}
		if message == "" {
			res.Status, res.Message = MonitorStatusFailed, "connection: model is not advertised; generation not checked"
			if available[upstream] {
				res.Status, res.Message = MonitorStatusOperational, "connection: model advertised; generation not checked"
			}
		}
		results = append(results, res)
	}
	return results
}

// A durable encrypted acceptance journal is written BEFORE the billable POST.
// Each subsequent check polls the same task. Interrupted or ambiguous POSTs
// remain blocked until an administrator explicitly resets the revision.
type monitorMediaTask struct {
	Revision       string           `json:"revision"`
	Fingerprint    string           `json:"fingerprint"`
	StartedAt      time.Time        `json:"started_at"`
	State          string           `json:"state"`
	UpstreamID     string           `json:"upstream_id"`
	ResultStatus   string           `json:"result_status,omitempty"`
	Message        string           `json:"message,omitempty"`
	Capability     media.Capability `json:"capability"`
	Account        media.Account    `json:"account"`
	IdempotencyKey string           `json:"idempotency_key,omitempty"`
}

func monitorTaskAPI(m *ChannelMonitor, c MonitorMediaConfig) bool {
	u, err := url.Parse(m.Endpoint)
	if err != nil {
		return false
	}
	return c.TaskAPI || u.Hostname() == "code.aicodeme.xyz" || u.Hostname() == "code.aicodeme.cn" || u.Hostname() == "hkcode.aicodeme.xyz"
}

func (s *ChannelMonitorService) monitorMediaStatePath(id int64, model string) string {
	root := os.Getenv("SUB2API_MONITOR_DATA_DIR")
	if root == "" {
		root = filepath.Join("data", "channel-monitor-tasks")
	}
	return filepath.Join(root, fmt.Sprintf("%d-%x.state", id, sha256.Sum256([]byte(model))))
}

func (s *ChannelMonitorService) saveMonitorMediaTask(path string, task monitorMediaTask) error {
	raw, err := json.Marshal(task)
	if err != nil {
		return err
	}
	sealed, err := s.encryptor.Encrypt(string(raw))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".journal-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.WriteString(sealed); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	// Persist the directory entry too, so a host crash cannot lose the journal
	// after the upstream has accepted a request. Windows lacks directory fsync.
	if runtime.GOOS != "windows" {
		dir, err := os.Open(filepath.Dir(path))
		if err != nil {
			return err
		}
		defer func() { _ = dir.Close() }()
		return dir.Sync()
	}
	return nil
}

func (s *ChannelMonitorService) runMediaMonitor(ctx context.Context, m *ChannelMonitor) []*CheckResult {
	lock, _ := s.mediaMonitorLocks.LoadOrStore(m.ID, &sync.Mutex{})
	mu, ok := lock.(*sync.Mutex)
	if !ok || !mu.TryLock() {
		return []*CheckResult{{Model: m.PrimaryModel, Status: MonitorStatusPending, Message: "media: detection already running", CheckedAt: time.Now()}}
	}
	defer mu.Unlock()
	c, err := monitorMediaConfig(m.BodyOverride)
	if err != nil {
		return []*CheckResult{{Model: m.PrimaryModel, Status: MonitorStatusError, Message: "media: invalid configuration", CheckedAt: time.Now()}}
	}
	client := monitorMediaClient(m)
	if c.Mode == "connection" {
		return runMonitorMediaConnection(ctx, m, c, client)
	}
	results := []*CheckResult{}
	for _, model := range normalizeModels(append([]string{m.PrimaryModel}, m.ExtraModels...)) {
		if ctx.Err() != nil {
			results = append(results, &CheckResult{Model: model, Status: MonitorStatusError, Message: "media: detection deadline reached; no submission", CheckedAt: time.Now()})
			continue
		}
		results = append(results, s.runMonitorMediaGeneration(ctx, m, model, c, client))
	}
	return results
}

func (s *ChannelMonitorService) runMonitorMediaGeneration(ctx context.Context, m *ChannelMonitor, model string, c MonitorMediaConfig, client *http.Client) *CheckResult {
	res := &CheckResult{Model: model, Status: MonitorStatusError, CheckedAt: time.Now()}
	start := time.Now()
	defer func() { ms := int(time.Since(start).Milliseconds()); res.LatencyMs = &ms }()
	cap, req, err := normalizeMonitorMediaModel(model, c.Models[model])
	if err != nil {
		res.Message = "generation: invalid model parameters"
		return res
	}
	raw, _ := json.Marshal([]any{m.Endpoint, m.APIKey, m.ExtraHeaders, cap, req, c.Revision})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(raw))
	path := s.monitorMediaStatePath(m.ID, model)
	task := monitorMediaTask{Fingerprint: fingerprint, StartedAt: start, State: "submitting", Capability: cap, Account: media.Account{BaseURL: m.Endpoint, Key: m.APIKey}}
	stored, readErr := os.ReadFile(path)
	if readErr == nil {
		plain, decryptErr := s.encryptor.Decrypt(string(stored))
		if decryptErr != nil || json.Unmarshal([]byte(plain), &task) != nil {
			res.Message = "generation: task journal unreadable; no resubmission"
			return res
		}
		if task.Fingerprint != fingerprint {
			sameOwner := task.Account.BaseURL == m.Endpoint && task.Account.Key == m.APIKey
			if task.State == "completed" {
				task = monitorMediaTask{Fingerprint: fingerprint, Capability: cap, Account: media.Account{BaseURL: m.Endpoint, Key: m.APIKey}, Revision: c.Revision, State: "completed"}
			} else if sameOwner && task.UpstreamID != "" && task.ResultStatus != MonitorStatusFailed {
				// Updated defaults must not orphan the accepted original task.
				task.Fingerprint = fingerprint
			} else if task.Revision != c.Revision && task.ResultStatus == MonitorStatusFailed {
				task = monitorMediaTask{Fingerprint: fingerprint, Capability: cap, Account: media.Account{BaseURL: m.Endpoint, Key: m.APIKey}, Revision: c.Revision}
				readErr = os.ErrNotExist
			} else {
				res.Message = "generation: 配置已变化，需核对原任务后恢复；no resubmission"
				return res
			}
		}
	} else if !os.IsNotExist(readErr) {
		res.Message = "generation: task journal unavailable; no submission"
		return res
	}
	adapter := &media.Adapter{Client: client}
	task.Account.TaskAPI = monitorTaskAPI(m, c)
	wasPending := task.State != "completed" && !os.IsNotExist(readErr)
	var upstream media.UpstreamResult
	if os.IsNotExist(readErr) || task.State == "completed" {
		task.StartedAt, task.State, task.UpstreamID, task.Revision = start, "submitting", "", c.Revision
		task.Capability, task.Fingerprint = cap, fingerprint
		task.IdempotencyKey = "monitor-" + uuid.NewString()
		task.Account.IdempotencyKey = task.IdempotencyKey
		if s.saveMonitorMediaTask(path, task) != nil {
			res.Message = "generation: cannot persist acceptance journal; no submission"
			return res
		}
		upstream, err = adapter.Submit(ctx, cap, req, task.Account)
	} else {
		if task.UpstreamID == "" && task.Account.TaskAPI && task.IdempotencyKey != "" {
			task.Account.IdempotencyKey = task.IdempotencyKey
			upstream, err = adapter.Recover(ctx, task.Account, task.Capability, string(req.Operation))
			if err != nil || upstream.ID == "" {
				res.Message = "generation: 正在核对原任务；未重新提交 (no resubmission)"
				return res
			}
			task.UpstreamID = upstream.ID
			task.State = "in_progress"
		}
		if (task.State == "blocked" && task.ResultStatus == MonitorStatusFailed) || task.UpstreamID == "" {
			if task.ResultStatus == MonitorStatusFailed {
				res.Status = MonitorStatusFailed
			}
			res.Message = task.Message
			if res.Message == "" {
				res.Message = "generation: previous probe stopped; edit detection settings and reset task before retrying; no resubmission"
			}
			return res
		}

		if upstream.ID == "" {
			upstream, err = adapter.Poll(ctx, media.StoredTask{UpstreamID: task.UpstreamID, Snapshot: media.Snapshot{Capability: task.Capability}}, task.Account)
		}
	}
	if err != nil {
		res.Message = "generation: 原任务查询暂不可用；保留任务 " + task.UpstreamID + "，下次继续查询"
		return res
	}
	if upstream.ID != "" {
		task.UpstreamID = upstream.ID
	}
	switch upstream.Status {
	case "in_progress":
		task.State = "in_progress"
		if task.UpstreamID == "" {
			task.State = "blocked"
			res.Message = "generation: accepted response missing task ID; no resubmission"
			break
		}
		res.Status, res.Message = MonitorStatusPending, "generation: task in progress; 下次只查询原任务 "+task.UpstreamID
	case "completed":
		if err := media.ValidateMonitorArtifacts(ctx, client, task.Capability, upstream.Artifacts, task.Account); err != nil {
			task.State, res.Message = "blocked", "generation: result is not a valid deliverable artifact; no resubmission"
		} else {
			task.State, res.Status, res.Message = "completed", MonitorStatusOperational, "generation: 已校验实际 "+task.Capability.MediaType+" 文件；任务 "+task.UpstreamID
		}
	case "failed":
		task.State, res.Status = "blocked", MonitorStatusFailed
		res.Message = "generation: " + monitorMediaDiagnostic(upstream, m) + "; edit detection settings and reset task before retrying; no resubmission"
	default:
		task.State = "blocked"
		res.Message = "generation: " + monitorMediaDiagnostic(upstream, m) + "; acceptance uncertain; reset task only after checking the original request; no resubmission"
	}
	res.Message = truncateMessage(res.Message)
	task.ResultStatus, task.Message = res.Status, res.Message
	if s.saveMonitorMediaTask(path, task) != nil {
		res.Status, res.Message = MonitorStatusError, "generation: result journal could not be persisted; no resubmission"
	}
	if wasPending && task.State == "completed" && res.Status == MonitorStatusOperational && m.IntervalSeconds > 0 && time.Since(task.StartedAt) >= time.Duration(m.IntervalSeconds)*time.Second {
		// The previous async probe finished between scheduled checks. Start the
		// next interval only after validating and persisting that original result.
		// This avoids turning a four-minute generation schedule into eight minutes.
		next := s.runMonitorMediaGeneration(ctx, m, model, c, client)
		if next.Status == MonitorStatusPending {
			res.Message += "；本轮已提交：" + next.Message
		} else {
			res = next
		}
	}
	res.Message = truncateMessage(res.Message)
	return res
}

// Keep the upstream's actionable error instead of replacing a definite 4xx
// rejection with an ambiguous acceptance warning on the next check. Never put
// the complete response or credentials in the monitor history.
func monitorMediaDiagnostic(result media.UpstreamResult, m *ChannelMonitor) string {
	message := result.Diagnostic
	var payload struct {
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(result.Response, &payload) == nil {
		detail := payload.Message
		var nested struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(payload.Error, &nested) == nil && nested.Message != "" {
			detail = nested.Message
		} else {
			var text string
			if json.Unmarshal(payload.Error, &text) == nil && text != "" {
				detail = text
			}
		}
		if detail != "" {
			message += ": " + detail
		}
	}
	if message == "" {
		message = "upstream generation failed"
	}
	for _, secret := range append([]string{m.APIKey}, monitorMediaHeaderValues(m.ExtraHeaders)...) {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	return truncateMessage(sanitizeErrorMessage(strings.Join(strings.Fields(message), " ")))
}

func monitorMediaHeaderValues(headers map[string]string) []string {
	values := make([]string, 0, len(headers))
	for _, value := range headers {
		values = append(values, value)
	}
	return values
}
