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

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/stretchr/testify/require"
)

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

func TestMediaMonitorNormalizesKnownProtocolsAndRejectsUnsafeParameters(t *testing.T) {
	for _, model := range []string{"gpt-image-2", "omni-flash", "veo-3.1-lite", "lyria-3-pro"} {
		t.Run(model, func(t *testing.T) {
			cap, req, err := normalizeMonitorMediaModel(model, MonitorMediaModel{Parameters: map[string]any{"prompt": "custom probe"}})
			require.NoError(t, err)
			require.Equal(t, cap.UpstreamModel, req.Parameters["model"])
			require.NotEmpty(t, req.Operation)
		})
	}
	for _, params := range []map[string]any{{"n": 2}, {"unsupported": true}, {"messages": []any{}}, {"seconds": 1000}, {"model": "different"}} {
		_, _, err := normalizeMonitorMediaModel("omni-flash", MonitorMediaModel{Parameters: params})
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
	m := &ChannelMonitor{ID: 8, Endpoint: server.URL, APIKey: "key", PrimaryModel: "lyria-3-pro", BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"lyria-3-pro": {Parameters: map[string]any{"prompt": "custom music"}}}}}}
	s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	require.Equal(t, MonitorStatusPending, s.runMediaMonitor(context.Background(), m)[0].Status)
	require.Equal(t, MonitorStatusPending, s.runMediaMonitor(context.Background(), m)[0].Status)
	require.Equal(t, 1, posts)
	require.Equal(t, 1, polls)
}

func TestMediaMonitorCreateReadUpdateAndDuplicatePreserveSnapshot(t *testing.T) {
	repo := &duplicateChannelMonitorRepoStub{}
	s := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	body := map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"omni-flash": {Parameters: map[string]any{"prompt": "saved prompt"}}}}}
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
			require.Equal(t, float64(4), p["duration"])
			require.Equal(t, "custom probe", p["prompt"])
			_, _ = w.Write([]byte(`{"id":"accepted-task","status":"queued"}`))
		} else {
			polls++
			require.Equal(t, "/v1/videos/accepted-task", r.URL.Path)
			_, _ = w.Write([]byte(`{"id":"accepted-task","status":"failed","error":"test-key"}`))
		}
	}))
	defer server.Close()
	m := &ChannelMonitor{ID: 5, Endpoint: server.URL, APIKey: "test-key", PrimaryModel: "omni-flash", BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", Models: map[string]MonitorMediaModel{"omni-flash": {Parameters: map[string]any{"prompt": "custom probe"}}}}}}
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
	m := &ChannelMonitor{ID: 6, Endpoint: server.URL, APIKey: "key", PrimaryModel: "gpt-image-2", BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation"}}}
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
	cap, req, err := normalizeMonitorMediaModel(m.PrimaryModel, MonitorMediaModel{})
	require.NoError(t, err)
	raw, _ := json.Marshal([]any{m.Endpoint, m.APIKey, m.ExtraHeaders, cap, req, c.Revision})
	path := s.monitorMediaStatePath(m.ID, m.PrimaryModel)
	require.NoError(t, s.saveMonitorMediaTask(path, monitorMediaTask{Fingerprint: fmt.Sprintf("%x", sha256.Sum256(raw)), State: "submitting", Capability: cap}))
	res := s.runMonitorMediaGeneration(context.Background(), m, m.PrimaryModel, c, &http.Client{})
	require.Contains(t, res.Message, "no resubmission")
	require.NoError(t, os.WriteFile(path, []byte("corrupt"), 0600))
	res = s.runMonitorMediaGeneration(context.Background(), m, m.PrimaryModel, c, &http.Client{})
	require.Contains(t, res.Message, "journal unreadable")
}
