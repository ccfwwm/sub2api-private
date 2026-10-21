package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MediaTestResult struct {
	ID             string           `json:"test_id"`
	AccountID      int64            `json:"account_id"`
	Model          string           `json:"model"`
	Kind           string           `json:"test_kind"`
	Operation      string           `json:"operation"`
	State          string           `json:"status"`
	StartedAt      time.Time        `json:"started_at"`
	CompletedAt    *time.Time       `json:"completed_at,omitempty"`
	Error          string           `json:"error,omitempty"`
	Artifacts      []media.Artifact `json:"artifacts"`
	ConfigChanged  bool             `json:"config_changed"`
	Request        map[string]any   `json:"request,omitempty"`
	UpstreamID     string           `json:"upstream_task_id,omitempty"`
	UpstreamStatus string           `json:"upstream_status,omitempty"`
}
type mediaTestRecord struct {
	Result      MediaTestResult   `json:"result"`
	Capability  media.Capability  `json:"capability"`
	Fingerprint string            `json:"fingerprint"`
	UpstreamID  string            `json:"upstream_id"`
	Parameters  map[string]any    `json:"parameters"`
	Sources     []string          `json:"sources"`
	Files       map[string]string `json:"files"`
}
type mediaAccountTester struct {
	service *AccountTestService
	blobs   *media.BlobStore
	adapter *media.Adapter
	mu      sync.Mutex
	active  map[int64]string
}

func (s *AccountTestService) initMediaTests() {
	if s.cfg == nil || len(s.cfg.JWT.Secret) < 32 || s.accountRepo == nil {
		return
	}
	root := os.Getenv("SUB2API_ACCOUNT_TEST_DATA_DIR")
	if root == "" {
		root = filepath.Join("data", "account-tests")
	}
	blobs, err := media.NewBlobStore(root, s.cfg.JWT.Secret)
	if err != nil {
		return
	}
	s.mediaTests = &mediaAccountTester{service: s, blobs: blobs, adapter: &media.Adapter{Client: media.SafeClient(s.cfg.Security.URLAllowlist.AllowPrivateHosts)}, active: map[int64]string{}}
	// Only resume polling. A crash while submitting is uncertain and must never
	// result in another billable upstream POST.
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "mt_") {
			continue
		}
		r, err := s.mediaTests.read(entry.Name())
		if err != nil || (r.Result.State != "in_progress" && r.Result.State != "submitting") {
			continue
		}
		if r.UpstreamID == "" && len(r.Sources) == 0 {
			r.Result.State = "unknown"
			r.Result.Error = "Submission interrupted; acceptance uncertain; not resubmitted"
			_ = s.mediaTests.save(r)
			continue
		}
		for i := range r.Result.Artifacts {
			if i < len(r.Sources) {
				r.Result.Artifacts[i].Source = r.Sources[i]
			}
		}
		s.mediaTests.mu.Lock()
		s.mediaTests.active[r.Result.AccountID] = r.Result.ID
		s.mediaTests.mu.Unlock()
		go s.mediaTests.work(r, false)
	}
}

