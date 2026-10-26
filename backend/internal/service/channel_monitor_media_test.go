//go:build unit

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/stretchr/testify/require"
)

type cancelledMediaCheckRepository struct {
	ChannelMonitorRepository
	monitor        *ChannelMonitor
	historyRows    []*ChannelMonitorHistoryRow
	historyCtxErr  error
	markCheckedErr error
}

func (r *cancelledMediaCheckRepository) GetByID(context.Context, int64) (*ChannelMonitor, error) {
	return r.monitor, nil
}

func (r *cancelledMediaCheckRepository) InsertHistoryBatch(ctx context.Context, rows []*ChannelMonitorHistoryRow) error {
	r.historyRows = rows
	r.historyCtxErr = ctx.Err()
	return nil
}

func (r *cancelledMediaCheckRepository) MarkChecked(ctx context.Context, _ int64, _ time.Time) error {
	r.markCheckedErr = ctx.Err()
	return nil
}

type mediaCancellationRuntimeReader struct{}

func (mediaCancellationRuntimeReader) GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntime {
	return ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1}
}

func TestMediaMonitorConnectionNoGenerationAndModelVisibility(t *testing.T) {
	swapMonitorHTTPClient(t)
	count := 0
	body := `{"data":[{"id":"omni-flash"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "/prefix/v1/models", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	m := &ChannelMonitor{ID: 1, Endpoint: server.URL + "/prefix/v1", APIKey: "test-key", PrimaryModel: "omni-flash", ExtraModels: []string{"missing"}}
	s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	result := s.runMediaMonitor(context.Background(), m)
	require.Equal(t, MonitorStatusOperational, result[0].Status)
	require.Contains(t, result[0].Message, "generation not checked")
	require.Equal(t, MonitorStatusFailed, result[1].Status)
	require.Equal(t, 1, count)
	body = `{"error":{"message":"test-key"}}`
	result = s.runMediaMonitor(context.Background(), m)
	require.Equal(t, MonitorStatusError, result[0].Status)
	require.NotContains(t, result[0].Message, "test-key")
}

func monitorTestMediaModel(t *testing.T, model, preset, templateID string, parameters map[string]any) MonitorMediaModel {
	t.Helper()
	cap, ok := media.ModelPreset(preset)
	require.True(t, ok)
	for _, template := range media.BuiltinTemplates() {
		if template.ID == templateID {
			cap.Model, cap.UpstreamModel = model, model
			cap.Template, cap.TemplateRef = &template, &media.TemplateRef{ID: templateID, Version: 1}
			cap.Adapter, cap.RequestFormat = "template", "unified"
			cap.Operations = []string{template.Operations[0]}
			return MonitorMediaModel{Capability: &cap, Parameters: parameters}
		}
	}
	t.Fatalf("missing template %s", templateID)
	return MonitorMediaModel{}
}

func TestMediaMonitorNormalizesExplicitProtocolsAndRejectsUnsafeParameters(t *testing.T) {
	for model, templateID := range map[string]string{"gpt-image-2": "image-json", "omni-flash": "video-arrays", "veo-3.1-lite": "video-arrays", "lyria-3-pro": "music-chat-json"} {
		t.Run(model, func(t *testing.T) {
			entry := monitorTestMediaModel(t, model, model, templateID, map[string]any{"prompt": "custom probe", "optional": true})
			cap, req, err := normalizeMonitorMediaModel(model, entry)
			require.NoError(t, err)
			require.Equal(t, cap.UpstreamModel, req.Parameters["model"])
			require.NotEmpty(t, req.Operation)
			require.Equal(t, true, req.Parameters["optional"])
			_, _, err = normalizeMonitorMediaModel(model, MonitorMediaModel{})
			require.ErrorContains(t, err, "configure an exact media capability")
		})
	}
	for _, params := range []map[string]any{{"n": 2}, {"messages": []any{}}, {"seconds": 3601}, {"seconds": 0}, {"seconds": 1.5}, {"model": "different"}, {"images": []any{}}, {"stream": false}} {
		_, _, err := normalizeMonitorMediaModel("omni-flash", monitorTestMediaModel(t, "omni-flash", "omni-flash", "video-arrays", params))
		require.Error(t, err)
	}
	require.Error(t, validateMonitorMediaModels(MonitorProviderImageVideo, "unknown", nil, map[string]any{"media_monitor": map[string]any{"mode": "generation"}}))
	require.NoError(t, validateMonitorMediaModels(MonitorProviderImageVideo, "unknown", nil, nil))
	require.Error(t, validateCheckMode(MonitorProviderImageVideo, MonitorCheckModeQuota))
}

func TestMediaMonitorNativeGeminiPrompt(t *testing.T) {
	cap, _ := media.ModelPreset("gpt-image-2")
	cap.Adapter, cap.MediaType, cap.Operations = "yunfei_gemini_native", "image", []string{"gemini_image"}
	_, req, err := normalizeMonitorMediaModel(cap.Model, MonitorMediaModel{Capability: &cap, Parameters: map[string]any{"prompt": "custom geometry"}})
	require.NoError(t, err)
	raw, _ := json.Marshal(req.Parameters)
	require.Contains(t, string(raw), "custom geometry")
	require.NotContains(t, string(raw), `"prompt"`)
}

func TestMediaMonitorAudioUsesChatAndAudioTaskQueries(t *testing.T) {
	swapMonitorHTTPClient(t)
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	posts, polls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			require.Equal(t, "/v1/chat/completions", r.URL.Path)
			var p map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&p))
			require.Equal(t, false, p["stream"])
			require.Contains(t, fmt.Sprint(p["messages"]), "custom music")
			_, _ = w.Write([]byte(`{"id":"audio-task","status":"queued"}`))
		} else {
			polls++
			require.Equal(t, "/v1/audio/generations/audio-task", r.URL.Path)
			_, _ = w.Write([]byte(`{"id":"audio-task","status":"processing"}`))
		}
	}))
	defer server.Close()
	entry := monitorTestMediaModel(t, "lyria-3-pro", "lyria-3-pro", "music-chat-json", map[string]any{"prompt": "custom music"})
	entry.Capability.Template.Query = &media.TemplateEndpoint{Method: "GET", Path: "/v1/audio/generations/{task_id}"}
	m := &ChannelMonitor{ID: 8, Endpoint: server.URL, APIKey: "key", PrimaryModel: "lyria-3-pro", BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"lyria-3-pro": entry}}}}
	s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	require.Equal(t, MonitorStatusPending, s.runMediaMonitor(context.Background(), m)[0].Status)
	require.Equal(t, MonitorStatusPending, s.runMediaMonitor(context.Background(), m)[0].Status)
	require.Equal(t, 1, posts)
	require.Equal(t, 1, polls)
}

func TestMediaMonitorCreateReadUpdateAndDuplicatePreserveSnapshot(t *testing.T) {
	repo := &duplicateChannelMonitorRepoStub{}
	s := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	body := map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"omni-flash": monitorTestMediaModel(t, "omni-flash", "omni-flash", "video-arrays", map[string]any{"prompt": "saved prompt"})}}}
	m, err := s.Create(context.Background(), ChannelMonitorCreateParams{Name: "media", Provider: MonitorProviderImageVideo, Endpoint: "https://8.8.8.8", APIKey: "test-key", PrimaryModel: "omni-flash", IntervalSeconds: 60, BodyOverrideMode: "merge", BodyOverride: body})
	require.NoError(t, err)
	repo.source = repo.created[0]
	copy, err := s.Duplicate(context.Background(), m.ID, 1, "admin:1", "copy-media")
	require.NoError(t, err)
	require.False(t, copy.Enabled)
	c, err := monitorMediaConfig(copy.BodyOverride)
	require.NoError(t, err)
	require.Equal(t, "saved prompt", c.Models["omni-flash"].Parameters["prompt"])
	connection := map[string]any{"media_monitor": MonitorMediaConfig{Mode: "connection"}}
	require.NoError(t, applyMonitorUpdate(copy, ChannelMonitorUpdateParams{BodyOverride: &connection}))
	c, err = monitorMediaConfig(copy.BodyOverride)
	require.NoError(t, err)
	require.Equal(t, "connection", c.Mode)
}

func TestMediaMonitorAcceptedTaskSurvivesRestartAndFailureNeverResubmits(t *testing.T) {
	swapMonitorHTTPClient(t)
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	posts, polls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			require.Equal(t, "/v1/videos", r.URL.Path)
			var p map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&p))
			require.Equal(t, float64(4), p["seconds"])
			require.Equal(t, "custom probe", p["prompt"])
			_, _ = w.Write([]byte(`{"id":"accepted-task","status":"queued"}`))
		} else {
			polls++
			require.Equal(t, "/v1/videos/accepted-task", r.URL.Path)
			_, _ = w.Write([]byte(`{"id":"accepted-task","status":"failed","error":"test-key"}`))
		}
	}))
	defer server.Close()
	m := &ChannelMonitor{ID: 5, Endpoint: server.URL, APIKey: "test-key", PrimaryModel: "omni-flash", BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"omni-flash": monitorTestMediaModel(t, "omni-flash", "omni-flash", "video-arrays", map[string]any{"prompt": "custom probe"})}}}}
	s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	result := s.runMediaMonitor(context.Background(), m)
	require.Equal(t, MonitorStatusPending, result[0].Status)
	require.Equal(t, 1, posts)
	// A new service instance reads the persistent acceptance journal.
	s = NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	result = s.runMediaMonitor(context.Background(), m)
	require.Equal(t, MonitorStatusFailed, result[0].Status)
	require.Equal(t, 1, polls)
	result = s.runMediaMonitor(context.Background(), m)
	require.Equal(t, 1, posts)
	require.NotContains(t, result[0].Message, "test-key")
	c := m.BodyOverride["media_monitor"].(MonitorMediaConfig)
	c.Revision = "manual-reset"
	m.BodyOverride["media_monitor"] = c
	result = s.runMediaMonitor(context.Background(), m)
	require.Equal(t, MonitorStatusPending, result[0].Status)
	require.Equal(t, 2, posts)
}

func TestMediaMonitorRequiresValidArtifactAndRoutesImages(t *testing.T) {
	swapMonitorHTTPClient(t)
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	output := base64.StdEncoding.EncodeToString(buf.Bytes())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/images/generations", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": output}}})
	}))
	defer server.Close()
	m := &ChannelMonitor{ID: 6, Endpoint: server.URL, APIKey: "key", PrimaryModel: "gpt-image-2", BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"gpt-image-2": monitorTestMediaModel(t, "gpt-image-2", "gpt-image-2", "image-json", nil)}}}}
	s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	require.Equal(t, MonitorStatusOperational, s.runMediaMonitor(context.Background(), m)[0].Status)
	output = base64.StdEncoding.EncodeToString([]byte("not an image"))
	require.Equal(t, MonitorStatusError, s.runMediaMonitor(context.Background(), m)[0].Status)
}

func TestMediaMonitorInterruptedSubmissionAndCorruptJournalNeverSubmit(t *testing.T) {
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	m := &ChannelMonitor{ID: 7, Endpoint: "https://invalid.invalid", APIKey: "key", PrimaryModel: "omni-flash"}
	c := MonitorMediaConfig{Mode: "generation"}
	cap, req, err := normalizeMonitorMediaModel(m.PrimaryModel, monitorTestMediaModel(t, "omni-flash", "omni-flash", "video-arrays", nil))
	require.NoError(t, err)
	raw, _ := json.Marshal([]any{m.Endpoint, m.APIKey, m.ExtraHeaders, cap, req, c.Revision})
	path := s.monitorMediaStatePath(m.ID, m.PrimaryModel)
	require.NoError(t, s.saveMonitorMediaTask(path, monitorMediaTask{Fingerprint: fmt.Sprintf("%x", sha256.Sum256(raw)), State: "submitting", Capability: cap, Account: media.Account{BaseURL: m.Endpoint, Key: m.APIKey}}))
	res := s.runMonitorMediaGeneration(context.Background(), m, m.PrimaryModel, c, &http.Client{})
	require.Contains(t, res.Message, "no resubmission")
	require.NoError(t, os.WriteFile(path, []byte("corrupt"), 0600))
	res = s.runMonitorMediaGeneration(context.Background(), m, m.PrimaryModel, c, &http.Client{})
	require.Contains(t, res.Message, "journal unreadable")
}

func TestMediaMonitorLostSubmissionResponseRequiresConfirmedReset(t *testing.T) {
	swapMonitorHTTPClient(t)
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		if posts == 1 {
			connection, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			require.NoError(t, connection.Close())
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"accepted-after-confirmed-reset","status":"queued"}`))
	}))
	defer server.Close()

	encryptor := &duplicateChannelMonitorEncryptor{}
	encryptedAPIKey, err := encryptor.Encrypt("test-key")
	require.NoError(t, err)
	monitor := &ChannelMonitor{ID: 48, Provider: MonitorProviderImageVideo, Endpoint: server.URL, APIKey: encryptedAPIKey, PrimaryModel: "omni-flash", BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"omni-flash": monitorTestMediaModel(t, "omni-flash", "omni-flash", "video-arrays", nil)}}}}
	repo := &cancelledMediaCheckRepository{monitor: monitor}
	service := NewChannelMonitorService(repo, encryptor)
	first := service.runMediaMonitor(context.Background(), monitor)[0]
	require.Equal(t, MonitorStatusError, first.Status)
	require.Contains(t, first.Message, "acceptance uncertain")
	require.Contains(t, first.Message, "reset task")
	require.Equal(t, 1, posts)

	next := service.runMediaMonitor(context.Background(), monitor)[0]
	require.Equal(t, first.Message, next.Message)
	require.Equal(t, 1, posts)
	require.Error(t, service.ConfirmMediaTaskReset(context.Background(), monitor.ID, monitor.PrimaryModel, false))
	require.NoError(t, service.ConfirmMediaTaskReset(context.Background(), monitor.ID, monitor.PrimaryModel, true))

	retried := service.runMediaMonitor(context.Background(), monitor)[0]
	require.Equal(t, MonitorStatusPending, retried.Status)
	require.Equal(t, 2, posts)
}

