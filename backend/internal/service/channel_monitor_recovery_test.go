//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	m := &ChannelMonitor{ID: 51, Endpoint: server.URL, APIKey: "test-key", PrimaryModel: "gpt-image-2.5", ExtraModels: []string{"gpt-image-2"}, BodyOverride: map[string]any{"media_monitor": MonitorMediaConfig{Mode: "generation", TaskAPI: true}}}
	service := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	require.Len(t, service.runMediaMonitor(context.Background(), m), 2)
	require.Equal(t, 2, posts)
	service = NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{})
	service.runMediaMonitor(context.Background(), m)
	require.Equal(t, 2, recoveries)
	require.Equal(t, 2, posts)
	service.runMediaMonitor(context.Background(), m)
	require.Equal(t, 2, polls)
	require.Equal(t, 2, posts)
}
