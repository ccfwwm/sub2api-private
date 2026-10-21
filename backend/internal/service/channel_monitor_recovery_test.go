//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/stretchr/testify/require"
)

func TestMediaMonitorLostResponseRecoversAcrossRestartForEachModel(t *testing.T) {
	swapMonitorHTTPClient(t)
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	posts, recoveries, polls := 0, 0, 0
	keys := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST":
			posts++
			require.NotEmpty(t, r.Header.Get("Idempotency-Key"))
			keys[r.Header.Get("Idempotency-Key")] = true
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, true, body["async"])
			// The gateway accepted the request, but the response did not reach us.
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			conn.Close()
		case r.URL.Path == "/v1/media/tasks/recover":
			recoveries++
			require.True(t, keys[r.URL.Query().Get("idempotency_key")])
			require.Equal(t, "image", r.URL.Query().Get("operation"))
			w.Write([]byte(`{"id":"original-task","status":"queued"}`))
		default:
			polls++
			require.Equal(t, "/v1/tasks/original-task", r.URL.Path)
			w.Write([]byte(`{"id":"original-task","status":"in_progress"}`))
		}
	}))
	defer server.Close()
	models := map[string]MonitorMediaModel{}
	for _, model := range []string{"gpt-image-2.5", "gpt-image-2"} {
		entry := monitorTestMediaModel(t, model, "gpt-image-2", "image-json", nil)
		entry.Capability.Template.Mode = "async"
		entry.Capability.Template.Query = &media.TemplateEndpoint{Method: "GET", Path: "/v1/tasks/{task_id}"}
		entry.Capability.Template.Recover = &media.TemplateEndpoint{Method: "GET", Path: "/v1/media/tasks/recover", Query: map[string]string{"idempotency_key": "{idempotency_key}", "operation": "image"}}
		entry.Capability.Template.Rules = []media.ParameterRule{{Phase: "wire", Op: "fixed", Target: "async", Value: true}}
		_, _, err := normalizeMonitorMediaModel(model, entry)
		require.NoError(t, err)
		models[model] = entry
	}
	m := &ChannelMonitor{ID: 51, Endpoint: server.URL, APIKey: "test-key", PrimaryModel: "gpt-image-2.5", ExtraModels: []string{"gpt-image-2"}, BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", TaskAPI: true, Models: models}}}
	service := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	first := service.runMediaMonitor(context.Background(), m)
	require.Len(t, first, 2)
	require.Contains(t, first[0].Message, "acceptance uncertain")
	require.Contains(t, first[1].Message, "acceptance uncertain")
	require.Equal(t, 2, posts)
	require.Len(t, keys, 2)
	// An unavailable live template store and removed capabilities must not prevent
	// recovering either frozen original request after a restart.
	m.BodyOverride = map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation"}}
	service = NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	service.SetMediaTemplateStore(&media.Store{})
	service.runMediaMonitor(context.Background(), m)
	require.Equal(t, 2, recoveries)
	require.Equal(t, 2, posts)
	service.runMediaMonitor(context.Background(), m)
	require.Equal(t, 2, polls)
	require.Equal(t, 2, posts)
}

func TestMediaMonitorLegacyJournalResumesWithoutCurrentTemplate(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "lost-response", true: "accepted"}[accepted], func(t *testing.T) {
			swapMonitorHTTPClient(t)
			t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
				if !accepted && requests == 1 {
					require.Equal(t, "/v1/media/tasks/recover", r.URL.Path)
					require.Equal(t, "original-key", r.URL.Query().Get("idempotency_key"))
					require.Equal(t, "video", r.URL.Query().Get("operation"))
				} else if accepted {
					require.Equal(t, "/v1/videos/original-task", r.URL.Path)
				} else {
					require.Equal(t, "/v1/tasks/original-task", r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"id":"original-task","status":"processing"}`))
			}))
			defer server.Close()
			cap, ok := media.ModelPreset("omni-flash")
			require.True(t, ok)
			m := &ChannelMonitor{ID: 62, Endpoint: server.URL, APIKey: "test-key", PrimaryModel: cap.Model,
				BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", TaskAPI: accepted, Models: map[string]MonitorMediaModel{cap.Model: {Operation: "image", Parameters: map[string]any{"prompt": "changed settings"}}}}}}
			s := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
			task := monitorMediaTask{State: "submitting", Capability: cap, IdempotencyKey: "original-key",
				Account: media.Account{BaseURL: server.URL, Key: "test-key", TaskAPI: !accepted}}
			if accepted {
				task.State, task.UpstreamID = "in_progress", "original-task"
			}
			require.NoError(t, s.saveMonitorMediaTask(s.monitorMediaStatePath(m.ID, cap.Model), task))
			s = NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
			s.SetMediaTemplateStore(&media.Store{}) // No live database is available.
			for range 2 {
				result := s.runMediaMonitor(context.Background(), m)
				require.Len(t, result, 1)
				require.Equal(t, MonitorStatusPending, result[0].Status)
			}
			require.Equal(t, 2, requests)
			m.APIKey = "different-owner"
			result := s.runMediaMonitor(context.Background(), m)
			require.Equal(t, MonitorStatusError, result[0].Status)
			require.Contains(t, result[0].Message, "no resubmission")
			require.Equal(t, 2, requests)
		})
	}
}

func TestMediaMonitorLegacyRecoveryRequiresRecordedOrUnambiguousOperation(t *testing.T) {
	cap, ok := media.ModelPreset("gpt-image-2")
	require.True(t, ok)
	cap.Operations = []string{"image", "image_edit"}
	// No client is needed: ambiguity must stop before any network access.
	result, err := recoverMonitorMediaTask(context.Background(), &media.Adapter{}, monitorMediaTask{Capability: cap, Account: media.Account{TaskAPI: true}})
	require.NoError(t, err)
	require.Equal(t, "unknown", result.Status)
	require.Contains(t, result.Diagnostic, "Original operation was not recorded")
}

func TestMediaMonitorLegacyCompletedRecoveryQueriesOriginalResult(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)
		requests = append(requests, r.URL.Path)
		if r.URL.Path == "/v1/media/tasks/recover" {
			_, _ = w.Write([]byte(`{"id":"original-task","status":"completed"}`))
			return
		}
		require.Equal(t, "/v1/tasks/original-task", r.URL.Path)
		_, _ = w.Write([]byte(`{"id":"original-task","status":"failed"}`))
	}))
	defer server.Close()
	cap, ok := media.ModelPreset("omni-flash")
	require.True(t, ok)
	result, err := recoverMonitorMediaTask(context.Background(), &media.Adapter{Client: server.Client()}, monitorMediaTask{Capability: cap,
		Account: media.Account{BaseURL: server.URL, Key: "test-key", TaskAPI: true, IdempotencyKey: "original-key"}})
	require.NoError(t, err)
	require.Equal(t, []string{"/v1/media/tasks/recover", "/v1/tasks/original-task"}, requests)
	require.Equal(t, "original-task", result.ID)
	require.Equal(t, "failed", result.Status)
}
