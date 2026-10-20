package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/shopspring/decimal"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Engine struct {
	PlatformActive atomic.Bool
	Config         func(context.Context, int64) ([]Capability, []PriceCard, error)
	Store          *Store
	Blobs          *BlobStore
	Adapter        *Adapter
	Lookup         AccountLookup
	Invalidate     CacheInvalidate
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

func NewEngine(store *Store, blobs *BlobStore, lookup AccountLookup, invalidate CacheInvalidate) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	engine := &Engine{Store: store, Blobs: blobs, Lookup: lookup, Invalidate: invalidate, Adapter: &Adapter{SafeClient(false)}, ctx: ctx, cancel: cancel}
	var activity bool
	err := store.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM media_tasks) OR EXISTS(SELECT 1 FROM media_config_heads WHERE kind='capability' AND published_version IS NOT NULL)`).Scan(&activity)
	if err == nil {
		engine.PlatformActive.Store(activity)
	}
	return engine
}
func (e *Engine) Start() {
	for i := 0; i < 2; i++ {
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-e.ctx.Done():
					return
				case <-ticker.C:
					t, err := e.Store.Claim(e.ctx)
					if err == nil && t != nil {
						_ = e.Process(e.ctx, *t)
					}
					_ = e.FlushCaches(e.ctx)
				}
			}
		}()
	}
}
func (e *Engine) Stop() { e.cancel(); e.wg.Wait() }
func (e *Engine) configuration(ctx context.Context, groupID int64) ([]Capability, []PriceCard, error) {
	if e.Config != nil {
		caps, cards, err := e.Config(ctx, groupID)
		for _, cap := range caps {
			if cap.Enabled {
				e.PlatformActive.Store(true)
				break
			}
		}
		return caps, cards, err
	}
	return e.Store.Published(ctx)
}
func (e *Engine) Capability(ctx context.Context, model, operation string, identity Identity) (*Capability, error) {
	caps, _, err := e.configuration(ctx, identity.GroupID)
	if err != nil {
		return nil, err
	}
	var found *Capability
	for _, c := range caps {
		if c.Matches(model, operation, identity.GroupID) {
			if found != nil && e.Config == nil {
				return nil, fail(409, "capability_conflict", "Multiple active profiles cover this request")
			}
			if found == nil {
				copy := c
				found = &copy
			}
		}
	}
	return found, nil
}

// SelectCapability keeps each account's limits and protocol together. A request
// may use a second account when the first profile does not support its parameters.
func (e *Engine) SelectCapability(ctx context.Context, model, operation string, p map[string]any, uploads []Upload, identity Identity) (*Capability, error) {
	if e.Config == nil && p["public_profile_id"] == nil {
		return e.Capability(ctx, model, operation, identity)
	}
	caps, _, err := e.configuration(ctx, identity.GroupID)
	if err != nil {
		return nil, err
	}
	var validation error
	for _, cap := range caps {
		if !cap.Matches(model, operation, identity.GroupID) || (p["public_profile_id"] != nil && stringValue(p["public_profile_id"]) != publicProfileID(cap)) {
			continue
		}
		if _, err := NormalizeUploads(cap, operation, p, uploads); err != nil {
			if validation == nil {
				validation = err
			}
			continue
		}
		return &cap, nil
	}
	if validation == nil && p["public_profile_id"] != nil {
		validation = fail(400, "invalid_public_profile", "Selected capability is unavailable")
	}
	return nil, validation
}

// A published profile retains routing ownership after disablement. It must
// never fall back to legacy media pricing when new submissions are stopped.
func (e *Engine) ManagedModel(ctx context.Context, model, operation string, groupID int64) (bool, error) {
	models, err := e.ManagedModels(ctx, groupID, operation)
	return models[model], err
}

func (e *Engine) ManagedModels(ctx context.Context, groupID int64, operation string) (map[string]bool, error) {
	caps, _, err := e.configuration(ctx, groupID)
	if err != nil {
		return nil, err
	}
	models := map[string]bool{}
	for _, cap := range caps {
		for _, g := range cap.GroupIDs {
			if g != groupID {
				continue
			}
			for _, op := range cap.Operations {
				if operation == "" || Operation(op) == Operation(operation) {
					models[cap.Model] = true
				}
			}
		}
	}
	return models, nil
}
func (e *Engine) Snapshot(ctx context.Context, cap Capability, req Request, identity Identity) (Snapshot, int64, Calculation, error) {
	_, cards, err := e.configuration(ctx, identity.GroupID)
	if err != nil {
		return Snapshot{}, 0, Calculation{}, err
	}
	identity.Multiplier = EffectiveMultiplier(cap, identity)
	var account Account
	for _, id := range cap.AccountIDs {
		a, err := e.Lookup(id, identity.GroupID)
		if err == nil && a.Allowed {
			account = a
			break
		}
	}
	if account.ID == 0 {
		return Snapshot{}, 0, Calculation{}, fail(503, "channel_unavailable", "No configured account is currently eligible")
	}
	sale, err := ResolvePrice(cards, cap, identity, req.Operation, false, account.ID, time.Now())
	if err != nil {
		return Snapshot{}, 0, Calculation{}, err
	}
	if sale.TimeMultiplier != "" {
		a, er := decimal.NewFromString(identity.TimeMultiplier)
		if er != nil {
			return Snapshot{}, 0, Calculation{}, er
		}
		b, er := decimal.NewFromString(sale.TimeMultiplier)
		if er != nil {
			return Snapshot{}, 0, Calculation{}, er
		}
		identity.TimeMultiplier = a.Mul(b).String()
	}
	rule, err := MatchRule(sale, req.Conditions)
	if err != nil {
		return Snapshot{}, 0, Calculation{}, err
	}
	quote, maximum, err := Quote(sale, rule, req, identity)
	if err != nil {
		return Snapshot{}, 0, quote, err
	}
	snapshot := Snapshot{Capability: cap, Sale: sale, SaleRule: rule, Identity: identity, Request: req, Maximum: maximum, CostMultiplier: account.CostMultiplier}
	if snapshot.CostMultiplier == "" {
		snapshot.CostMultiplier = "1"
	}
	cost, err := ResolvePrice(cards, cap, identity, req.Operation, true, account.ID, time.Now())
	if err == nil {
		costRule, err := MatchRule(cost, req.Conditions)
		if err == nil {
			snapshot.Cost = &cost
			snapshot.CostRule = &costRule
		}
	}
	return snapshot, account.ID, quote, nil
}

// EffectiveMultiplier is shared by estimates, task snapshots and the simulator.
func EffectiveMultiplier(cap Capability, identity Identity) string {
	multiplier := identity.Multiplier
	if cap.MediaType == "image" && identity.ImageOverride != nil {
		multiplier = *identity.ImageOverride
	}
	if cap.MediaType == "video" && identity.VideoOverride != nil {
		multiplier = *identity.VideoOverride
	}
	if cap.MediaType == "audio" && cap.MusicRateOverride != nil {
		multiplier = *cap.MusicRateOverride
	}
	return multiplier
}

// PrepareRequest validates the same materials and measures the same accepted
// input durations for estimates and submissions. Caller metadata is never usage.
func (e *Engine) PrepareRequest(ctx context.Context, cap Capability, operation string, p map[string]any, uploads []Upload) (Request, error) {
	req, err := NormalizeUploads(cap, operation, p, uploads)
	if err != nil {
		return req, err
	}
	if err = e.measureInputs(ctx, cap, &req); err != nil {
		return req, err
	}
	req.Quantities["task_count"] = []string{"1"}
	return req, nil
}

func (e *Engine) Create(ctx context.Context, cap Capability, operation string, p map[string]any, uploads []Upload, identity Identity, idem string) (StoredTask, error) {
	if len(idem) > 200 {
		return StoredTask{}, fail(400, "invalid_idempotency_key", "Idempotency key exceeds 200 characters")
	}
	canonical, err := json.Marshal(struct {
		Params map[string]any
		Files  []Upload
	}{p, uploads})
	if err != nil {
		return StoredTask{}, err
	}
	sum := sha256.Sum256(canonical)
	hash := hex.EncodeToString(sum[:])
	previous, err := e.Store.Existing(ctx, identity.KeyID, operation, idem, hash)
	if err != nil {
		return StoredTask{}, err
	}
	if previous != nil {
		return *previous, nil
	}
	req, err := e.PrepareRequest(ctx, cap, operation, p, uploads)
	if err != nil {
		return StoredTask{}, err
	}
	snapshot, accountID, _, err := e.Snapshot(ctx, cap, req, identity)
	if err != nil {
		return StoredTask{}, err
	}
	id := NewTaskID()
	path, err := e.Blobs.WriteJSON(id, "request", req)
	if err != nil {
		return StoredTask{}, err
	}
	snapshot.Request.Parameters = map[string]any{"request_blob": path}
	snapshot.Request.Incoming = nil
	snapshot.Request.Uploads = nil
	snapshot.Request.MaterialRefs = nil
	task, err := e.Store.CreateTask(ctx, snapshot, accountID, idem, hash, id)
	if err != nil {
		return StoredTask{}, err
	}
	e.PlatformActive.Store(true)
	_ = e.FlushCaches(ctx)
	return task, nil
}
func (e *Engine) Process(ctx context.Context, t StoredTask) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Downloads and multi-output processing can outlast one lease interval.
	// Renew only the epoch that owns this worker, and stop on lost ownership.
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(45 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				r, err := e.Store.DB.ExecContext(ctx, `UPDATE media_tasks SET lease_until=now()+interval '4 minutes' WHERE id=$1 AND lease_epoch=$2 AND settlement_state IN ('held','pending_usage')`, t.ID, t.Epoch)
				if requireOne(r, err) != nil {
					cancel()
					return
				}
			}
		}
	}()
	if t.State == "failed" {
		return e.settleTask(ctx, t, "failure")
	}
	account, err := e.Lookup(t.AccountID, t.Identity.GroupID)
	if err != nil {
		return err
	}
	// Bound tasks retain their account even after it is paused for new requests.
	if account.Key == "" {
		return e.deferTask(ctx, t, "Bound account credentials unavailable")
	}
	var result UpstreamResult
	if t.State != "submitting" && t.ResultPath != "" {
		var object map[string]any
		if e.Blobs.ReadJSON(t.ResultPath, &object) == nil {
			stored := parseResult(object, t.Snapshot.Capability)
			if stored.Status == "completed" {
				result = stored
				result.Response, _ = json.Marshal(object)
			}
		}
	}
	if t.State == "submitting" {
		var request Request
		path := stringValue(t.Snapshot.Request.Parameters["request_blob"])
		if err = e.Blobs.ReadJSON(path, &request); err != nil {
			return e.deferTask(ctx, t, "Durable request unavailable; inspect storage")
		}
		account.ObserveSubmission = func(receipt Submission) error { _, err := e.Blobs.WriteJSON(t.ID, "submission", receipt); return err }
		result, err = e.Adapter.Submit(ctx, t.Snapshot.Capability, request, account)
	} else if result.Status == "completed" {
		// A newer adapter may recognize a durable response without resubmitting.
	} else if t.State == "completed" && t.ResultPath != "" {
		var object map[string]any
		if err = e.Blobs.ReadJSON(t.ResultPath, &object); err == nil {
			result = parseResult(object, t.Snapshot.Capability)
			result.Response, _ = json.Marshal(object)
		}
	} else if t.State == "failed" {
		return e.settleTask(ctx, t, "failure")
	} else if t.UpstreamID != "" {
		result, err = e.Adapter.Poll(ctx, t, account)
	} else {
		return e.deferTask(ctx, t, "Acceptance requires operator reconciliation")
	}
	if err != nil {
		return e.deferTask(ctx, t, "Temporary upstream query failure")
	}
	path := t.ResultPath
	if len(result.Response) > 0 {
		var object any
		if json.Unmarshal(result.Response, &object) == nil {
			path, err = e.Blobs.WriteJSON(t.ID, fmt.Sprintf("result-%d", t.Epoch), object)
			if err != nil {
				return err
			}
		}
	}
	// Completion and its response persist before downloading; a crash resumes here.
	if err = e.Store.Progress(ctx, t, result, path); err != nil {
		return err
	}
	current, err := e.Store.GetTask(ctx, t.ID)
	if err != nil {
		return err
	}
	if current.Epoch != t.Epoch {
		return fail(409, "stale_lease", "Worker lease changed")
	}
	if result.Accepted {
		calc, err := Calculate(t.Snapshot.SaleRule, current.Quantities, t.Identity.Multiplier, t.Identity.TimeMultiplier, "accepted", &t.Snapshot.Maximum)
		if err != nil {
			return err
		}
		if err = e.Store.SettleClaim(ctx, t, calc, "accepted", e.costCalculation(current, "accepted")); err != nil {
			return err
		}
	}
	if result.Status == "completed" {
		if err = e.collectArtifacts(ctx, current, result.Artifacts, account); err != nil {
			return e.deferTask(ctx, current, "Completed task awaiting valid artifact delivery")
		}
		current, err = e.Store.GetTask(ctx, t.ID)
		if err != nil {
			return err
		}
		if current.Epoch != t.Epoch {
			return fail(409, "stale_lease", "Worker lease changed")
		}
		return e.settleTask(ctx, current, "all")
	}
	if result.Status == "failed" {
		return e.settleTask(ctx, current, "failure")
	}
	return nil
}
func (e *Engine) costCalculation(t StoredTask, event string) *Calculation {
	if t.Snapshot.CostRule == nil {
		return nil
	}
	calc, err := Calculate(*t.Snapshot.CostRule, t.Quantities, t.Snapshot.CostMultiplier, "1", event, nil)
	if err != nil {
		return nil
	}
	return &calc
}
func (e *Engine) settleTask(ctx context.Context, t StoredTask, event string) error {
	if event == "failure" {
		accepted, _ := number(t.Accepted)
		if accepted.IsZero() {
			t.Quantities["task_count"] = []string{"0"}
		}
	}
	calc, err := Calculate(t.Snapshot.SaleRule, t.Quantities, t.Identity.Multiplier, t.Identity.TimeMultiplier, event, &t.Snapshot.Maximum)
	if err != nil {
		return err
	}
	return e.Store.SettleClaim(ctx, t, calc, event, e.costCalculation(t, event))
}
func (e *Engine) deferTask(ctx context.Context, t StoredTask, diagnostic string) error {
	manual := time.Since(t.Created) > time.Duration(*t.Snapshot.Capability.Limits.ObservationSeconds)*time.Second
	if t.State == "submitting" && t.UpstreamID == "" {
		manual = true
	}
	_, err := e.Store.DB.ExecContext(ctx, `UPDATE media_tasks SET diagnostic=$1,lease_until=NULL,next_poll_at=now()+interval '30 seconds',settlement_state=CASE WHEN $2 THEN 'manual_review' ELSE settlement_state END,state=CASE WHEN $2 THEN 'unknown' ELSE state END WHERE id=$3 AND lease_epoch=$4 AND settlement_state IN ('held','pending_usage')`, diagnostic, manual, t.ID, t.Epoch)
	return err
}
func (e *Engine) FlushCaches(ctx context.Context) error {
	if e.Invalidate == nil {
		return nil
	}
	rows, err := e.Store.DB.QueryContext(ctx, `SELECT `+taskSelect+` FROM media_tasks WHERE cache_dirty=true ORDER BY created_at LIMIT 20`)
	if err != nil {
		return err
	}
	tasks := []StoredTask{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			_ = rows.Close()
			return err
		}
		tasks = append(tasks, t)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if err = e.Invalidate(t.Identity); err != nil {
			return err
		}
		_, err = e.Store.DB.ExecContext(ctx, `UPDATE media_tasks SET cache_dirty=false WHERE id=$1 AND version=$2`, t.ID, t.Version)
		if err != nil {
			return err
		}
	}
	return nil
}
func (e *Engine) collectArtifacts(ctx context.Context, t StoredTask, artifacts []Artifact, account Account) error {
	if len(artifacts) == 0 {
		return fmt.Errorf("missing artifacts")
	}
	seen := map[string]bool{}
	for _, artifact := range artifacts {
		// A stale worker may finish an HTTP download after losing its lease.
		// Epoch-specific files keep it from replacing a newer worker's output.
		publicKey := artifact.Key
		artifact.Key = fmt.Sprintf("%s-%d", artifact.Key, t.Epoch)
		var path, hash string
		var err error
		if strings.HasPrefix(artifact.Source, "data:") {
			var raw []byte
			raw, artifact.MIME, err = decodeArtifact(artifact.Source)
			if err == nil && (len(raw) == 0 || len(raw) > 256<<20 || !strings.HasPrefix(artifact.MIME, artifact.Type+"/")) {
				err = fmt.Errorf("invalid inline artifact")
			}
			if err == nil {
				path, err = e.Blobs.Path(t.ID, artifact.Key)
			}
			if err == nil {
				err = atomicWrite(path, raw)
				size := int64(len(raw))
				artifact.Size = &size
				sum := sha256.Sum256(raw)
				hash = hex.EncodeToString(sum[:])
				artifact.Available = true
			}
		} else {
			artifact, path, hash, err = e.Blobs.Download(ctx, e.Adapter.Client, t, artifact, account, 512<<20)
		}
		if err != nil {
			if t.Snapshot.Sale.SuccessPolicy == "all_outputs" {
				return err
			}
			continue
		}
		if seen[hash] {
			continue
		}
		seen[hash] = true
		if err = validateArtifact(ctx, path, &artifact); err != nil {
			if t.Snapshot.Sale.SuccessPolicy == "all_outputs" {
				return err
			}
			continue
		}
		encrypted, err := e.Blobs.Seal([]byte(artifact.Source))
		if err != nil {
			return err
		}
		r, err := e.Store.DB.ExecContext(ctx, `INSERT INTO media_artifacts(task_id,key,media_type,mime_type,size_bytes,duration_seconds,content_hash,source_encrypted,file_path,available) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,true FROM media_tasks WHERE id=$1 AND lease_epoch=$10 AND settlement_state IN ('held','pending_usage') ON CONFLICT(task_id,content_hash) DO UPDATE SET available=true`, t.ID, publicKey, artifact.Type, artifact.MIME, artifact.Size, artifact.Duration, hash, encrypted, path, t.Epoch)
		if err = requireOne(r, err); err != nil {
			return err
		}
	}
	// Recover the entire durable set, including artifacts persisted before a crash.
	stored, err := e.Store.Artifacts(ctx, t.ID)
	if err != nil {
		return err
	}
	valid := 0
	durations := []string{}
	for _, a := range stored {
		if a.Available {
			valid++
			if a.Duration != nil {
				durations = append(durations, *a.Duration)
			}
		}
	}
	if valid == 0 {
		return fmt.Errorf("no deliverable artifact")
	}
	q := map[string][]string{"successful_outputs": {fmt.Sprint(valid)}, "task_count": {"1"}}
	if len(durations) == valid {
		q["artifact_duration"] = durations
	}
	raw, _ := json.Marshal(q)
	r, err := e.Store.DB.ExecContext(ctx, `UPDATE media_tasks SET quantities=quantities||$1::jsonb WHERE id=$2 AND lease_epoch=$3 AND settlement_state='pending_usage'`, raw, t.ID, t.Epoch)
	return requireOne(r, err)
}

func validateArtifact(ctx context.Context, path string, a *Artifact) error {
	if a.Type == "image" {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		config, format, err := image.DecodeConfig(file)
		if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 100000000 {
			return fmt.Errorf("invalid image artifact")
		}
		// Decoding catches truncated files whose header alone is valid.
		if _, err = file.Seek(0, 0); err != nil {
			return err
		}
		if _, _, err = image.Decode(file); err != nil {
			return fmt.Errorf("incomplete image artifact")
		}
		a.MIME = "image/" + format
		return nil
	}
	d, err := probeMedia(ctx, path, a.Type)
	if err != nil {
		return err
	}
	a.Duration = &d
	return nil
}
func probeMedia(ctx context.Context, path, kind string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries", "format=duration:stream=codec_type", "-of", "json", path)
	var output limitedBuffer
	command.Stdout = &output
	command.Stderr = &limitedBuffer{}
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("media duration unavailable")
	}
	var probe struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type string `json:"codec_type"`
		} `json:"streams"`
	}
	if err := json.NewDecoder(bytes.NewBufferString(output.String())).Decode(&probe); err != nil {
		return "", fmt.Errorf("invalid media probe")
	}
	if kind != "" {
		found := false
		for _, s := range probe.Streams {
			if s.Type == kind {
				found = true
			}
		}
		if !found {
			return "", fmt.Errorf("missing media stream")
		}
	}
	value := probe.Format.Duration
	n, err := number(value)
	if err != nil || !n.IsPositive() {
		return "", fmt.Errorf("invalid artifact duration")
	}
	return n.String(), nil
}

type limitedBuffer struct{ raw []byte }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(b.raw) < 8192 {
		remaining := 8192 - len(b.raw)
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.raw = append(b.raw, p...)
	}
	return n, nil
}
func (b *limitedBuffer) String() string { return string(b.raw) }
func (e *Engine) measureInputs(ctx context.Context, cap Capability, req *Request) error {
	// Input video/audio billing is never sourced from caller duration metadata.
	kinds := []string{"video", "audio"}
	if cap.RequestFormat == "yunfei_multimodal" || cap.RequestFormat == "v6k_url" || cap.Adapter == "yunfei_gpt_images" || cap.Adapter == "yunfei_gemini_native" || cap.Adapter == "yunfei_lyria" {
		kinds = append(kinds, "image")
	}
	for _, kind := range kinds {
		values := req.MaterialRefs[kind]
		if len(values) == 0 {
			continue
		}
		// Probe under transport bounds, never stale per-model capability limits.
		// Measurements used by pricing remain server-owned.
		maxBytes := int64(50 << 20)
		material := Material{MaxFileBytes: &maxBytes}
		measured := []string{}
		id := NewTaskID()
		defer func() { _ = os.RemoveAll(e.Blobs.Root + string(os.PathSeparator) + id) }()
		for index, v := range values {
			source, _, err := refString(v)
			if err != nil {
				return err
			}
			t := StoredTask{ID: id, Snapshot: Snapshot{Capability: cap}}
			account := Account{BaseURL: "https://invalid.invalid"}
			var path string
			if !strings.HasPrefix(source, "https://") && !strings.HasPrefix(source, "data:") {
				decoded, decodeErr := base64.StdEncoding.DecodeString(source)
				if decodeErr != nil {
					return fail(400, "invalid_material", "Invalid Base64 material")
				}
				source = "data:" + http.DetectContentType(decoded) + ";base64," + source
			}
			if strings.HasPrefix(source, "data:") {
				var raw []byte
				var mime string
				raw, mime, err = decodeArtifact(source)
				if err == nil && (!strings.HasPrefix(mime, kind+"/") || int64(len(raw)) > *material.MaxFileBytes) {
					err = fmt.Errorf("invalid input media")
				}
				if err == nil {
					path, err = e.Blobs.Path(id, fmt.Sprint(kind, index))
					if err == nil {
						err = atomicWrite(path, raw)
					}
				}
			} else {
				_, path, _, err = e.Blobs.Download(ctx, e.Adapter.Client, t, Artifact{Key: fmt.Sprint(kind, index), Type: kind, Source: source}, account, *material.MaxFileBytes)
			}
			if err != nil {
				return fail(400, "invalid_material", "Cannot validate input media")
			}
			if kind == "image" {
				artifact := Artifact{Type: "image"}
				if err := validateArtifact(ctx, path, &artifact); err != nil {
					return fail(400, "invalid_material", "Cannot validate reference image")
				}
				continue
			}
			duration, err := probeMedia(ctx, path, kind)
			if err != nil {
				return fail(400, "invalid_material_duration", "Cannot measure input duration")
			}
			measured = append(measured, duration)
		}
		req.Quantities["accepted_input_"+kind+"_duration"] = measured
	}
	return nil
}
func NormalizeUploads(cap Capability, operation string, p map[string]any, uploads []Upload) (Request, error) {
	if len(uploads) == 0 {
		return Normalize(cap, operation, p)
	}
	original := cap
	caps := map[string]Material{}
	for k, v := range cap.Materials {
		caps[k] = v
	}
	cap.Materials = caps
	normalized := map[string]any{}
	for k, v := range p {
		normalized[k] = v
	}
	if !contains(cap.Formats, "multipart") {
		return Request{}, fail(400, "unsupported_multipart", "Model only accepts JSON")
	}
	refs := map[string][]any{"image": {}, "video": {}, "audio": {}}
	originalCounts := map[string]int{}
	for kind := range refs {
		values, err := orderedValues(normalized[kind+"s"])
		if err != nil {
			return Request{}, err
		}
		refs[kind] = values
		originalCounts[kind] = len(values)
	}
	for _, u := range uploads {
		if cap.RequestFormat == "v6k_url" {
			return Request{}, fail(400, "reference_url_required", "This model accepts public material URLs only; Base64 and uploads are not supported")
		}
		if u.Field == "mask" {
			if normalized["mask"] != nil || u.MIME != "image/png" {
				return Request{}, fail(400, "invalid_mask", "One PNG mask is allowed")
			}
			normalized["mask"] = "data:image/png;base64," + base64String(u.Data)
			continue
		}
		kind := ""
		switch u.Field {
		case "image", "image[]", "images", "input_reference":
			kind = "image"
		case "video", "video[]", "videos":
			kind = "video"
		case "audio", "audio[]", "audios":
			kind = "audio"
		default:
			return Request{}, fail(400, "unsupported_upload_field", "Unsupported multipart file field")
		}
		format := strings.TrimPrefix(u.MIME, kind+"/")
		material := original.Materials[kind]
		allowed := contains(material.Formats, "file") || contains(material.Formats, "file_"+format) || (kind == "image" && u.Field == "input_reference" && contains(material.Formats, "multipart_input_reference"))
		if material.MaxFileBytes == nil || int64(len(u.Data)) > *material.MaxFileBytes || !strings.HasPrefix(u.MIME, kind+"/") || !allowed {
			return Request{}, fail(400, "unsupported_upload", "File MIME or size is not allowed")
		}
		if kind == "image" && http.DetectContentType(u.Data) != u.MIME {
			return Request{}, fail(400, "invalid_file_mime", "File content does not match MIME")
		}
		refs[kind] = append(refs[kind], "data:"+u.MIME+";base64,"+base64String(u.Data))
		material.Formats = append(material.Formats, "data_url")
		cap.Materials[kind] = material
	}
	for kind, values := range refs {
		if len(values) == originalCounts[kind] {
			continue
		}
		if cap.Adapter == "yunfei_grok" {
			if kind != "image" || len(values) != 1 || normalized["image"] != nil {
				return Request{}, fail(400, "reference_limit", "Grok accepts one image")
			}
			normalized["image"] = map[string]any{"url": values[0]}
			material := cap.Materials[kind]
			material.Formats = append(material.Formats, "data_url_in_url_object")
			cap.Materials[kind] = material
		} else {
			normalized[kind+"s"] = values
		}
	}
	req, err := Normalize(cap, operation, normalized)
	if err != nil {
		return req, err
	}
	for kind, values := range refs {
		if len(values) == originalCounts[kind] {
			continue
		}
		if originalCounts[kind] > 0 {
			req.Parameters[kind+"s"] = values[:originalCounts[kind]]
		} else {
			delete(req.Parameters, kind+"s")
		}
	}
	if cap.Adapter == "yunfei_grok" {
		delete(req.Parameters, "image")
	}
	req.Uploads = uploads
	for _, u := range uploads {
		if u.Field == "mask" {
			delete(req.Parameters, "mask")
		}
	}
	return req, nil
}
func base64String(raw []byte) string { return base64.StdEncoding.EncodeToString(raw) }