func testFingerprint(account *Account, cap media.Capability) string {
	cap.ValidationStatus, cap.VerificationEvidence = "", ""
	cap.Enabled = false
	raw, _ := json.Marshal([]any{account.GetCredential("base_url"), account.GetCredential("api_key"), cap})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func accountTestCapability(account *Account, model string) (media.Capability, error) {
	_, extra, err := NormalizeMediaAccountConfig(account.Platform, account.Credentials, account.Extra, false)
	if err != nil {
		return media.Capability{}, err
	}
	caps, err := accountMediaModels(extra)
	if err != nil {
		return media.Capability{}, err
	}
	for _, cap := range caps {
		if cap.Model == model {
			return cap, nil
		}
	}
	return media.Capability{}, fmt.Errorf("model %s is not configured on this account", model)
}

func (s *AccountTestService) SetMediaTemplateStore(store *media.Store) { s.mediaTemplateStore = store }
func (s *AccountTestService) resolveAccountTestTemplate(ctx context.Context, cap media.Capability) (media.Capability, error) {
	if s.mediaTemplateStore == nil {
		return cap, nil
	}
	resolved, err := s.mediaTemplateStore.ResolveTemplates(ctx, []media.Capability{cap})
	if err != nil {
		return cap, err
	}
	cap = resolved[0]
	if cap.Template == nil {
		return cap, fmt.Errorf("Configure a published protocol template in Media management before generation")
	}
	return cap, nil
}
func DefaultAccountTestPrompt(kind string) string {
	switch kind {
	case "image":
		return "A simple blue circle on a plain white background."
	case "video":
		return "A fixed camera watching a blue ball slowly roll across a plain white floor."
	case "audio":
		return "A short quiet instrumental piano melody, no vocals."
	default:
		return "请只回复 OK。"
	}
}
func accountTestPrompt(c *gin.Context) string {
	if prompt := c.GetString("account_test_prompt"); prompt != "" {
		return prompt
	}
	return DefaultAccountTestPrompt("text")
}
func applyAccountTestPrompt(c *gin.Context, payload map[string]any) {
	if payload == nil {
		return
	}
	messages, ok := payload["messages"].([]map[string]any)
	if !ok || len(messages) == 0 {
		return
	}
	content, ok := messages[0]["content"].([]map[string]any)
	if ok && len(content) > 0 {
		content[0]["text"] = accountTestPrompt(c)
	}
}
func minimalTestParameters(cap media.Capability, prompt string) map[string]any {
	if strings.TrimSpace(prompt) == "" {
		prompt = DefaultAccountTestPrompt(cap.MediaType)
	}
	p := map[string]any{"model": cap.Model, "prompt": prompt}
	if cap.MediaType == "image" {
		p["n"] = 1
		if len(cap.Sizes) > 0 {
			sizes := append([]string(nil), cap.Sizes...)
			sort.SliceStable(sizes, func(i, j int) bool { return imagePixelArea(sizes[i]) < imagePixelArea(sizes[j]) })
			p["size"] = sizes[0]
		}
	}
	if len(cap.Qualities) > 0 {
		p["quality"] = cap.Qualities[0]
	}
	if cap.MediaType == "video" {
		seconds := 0
		for _, value := range cap.Duration.Values {
			if value > 0 && (seconds == 0 || value < seconds) {
				seconds = value
			}
		}
		if cap.Duration.Min != nil {
			seconds = *cap.Duration.Min
		}
		if seconds > 0 {
			p["seconds"] = seconds
		}
	}
	if cap.MediaType == "audio" {
		p["seconds"] = 5
		p["n"] = 1
		p["instrumental"] = true
	}
	if cap.ResolutionPolicy == "required" && len(cap.Resolutions) > 0 {
		p["resolution"] = cap.Resolutions[0]
	}
	return p
}
func imagePixelArea(size string) int {
	var w, h int
	if _, err := fmt.Sscanf(size, "%dx%d", &w, &h); err != nil {
		return 1 << 30
	}
	return w * h
}

func (t *mediaAccountTester) read(id string) (*mediaTestRecord, error) {
	if !strings.HasPrefix(id, "mt_") || strings.ContainsAny(id, "/\\.\x00") {
		return nil, fmt.Errorf("invalid test ID")
	}
	var r mediaTestRecord
	err := t.blobs.ReadJSON(filepath.Join(t.blobs.Root, id, "state"), &r)
	return &r, err
}
func (t *mediaAccountTester) save(r *mediaTestRecord) error {
	_, err := t.blobs.WriteJSON(r.Result.ID, "state", r)
	return err
}
func basicTestFingerprint(account *Account) string {
	raw, _ := json.Marshal([]any{account.Platform, account.Type, account.Credentials})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func redactAccountTestError(c *gin.Context, message string) string {
	if value, ok := c.Get("account_test_credentials"); ok {
		if credentials, ok := value.(map[string]any); ok {
			for key, raw := range credentials {
				name := strings.ToLower(key)
				if strings.Contains(name, "key") || strings.Contains(name, "token") || strings.Contains(name, "secret") || strings.Contains(name, "password") || strings.Contains(name, "cookie") {
					if secret, ok := raw.(string); ok && secret != "" {
						message = strings.ReplaceAll(message, secret, "[redacted]")
					}
				}
			}
		}
	}
	return logredact.RedactText(message, "api_key", "authorization")
}
func (s *AccountTestService) beginBasicTest(c *gin.Context, account *Account, model, mode, kind string) {
	if s.mediaTests == nil {
		return
	}
	if kind == "" {
		kind = "minimal"
	}
	if account.IsSyntheticUITest() {
		kind = "simulated"
	}
	if mode == "realtime" {
		kind = "connection"
	}
	record := &mediaTestRecord{Result: MediaTestResult{ID: "mt_" + strings.ReplaceAll(uuid.NewString(), "-", ""), AccountID: account.ID, Model: model, Kind: kind, Operation: mode, State: "in_progress", StartedAt: time.Now().UTC(), Artifacts: []media.Artifact{}, Request: map[string]any{}}, Fingerprint: basicTestFingerprint(account)}
	if kind == "connection" {
		record.Result.Operation = "connection"
		record.Result.Request = map[string]any{"method": "GET", "path": "/v1/models", "generation_attempted": false}
		if account.Platform == PlatformImageVideo {
			if cap, err := accountTestCapability(account, model); err == nil {
				record.Capability = cap
				record.Fingerprint = testFingerprint(account, cap)
				record.Result.Request["adapter"] = cap.Adapter
			}
		}
	}
	if err := s.mediaTests.save(record); err != nil {
		slog.Error("account_test_persist", "account_id", account.ID)
		return
	}
	c.Set("basic_test_record", record)
}
func (s *AccountTestService) finishBasicTest(c *gin.Context) {
	value, exists := c.Get("basic_test_record")
	if !exists || s.mediaTests == nil {
		return
	}
	record, ok := value.(*mediaTestRecord)
	if !ok {
		return
	}
	if record.Result.CompletedAt != nil {
		return
	}
	record.Result.State = "unknown"
	if c.GetBool("account_test_completed_success") {
		record.Result.State = "completed"
	}
	if err := c.GetString("basic_test_error"); err != "" {
		record.Result.State = "failed"
		record.Result.Error = redactAccountTestError(c, err)
	}
	now := time.Now().UTC()
	record.Result.CompletedAt = &now
	if err := s.mediaTests.save(record); err != nil {
		slog.Error("account_test_persist", "account_id", record.Result.AccountID)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.accountRepo.UpdateExtra(ctx, record.Result.AccountID, map[string]any{"account_last_test": map[string]any{"test_id": record.Result.ID, "status": record.Result.State, "test_kind": record.Result.Kind, "model": record.Result.Model, "started_at": record.Result.StartedAt, "fingerprint": record.Fingerprint}})
	if record.Capability.Model != "" {
		s.mediaTests.recordSummary(record)
	}
}
func (s *AccountTestService) MediaTestResult(ctx context.Context, accountID int64, id string) (*MediaTestResult, error) {
	if s.mediaTests == nil {
		return nil, fmt.Errorf("test storage unavailable")
	}
	if id == "latest" {
		entries, _ := os.ReadDir(s.mediaTests.blobs.Root)
		var latest *mediaTestRecord
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			r, e := s.mediaTests.read(entry.Name())
			if e == nil && r.Result.AccountID == accountID && (latest == nil || r.Result.StartedAt.After(latest.Result.StartedAt)) {
				latest = r
			}
		}
		if latest == nil {
			return nil, fmt.Errorf("test not found")
		}
		id = latest.Result.ID
	}
	r, err := s.mediaTests.read(id)
	if err != nil || r.Result.AccountID != accountID {
		return nil, fmt.Errorf("test not found")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if r.Capability.Model == "" {
		r.Result.ConfigChanged = basicTestFingerprint(account) != r.Fingerprint
	} else {
		cap, capErr := s.recordTestCapability(ctx, account, r)
		r.Result.ConfigChanged = capErr != nil || testFingerprint(account, cap) != r.Fingerprint
	}
	return &r.Result, nil
}

// Generation records contain the resolved, pinned template. Compare the same
// representation on reads and worker checks, including after a restart.
func (s *AccountTestService) recordTestCapability(ctx context.Context, account *Account, r *mediaTestRecord) (media.Capability, error) {
	cap, err := accountTestCapability(account, r.Result.Model)
	if err != nil || r.Capability.Template == nil {
		return cap, err
	}
	return s.resolveAccountTestTemplate(ctx, cap)
}

func (s *AccountTestService) MediaTestArtifact(ctx context.Context, accountID int64, id, key string) (string, string, error) {
	if _, err := s.MediaTestResult(ctx, accountID, id); err != nil {
		return "", "", err
	}
	r, err := s.mediaTests.read(id)
	if err != nil {
		return "", "", err
	}
	for _, a := range r.Result.Artifacts {
		if a.Key == key && a.Available {
			return r.Files[key], a.MIME, nil
		}
	}
	return "", "", fmt.Errorf("artifact not found")
}

func (s *AccountTestService) testMediaAccount(c *gin.Context, account *Account, model, prompt string, opts AccountTestOptions) error {
	if s.mediaTests == nil {
		return s.sendErrorAndEnd(c, "Persistent test storage unavailable")
	}
	cap, err := accountTestCapability(account, model)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	cap, err = s.resolveAccountTestTemplate(c.Request.Context(), cap)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	base, err := s.validateUpstreamBaseURL(account.GetCredential("base_url"))
	if err != nil {
		return s.sendErrorAndEnd(c, "Invalid upstream Base URL")
	}
	if base == "" || account.GetCredential("api_key") == "" {
		return s.sendErrorAndEnd(c, "Base URL and API key required")
	}
	op := opts.Operation
	if op == "" && len(cap.Operations) > 0 {
		op = cap.Operations[0]
	}
	validOperation := false
	for _, supported := range cap.Operations {
		if supported == op {
			validOperation = true
		}
	}
	if !validOperation {
		return s.sendErrorAndEnd(c, "Operation is not configured for this model")
	}
	if err = media.ValidateCapability(cap, false); err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	p := minimalTestParameters(cap, prompt)
	if opts.TestKind == "custom" {
		for key, value := range opts.Parameters {
			if key == "model" || key == "prompt" {
				return s.sendErrorAndEnd(c, "Model and prompt have dedicated fields")
			}
			p[key] = value
		}
	}
	// Gemini's native endpoint uses contents rather than a compatibility prompt.
	if op == "gemini_image" && cap.Template == nil {
		p["contents"] = []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": p["prompt"]}}}}
		delete(p, "prompt")
		delete(p, "n")
	}
	if op == "chat_music" && cap.Template == nil {
		p["messages"] = []any{map[string]any{"role": "user", "content": p["prompt"]}}
	}
	raw, _ := json.Marshal(p)
	if len(raw) > 32<<20 || cap.Limits.MaxRequestBytes != nil && int64(len(raw)) > *cap.Limits.MaxRequestBytes {
		return s.sendErrorAndEnd(c, "Test request exceeds configured size limit")
	}
	if _, err = media.Normalize(cap, op, p); err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	t := s.mediaTests
	t.mu.Lock()
	if id := t.active[account.ID]; id != "" {
		t.mu.Unlock()
		return s.sendErrorAndEnd(c, "A media test is already running; view its result instead of resubmitting")
	}
	id := "mt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	summary := map[string]any{"payload_sha256": fmt.Sprintf("%x", sha256.Sum256(raw))}
	for _, key := range []string{"model", "seconds", "size", "quality", "resolution", "ratio", "n", "instrumental"} {
		if value, ok := p[key]; ok {
			summary[key] = value
		}
	}
	r := &mediaTestRecord{Result: MediaTestResult{ID: id, AccountID: account.ID, Model: model, Kind: opts.TestKind, Operation: op, State: "submitting", StartedAt: time.Now().UTC(), Artifacts: []media.Artifact{}, Request: summary}, Capability: cap, Fingerprint: testFingerprint(account, cap), Parameters: p, Files: map[string]string{}}
	if r.Result.Kind == "" {
		r.Result.Kind = "minimal"
	}
	err = t.save(r)
	if err == nil {
		t.active[account.ID] = id
	}
	t.mu.Unlock()
	if err != nil {
		return s.sendErrorAndEnd(c, "Unable to persist test before submission")
	}
	t.recordSummary(r)
	go t.work(r, true)
	s.sendEvent(c, TestEvent{Type: "test_start", Model: model, Data: map[string]any{"test_id": id, "test_kind": r.Result.Kind}})
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	last := ""
	for {
		select {
		case <-c.Request.Context().Done():
			return nil
		case <-ticker.C:
			current, e := s.MediaTestResult(c.Request.Context(), account.ID, id)
			if e != nil {
				return s.sendErrorAndEnd(c, e.Error())
			}
			if current.State != last {
				s.sendEvent(c, TestEvent{Type: "status", Text: current.State, Data: current})
				last = current.State
			}
			if current.State == "completed" || current.State == "failed" || current.State == "unknown" {
				for _, a := range current.Artifacts {
					event := TestEvent{Type: a.Type, MimeType: a.MIME}
					switch a.Type {
					case "image":
						event.ImageURL = a.ContentURL
					case "video":
						event.VideoURL = a.ContentURL
					case "audio":
						event.AudioURL = a.ContentURL
					}
					s.sendEvent(c, event)
				}
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: current.State == "completed", Error: current.Error, Data: current})
				return nil
			}
		}
	}
}

func (t *mediaAccountTester) work(r *mediaTestRecord, submit bool) {
	defer func() { t.mu.Lock(); delete(t.active, r.Result.AccountID); t.mu.Unlock() }()
	seconds := 3600
	if r.Capability.Limits.ObservationSeconds != nil {
		seconds = *r.Capability.Limits.ObservationSeconds
	}
	deadline := r.Result.StartedAt.Add(time.Duration(seconds) * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	finish := func(state string, err error) {
		r.Result.State = state
		now := time.Now().UTC()
		r.Result.CompletedAt = &now
		if err != nil {
			r.Result.Error = logredact.RedactText(err.Error(), "api_key", "authorization")
		}
		if saveErr := t.save(r); saveErr != nil {
			slog.Error("media_test_persist", "test_id", r.Result.ID)
			return
		}
		t.recordSummary(r)
	}
	account, err := t.service.accountRepo.GetByID(ctx, r.Result.AccountID)
	if err != nil {
		finish("failed", fmt.Errorf("account unavailable"))
		return
	}
	cap, err := t.service.recordTestCapability(ctx, account, r)
	if err != nil || testFingerprint(account, cap) != r.Fingerprint {
		finish("unknown", fmt.Errorf("account configuration changed; test not resubmitted"))
		return
	}
	a := media.Account{ID: account.ID, BaseURL: account.GetCredential("base_url"), Key: account.GetCredential("api_key")}
	req, err := media.Normalize(r.Capability, r.Result.Operation, r.Parameters)
	if err != nil {
		finish("failed", err)
		return
	}
	var result media.UpstreamResult
	if submit {
		if r.Capability.Template != nil {
			wire, e := media.BuildTemplateRequest(ctx, t.adapter.Client, r.Capability, req)
			if e != nil {
				finish("failed", e)
				return
			}
			req.Wire = &wire
		}
		result, err = t.adapter.Submit(ctx, r.Capability, req, a)
		if err != nil {
			detail := strings.ReplaceAll(err.Error(), a.Key, "[redacted]")
			finish("unknown", fmt.Errorf("submission failed; acceptance uncertain; no retry: %.1000s", detail))
			return
		}
		r.UpstreamID = result.ID
		r.Sources = nil
		for _, artifact := range result.Artifacts {
			r.Sources = append(r.Sources, artifact.Source)
		}
		r.Result.Artifacts = result.Artifacts
		r.Result.State = "in_progress"
		r.Result.UpstreamID, r.Result.UpstreamStatus = result.ID, result.Status
		if err = t.save(r); err != nil {
			slog.Error("media_test_persist", "test_id", r.Result.ID)
			finish("unknown", fmt.Errorf("unable to persist accepted task; inspect storage; do not resubmit"))
			return
		}
	} else {
		result = media.UpstreamResult{ID: r.UpstreamID, Status: r.Result.UpstreamStatus, Artifacts: r.Result.Artifacts}
	}
	for {
		if result.Status == "failed" {
			finish("failed", fmt.Errorf("%.1000s", strings.ReplaceAll(result.Diagnostic, a.Key, "[redacted]")))
			return
		}
		if len(result.Artifacts) > 0 {
			for _, artifact := range result.Artifacts {
				captured, path, _, e := t.blobs.CaptureTestArtifact(ctx, t.adapter.Client, r.Result.ID, r.Capability, artifact, a)
				if e != nil {
					finish("failed", fmt.Errorf("artifact validation failed: %s", e))
					return
				}
				captured.ContentURL = fmt.Sprintf("/api/v1/admin/accounts/%d/tests/%s/artifacts/%s", account.ID, r.Result.ID, captured.Key)
				if r.Files == nil {
					r.Files = map[string]string{}
				}
				r.Files[captured.Key] = path
				r.Result.Artifacts = []media.Artifact{captured}
				break // Administrative minimum test only needs one valid output.
			}
			finish("completed", nil)
			return
		}
		if r.UpstreamID == "" {
			finish("unknown", fmt.Errorf("no usable artifact or task ID; acceptance uncertain; do not resubmit"))
			return
		}
		interval := r.Capability.Delivery.PollSeconds
		if interval < 1 {
			interval = 10
		}
		select {
		case <-ctx.Done():
			finish("unknown", fmt.Errorf("observation deadline reached; upstream may still be running"))
			return
		case <-time.After(time.Duration(interval) * time.Second):
		}
		current, currentErr := t.service.accountRepo.GetByID(ctx, r.Result.AccountID)
		if currentErr != nil {
			finish("unknown", fmt.Errorf("account unavailable; task not resubmitted"))
			return
		}
		currentCap, currentErr := t.service.recordTestCapability(ctx, current, r)
		if currentErr != nil || testFingerprint(current, currentCap) != r.Fingerprint {
			finish("unknown", fmt.Errorf("account configuration changed; task not resubmitted"))
			return
		}
		result, err = t.adapter.Poll(ctx, media.StoredTask{ID: r.Result.ID, UpstreamID: r.UpstreamID, Snapshot: media.Snapshot{Capability: r.Capability}}, a)
		if err != nil {
			continue
		} // Retry reads only, never another generation POST.
		if result.ID == "" {
			result.ID = r.UpstreamID
		}
		r.Result.UpstreamStatus = result.Status
		r.Result.Artifacts = result.Artifacts
		r.Sources = nil
		for _, artifact := range result.Artifacts {
			r.Sources = append(r.Sources, artifact.Source)
		}
		if err = t.save(r); err != nil {
			slog.Error("media_test_persist", "test_id", r.Result.ID)
			finish("unknown", fmt.Errorf("unable to persist accepted task; inspect storage; do not resubmit"))
			return
		}
	}
}

func (t *mediaAccountTester) recordSummary(r *mediaTestRecord) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	summary := map[string]any{"test_id": r.Result.ID, "model": r.Result.Model, "status": r.Result.State, "test_kind": r.Result.Kind, "operation": r.Result.Operation, "started_at": r.Result.StartedAt, "completed_at": r.Result.CompletedAt, "fingerprint": r.Fingerprint}
	account, err := t.service.accountRepo.GetByID(ctx, r.Result.AccountID)
	if err != nil {
		return
	}
	byModel := map[string]any{}
	if old, ok := account.Extra["media_test_results"].(map[string]any); ok {
		for k, v := range old {
			byModel[k] = v
		}
	}
	byModel[r.Result.Model] = summary
	_ = t.service.accountRepo.UpdateExtra(ctx, r.Result.AccountID, map[string]any{"media_last_test": summary, "media_test_results": byModel})
}

func (s *AccountTestService) testConnectionOnly(c *gin.Context, account *Account) error {
	if account.IsSyntheticUITest() {
		return s.sendErrorAndEnd(c, "Simulated account cannot verify a real upstream connection")
	}
	if account.Type != AccountTypeAPIKey {
		return s.sendErrorAndEnd(c, "This authentication protocol has no separate model-list connectivity check; use minimum generation")
	}
	base, err := s.validateUpstreamBaseURL(account.GetCredential("base_url"))
	if err != nil {
		return s.sendErrorAndEnd(c, "Invalid upstream Base URL")
	}
	endpoint := strings.TrimRight(base, "/")
	if !strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/v1"
	}
	endpoint += "/models"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	key := account.GetCredential("api_key")
	if key == "" {
		return s.sendErrorAndEnd(c, "API key required")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	allowPrivate := s.cfg != nil && s.cfg.Security.URLAllowlist.AllowPrivateHosts
	resp, err := media.SafeClient(allowPrivate).Do(req)
	if err != nil {
		return s.sendErrorAndEnd(c, "Upstream connectivity failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Model-list check returned HTTP %d; generation was not attempted", resp.StatusCode))
	}
	var data any
	err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&data)
	if err != nil {
		return s.sendErrorAndEnd(c, "Model-list response is not JSON")
	}
	validList := false
	switch payload := data.(type) {
	case []any:
		validList = true
	case map[string]any:
		if payload["error"] == nil {
			_, validList = payload["data"].([]any)
			if !validList {
				_, validList = payload["models"].([]any)
			}
		}
	}
	if !validList {
		return s.sendErrorAndEnd(c, "Upstream did not return a model list; generation was not attempted")
	}
	s.sendEvent(c, TestEvent{Type: "status", Text: "Connection check passed; no real generation performed"})
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}