func TestMediaMonitorRejectionKeepsReasonAndFailedStatusWithoutResubmitting(t *testing.T) {
	swapMonitorHTTPClient(t)
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"Model gpt-image2.5 is not available for this group; private-key custom-secret"}}`))
	}))
	defer server.Close()
	m := &ChannelMonitor{ID: 18, Endpoint: server.URL, APIKey: "private-key", ExtraHeaders: map[string]string{"X-Secret": "custom-secret"}, PrimaryModel: "gpt-image-2", BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"gpt-image-2": monitorTestMediaModel(t, "gpt-image-2", "gpt-image-2", "image-json", nil)}}}}
	s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	first := s.runMediaMonitor(context.Background(), m)[0]
	require.Equal(t, MonitorStatusFailed, first.Status)
	require.Contains(t, first.Message, "HTTP 404")
	require.Contains(t, first.Message, "Model gpt-image2.5 is not available for this group")
	require.NotContains(t, first.Message, "acceptance uncertain")
	require.NotContains(t, first.Message, "private-key")
	require.NotContains(t, first.Message, "custom-secret")
	// The useful diagnostic and classification must survive service restarts.
	s = NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	next := s.runMediaMonitor(context.Background(), m)[0]
	require.Equal(t, first.Status, next.Status)
	require.Equal(t, first.Message, next.Message)
	require.Equal(t, 1, posts)
}

func TestMediaMonitorRetriesWrapped429OnlyAfterRetryAfterOnLaterCheck(t *testing.T) {
	swapMonitorHTTPClient(t)
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		posts++
		if posts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":{"code":"service_busy","message":"upstream busy"},"upstream_status_code":429}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"accepted-after-retry","status":"queued"}`))
	}))
	defer server.Close()
	m := &ChannelMonitor{ID: 33, Endpoint: server.URL, APIKey: "test-key", PrimaryModel: "omni-flash", BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"omni-flash": monitorTestMediaModel(t, "omni-flash", "omni-flash", "video-arrays", nil)}}}}
	s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})

	result := s.runMediaMonitor(context.Background(), m)
	require.Equal(t, MonitorStatusFailed, result[0].Status)
	require.Contains(t, result[0].Message, "HTTP 429")
	require.Contains(t, result[0].Message, "no resubmission in this request")
	require.Equal(t, 1, posts)

	result = s.runMediaMonitor(context.Background(), m)
	require.Equal(t, MonitorStatusFailed, result[0].Status)
	require.Contains(t, result[0].Message, "retry available after")
	require.Equal(t, 1, posts)

	time.Sleep(1100 * time.Millisecond)
	result = s.runMediaMonitor(context.Background(), m)
	require.Equal(t, MonitorStatusPending, result[0].Status)
	require.Equal(t, 2, posts)
}

