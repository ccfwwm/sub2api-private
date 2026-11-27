//go:build unit

package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorStreamingProtocols(t *testing.T) {
	swapMonitorHTTPClient(t)
	for _, tc := range []struct {
		name, provider, apiMode, path, query, frames string
	}{
		{"responses", MonitorProviderOpenAI, MonitorAPIModeResponses, "/relay/v1/responses", "",
			"event: response.output_text.delta\r\ndata: {\"delta\":\"4\"}\r\n\r\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"2\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"},
		{"chat", MonitorProviderDeepseek, "", "/relay/v1/chat/completions", "",
			"data: {\"choices\":[{\"delta\":{\"content\":\"4\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"2\"}}]}\n\ndata: [DONE]\n\n"},
		{"messages", MonitorProviderAnthropic, "", "/relay/v1/messages", "",
			"event: content_block_delta\ndata: {\"delta\":{\"type\":\"text_delta\",\"text\":\"4\"}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"2\"}}\n\nevent: message_stop\ndata: {}\n\n"},
		{"gemini", MonitorProviderGemini, "", "/relay/v1beta/models/test-model:streamGenerateContent", "alt=sse",
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"internal\",\"thought\":true},{\"text\":\"4\"}]}}]}\n\ndata: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"2\"}]},\"finishReason\":\"STOP\"}]}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.Method != http.MethodPost || r.URL.Path != tc.path || r.URL.RawQuery != tc.query || r.Header.Get("Accept") != "text/event-stream" {
					t.Errorf("unexpected probe request %s %s Accept=%q", r.Method, r.URL, r.Header.Get("Accept"))
				}
				if stream, exists := body["stream"]; tc.provider == MonitorProviderGemini && exists || tc.provider != MonitorProviderGemini && stream != true {
					t.Errorf("unexpected stream field: %v", stream)
				}
				w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
				_, _ = fmt.Fprint(w, ": keepalive\n\n"+tc.frames)
				w.(http.Flusher).Flush()
				// The probe should return and close even if the provider keeps the connection open.
				<-r.Context().Done()
			}))
			defer server.Close()
			base := server.URL + "/relay"
			if tc.provider == MonitorProviderGemini {
				base += "/v1beta"
			}
			text, _, status, err := callProvider(t.Context(), tc.provider, base, "fixture", "test-model", "prompt", &CheckOptions{
				APIMode: tc.apiMode, ExtraHeaders: map[string]string{"accept": "application/json"},
				ProbeConfig: domain.MonitorProbeConfig{RequestMode: MonitorRequestModeStream},
			})
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, "42", text)
		})
	}
}

func TestChannelMonitorResponsesStreamingAndSyncHealth(t *testing.T) {
	swapMonitorHTTPClient(t)
	const delta = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"
	const complete = "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"
	for _, tc := range []struct {
		name, mode, contentType, body, status, message string
		httpStatus                                     int
	}{
		{"stream-only upstream rejects sync", MonitorRequestModeSync, "application/json", `{"error":{"message":"stream is required"}}`, MonitorStatusError, "upstream HTTP 400", 400},
		{"successful stream", MonitorRequestModeStream, "text/event-stream", delta + complete, MonitorStatusOperational, "", 200},
		{"missing terminal event", MonitorRequestModeStream, "text/event-stream", delta, MonitorStatusError, "ended before completion", 200},
		{"failure after output", MonitorRequestModeStream, "text/event-stream", delta + "data: {\"type\":\"response.failed\"}\n\n", MonitorStatusError, "response.failed", 200},
		{"empty completion", MonitorRequestModeStream, "text/event-stream", complete, MonitorStatusFailed, "empty text", 200},
		{"unexpected JSON", MonitorRequestModeStream, "application/json", `{"output_text":"ok"}`, MonitorStatusError, "expected an SSE response", 200},
		{"oversized stream", MonitorRequestModeStream, "text/event-stream", ":" + strings.Repeat(" ", monitorStreamMaxBytes) + "\n\n" + delta + complete, MonitorStatusError, "size limit", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["stream"] != (tc.mode == MonitorRequestModeStream) {
					t.Errorf("explicit request mode did not take precedence: %v", body["stream"])
				}
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.httpStatus)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			// A replace body can contain an older stream field from a request template.
			opts := &CheckOptions{
				APIMode: MonitorAPIModeResponses, BodyOverrideMode: MonitorBodyOverrideModeReplace,
				BodyOverride: map[string]any{"model": "model", "input": "respond ok", "instructions": "respond briefly", "stream": tc.mode != MonitorRequestModeStream},
				ProbeConfig:  domain.MonitorProbeConfig{RequestMode: tc.mode},
			}
			result := runCheckForModel(t.Context(), MonitorProviderOpenAI, server.URL, "fixture", "model", opts)
			require.Equal(t, tc.status, result.Status, result.Message)
			require.Contains(t, result.Message, tc.message)
		})
	}
}

func TestChannelMonitorStreamingWaitIsBoundedByProbeTimeout(t *testing.T) {
	swapMonitorHTTPClient(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, ": keepalive\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	timeout := 1000
	opts := &CheckOptions{ProbeConfig: domain.MonitorProbeConfig{
		RequestMode: MonitorRequestModeStream,
		Defaults:    domain.MonitorProbePolicy{RequestTimeoutMs: &timeout},
	}}
	start := time.Now()
	result := runCheckForModel(t.Context(), MonitorProviderOpenAI, server.URL, "fixture", "model", opts)
	require.Equal(t, MonitorStatusError, result.Status)
	require.Less(t, time.Since(start), 5*time.Second)
}