func TestMediaMonitorOrdinary502NeverResubmitsUncertainTask(t *testing.T) {
	swapMonitorHTTPClient(t)
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"gateway failure"}}`))
	}))
	defer server.Close()
	m := &ChannelMonitor{ID: 34, Endpoint: server.URL, APIKey: "test-key", PrimaryModel: "omni-flash", BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"omni-flash": monitorTestMediaModel(t, "omni-flash", "omni-flash", "video-arrays", nil)}}}}
	s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})

	for i := 0; i < 2; i++ {
		result := s.runMediaMonitor(context.Background(), m)
		require.Equal(t, MonitorStatusError, result[0].Status)
		require.Contains(t, result[0].Message, "acceptance uncertain")
	}
	require.Equal(t, 1, posts)
}

func TestCancelledMediaCheckPersistsHistoryWithoutSubmitting(t *testing.T) {
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	encryptor := &duplicateChannelMonitorEncryptor{}
	encryptedAPIKey, err := encryptor.Encrypt("test-key")
	require.NoError(t, err)
	monitor := &ChannelMonitor{
		ID:           991,
		Provider:     MonitorProviderImageVideo,
		Endpoint:     "https://example.invalid",
		APIKey:       encryptedAPIKey,
		PrimaryModel: "omni-flash",
		BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{
			Mode:   "generation",
			Models: map[string]MonitorMediaModel{"omni-flash": monitorTestMediaModel(t, "omni-flash", "omni-flash", "video-arrays", nil)},
		}},
	}
	repo := &cancelledMediaCheckRepository{monitor: monitor}
	service := NewChannelMonitorService(repo, encryptor)
	service.SetRuntimeReader(mediaCancellationRuntimeReader{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results, err := service.RunCheck(ctx, monitor.ID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, MonitorStatusError, results[0].Status)
	require.Len(t, repo.historyRows, 1)
	require.Equal(t, MonitorStatusError, repo.historyRows[0].Status)
	require.NoError(t, repo.historyCtxErr)
	require.NoError(t, repo.markCheckedErr)
}

func TestMediaMonitorUnknownAcceptanceStaysBlocked(t *testing.T) {
	swapMonitorHTTPClient(t)
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer server.Close()
	m := &ChannelMonitor{ID: 19, Endpoint: server.URL, APIKey: "private-key", PrimaryModel: "gpt-image-2", BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"gpt-image-2": monitorTestMediaModel(t, "gpt-image-2", "gpt-image-2", "image-json", nil)}}}}
	s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	first := s.runMediaMonitor(context.Background(), m)[0]
	require.Equal(t, MonitorStatusError, first.Status)
	require.Contains(t, first.Message, "HTTP 504")
	require.Contains(t, first.Message, "acceptance uncertain")
	next := s.runMediaMonitor(context.Background(), m)[0]
	require.Equal(t, first.Message, next.Message)
	require.Equal(t, 1, posts)
}

func TestMediaMonitorNewProbeRequiresExplicitPublishedTemplate(t *testing.T) {
	swapMonitorHTTPClient(t)
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"id":"unexpected","status":"queued"}`))
	}))
	defer server.Close()
	legacy, ok := media.ModelPreset("gpt-image-2")
	require.True(t, ok)
	draft := monitorTestMediaModel(t, legacy.Model, legacy.Model, "image-json", nil)
	draft.Capability.Template.Status = "draft"
	for _, entry := range []MonitorMediaModel{{}, {Capability: &legacy}, draft} {
		m := &ChannelMonitor{ID: 61, Endpoint: server.URL, APIKey: "test-key", PrimaryModel: legacy.Model,
			BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{legacy.Model: entry}}}}
		s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
		result := s.runMediaMonitor(context.Background(), m)
		require.Equal(t, MonitorStatusError, result[0].Status)
		require.Contains(t, result[0].Message, "protocol template")
	}
	require.Zero(t, requests)
	_, req, err := normalizeMonitorMediaModel("unlisted-model", monitorTestMediaModel(t, "unlisted-model", "gpt-image-2", "image-json", nil))
	require.NoError(t, err)
	require.Equal(t, "unlisted-model", req.Parameters["model"])
}
