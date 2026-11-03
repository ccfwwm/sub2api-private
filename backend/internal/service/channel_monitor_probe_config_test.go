//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCallImageProviderBuildsProviderRequestsAndDetectsImages(t *testing.T) {
	swapMonitorProbeClients(t)
	tests := []struct {
		name       string
		provider   string
		model      string
		wantPath   string
		wantHeader string
		response   string
	}{
		{
			name: "openai", provider: MonitorProviderOpenAI, model: "gpt-image-2",
			wantPath: "/v1/images/generations", wantHeader: "Bearer probe-key",
			response: `{"data":[{"b64_json":"aGVsbG8="}]}`,
		},
		{
			name: "grok", provider: MonitorProviderGrok, model: "grok-imagine-image",
			wantPath: "/v1/images/generations", wantHeader: "Bearer probe-key",
			response: `{"data":[{"url":"https://images.example/image.png"}]}`,
		},
		{
			name: "gemini", provider: MonitorProviderGemini, model: "gemini-2.5-flash-image",
			wantPath: "/v1beta/models/gemini-2.5-flash-image:generateContent", wantHeader: "probe-key",
			response: `{"candidates":[{"content":{"parts":[{"inlineData":{"data":"aGVsbG8="}}]}}]}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			var gotHeader string
			var gotBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				if tc.provider == MonitorProviderGemini {
					gotHeader = r.Header.Get("x-goog-api-key")
				} else {
					gotHeader = r.Header.Get("Authorization")
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			got, raw, status, err := callImageProvider(context.Background(), tc.provider, server.URL, "probe-key", tc.model, "draw a circle", nil)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, "image", got)
			require.Equal(t, tc.response, raw)
			require.Equal(t, tc.wantPath, gotPath)
			require.Equal(t, tc.wantHeader, gotHeader)
			if tc.provider == MonitorProviderGemini {
				require.Equal(t, []any{"IMAGE"}, gotBody["generationConfig"].(map[string]any)["responseModalities"])
			} else {
				require.Equal(t, tc.model, gotBody["model"])
				require.Equal(t, "draw a circle", gotBody["prompt"])
			}
		})
	}
}

func TestRunCheckForModel_ImageAndCustomProbeResults(t *testing.T) {
	swapMonitorProbeClients(t)
	t.Run("image response must contain image data", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":[]}`))
		}))
		defer server.Close()
		res := runCheckForModel(context.Background(), MonitorProviderOpenAI, server.URL, "key", "image-model", &CheckOptions{ProbeType: MonitorProbeTypeImage})
		require.Equal(t, MonitorStatusFailed, res.Status)
		require.Contains(t, res.Message, "without an image")
	})

	t.Run("custom extracted field can be empty when configured", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, "/tenant/v1/health", r.URL.Path)
			require.Equal(t, "custom-secret", r.Header.Get("X-Probe-Key"))
			_, _ = w.Write([]byte(`{"data":{"status":""}}`))
		}))
		defer server.Close()
		res := runCheckForModel(context.Background(), MonitorProviderOpenAI, server.URL+"/tenant", "key", "ignored", &CheckOptions{
			ProbeType:                     MonitorProbeTypeCustom,
			CustomMethod:                  http.MethodGet,
			CustomPath:                    "/v1/health",
			CustomResponseJSONPath:        "data.status",
			CustomResponseRequireNonEmpty: false,
			ExtraHeaders:                  map[string]string{"X-Probe-Key": "custom-secret"},
		})
		require.Equal(t, MonitorStatusOperational, res.Status)
	})

	t.Run("custom non-2xx remains an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad request"}`))
		}))
		defer server.Close()
		res := runCheckForModel(context.Background(), MonitorProviderOpenAI, server.URL, "key", "ignored", &CheckOptions{
			ProbeType:              MonitorProbeTypeCustom,
			CustomMethod:           http.MethodPost,
			CustomPath:             "/health",
			CustomResponseJSONPath: "error",
		})
		require.Equal(t, MonitorStatusError, res.Status)
		require.Contains(t, res.Message, "upstream HTTP 400")
	})
}

func TestValidateCustomProbePathRejectsUnsafeTargets(t *testing.T) {
	for _, path := range []string{"https://example.com/health", "//example.com/health", "/health?key=secret", "/health#fragment", "/a/../health", "/%2e%2e/health", ""} {
		require.Error(t, validateCustomProbePath(path), path)
	}
	require.NoError(t, validateCustomProbePath("/v1/health"))
	require.ErrorIs(t, validateEndpoint("https://127.0.0.1/base"), ErrChannelMonitorEndpointPrivate)
}

func TestMonitorPeriodicProbeRequiresOptInForImageAndCustom(t *testing.T) {
	for _, tc := range []struct {
		name    string
		monitor *ChannelMonitor
		want    bool
	}{
		{name: "legacy text", monitor: &ChannelMonitor{ProbeType: "", CheckMode: MonitorCheckModeProbe}, want: true},
		{name: "image disabled", monitor: &ChannelMonitor{ProbeType: MonitorProbeTypeImage, CheckMode: MonitorCheckModeProbe}, want: false},
		{name: "image enabled", monitor: &ChannelMonitor{ProbeType: MonitorProbeTypeImage, ImageProbeScheduleEnabled: true, CheckMode: MonitorCheckModeProbe}, want: true},
		{name: "custom disabled", monitor: &ChannelMonitor{ProbeType: MonitorProbeTypeCustom, CheckMode: MonitorCheckModeProbe}, want: false},
		{name: "custom enabled", monitor: &ChannelMonitor{ProbeType: MonitorProbeTypeCustom, ImageProbeScheduleEnabled: true, CheckMode: MonitorCheckModeProbe}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, monitorPeriodicProbeAllowed(tc.monitor))
		})
	}
}

func TestRunCheckForModel_CustomProbeRequiresNonEmptyWhenConfigured(t *testing.T) {
	swapMonitorProbeClients(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":""}`))
	}))
	defer server.Close()
	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, server.URL, "key", "ignored", &CheckOptions{
		ProbeType:                     MonitorProbeTypeCustom,
		CustomMethod:                  http.MethodPost,
		CustomPath:                    "/health",
		CustomResponseJSONPath:        "status",
		CustomResponseRequireNonEmpty: true,
	})
	require.Equal(t, MonitorStatusFailed, res.Status)
	require.True(t, strings.Contains(res.Message, "empty extracted response"))
}

func swapMonitorProbeClients(t *testing.T) {
	t.Helper()
	swapMonitorHTTPClient(t)
	original := monitorImageHTTPClient
	monitorImageHTTPClient = &http.Client{Timeout: monitorImageRequestTimeout}
	t.Cleanup(func() { monitorImageHTTPClient = original })
}

func TestMonitorProbeManualAndScheduledExecution(t *testing.T) {
	swapMonitorProbeClients(t)
	originalPing := monitorPingHTTPClient
	monitorPingHTTPClient = &http.Client{Timeout: time.Second}
	t.Cleanup(func() { monitorPingHTTPClient = originalPing })
	for _, probeType := range []string{MonitorProbeTypeImage, MonitorProbeTypeCustom} {
		t.Run(probeType, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					return
				}
				calls.Add(1)
				_, _ = w.Write([]byte(`{"data":[{"b64_json":"aGVsbG8="}],"status":"ok"}`))
			}))
			defer server.Close()
			repo := &quotaModeRepoStub{monitor: &ChannelMonitor{
				ID: 77, Provider: MonitorProviderOpenAI, APIKey: "OLD:key", Endpoint: server.URL,
				PrimaryModel: "image-model", Enabled: true, IntervalSeconds: 60, CheckMode: MonitorCheckModeProbe,
				ProbeType: probeType, CustomMethod: http.MethodPost, CustomPath: "/health", CustomResponseJSONPath: "status",
				CustomResponseRequireNonEmpty: true,
			}}
			if probeType == MonitorProbeTypeCustom {
				repo.monitor.ExtraModels = []string{"unused-model"}
			}
			svc := newQuotaModeService(repo)
			results, err := svc.RunScheduledCheck(context.Background(), 77)
			require.NoError(t, err)
			require.Empty(t, results)
			require.Zero(t, calls.Load())
			require.Empty(t, repo.history)
			results, err = svc.RunCheck(context.Background(), 77)
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Equal(t, MonitorStatusOperational, results[0].Status)
			require.EqualValues(t, 1, calls.Load())
			repo.monitor.ImageProbeScheduleEnabled = true
			results, err = svc.RunScheduledCheck(context.Background(), 77)
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.EqualValues(t, 2, calls.Load())
			require.Len(t, repo.history, 2)
		})
	}
}

func TestScheduleImageAndCustomProbeRequiresOptIn(t *testing.T) {
	for _, probeType := range []string{MonitorProbeTypeImage, MonitorProbeTypeCustom} {
		t.Run(probeType, func(t *testing.T) {
			svc := &stubMonitorSvc{runCalled: make(chan int64, 4)}
			runner := newRunnerForTest(svc)
			runner.Start()
			defer runner.Stop()
			monitor := &ChannelMonitor{ID: 77, Enabled: true, IntervalSeconds: 60, ProbeType: probeType}
			runner.Schedule(monitor)
			require.Zero(t, runnerTaskCount(runner))
			require.Zero(t, svc.runCount.Load())
			monitor.ImageProbeScheduleEnabled = true
			runner.Schedule(monitor)
			require.Equal(t, 1, runnerTaskCount(runner))
			select {
			case <-svc.runCalled:
			case <-time.After(2 * time.Second):
				t.Fatal("explicit opt-in did not schedule probe")
			}
			monitor.ImageProbeScheduleEnabled = false
			runner.Schedule(monitor)
			require.Zero(t, runnerTaskCount(runner))
		})
	}
}

func TestCustomProbeMethodBodyAndExtraction(t *testing.T) {
	swapMonitorProbeClients(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, method, r.Method)
				require.Equal(t, "Custom probe-token", r.Header.Get("Authorization"))
				require.Equal(t, "application/json", r.Header.Get("Content-Type"))
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, "hello", body["prompt"])
				_, _ = w.Write([]byte(`{"data":[{"text":"ready"}]}`))
			}))
			defer server.Close()
			text, _, status, err := callCustomProvider(context.Background(), server.URL, "key", &CheckOptions{
				CustomMethod: method, CustomPath: "/health", CustomBody: map[string]any{"prompt": "hello"}, CustomResponseJSONPath: "data.0.text",
				ExtraHeaders: map[string]string{"authorization": "Custom probe-token"},
			})
			require.NoError(t, err)
			require.Equal(t, 200, status)
			require.Equal(t, "ready", text)
		})
	}
	for _, response := range []string{`{"data":[]}`, `{"data":{}}`, `{"data":null}`, `{}`, `{"data":"  "}`} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(response)) }))
			defer server.Close()
			result := runCheckForModel(context.Background(), MonitorProviderOpenAI, server.URL, "key", "custom", &CheckOptions{
				ProbeType: MonitorProbeTypeCustom, CustomPath: "/health", CustomResponseJSONPath: "data", CustomResponseRequireNonEmpty: true,
			})
			require.Equal(t, MonitorStatusFailed, result.Status)
		})
	}
}

func TestImageProbeResponseValidation(t *testing.T) {
	for _, tc := range []struct {
		provider, response string
		want               bool
	}{
		{MonitorProviderOpenAI, `{"data":[{"url":true}]}`, false},
		{MonitorProviderGrok, `{"data":[{"b64_json":" "}]}`, false},
		{MonitorProviderOpenAI, `{"data":[{"url":"https://example.com/img"}]}`, true},
		{MonitorProviderOpenAI, `{"data":[{"url":"x"}]`, false},
		{MonitorProviderGemini, `{"candidates":[{"content":{"parts":[{"inlineData":{"data":"abc","mimeType":"audio/mp3"}}]}}]}`, false},
		{MonitorProviderGemini, `{"candidates":[{"content":{"parts":[{"text":"no"}]}},{"content":{"parts":[{"fileData":{"fileUri":"gs://image","mimeType":"image/png"}}]}}]}`, true},
	} {
		require.Equal(t, tc.want, imageProbeResponseHasImage(tc.provider, []byte(tc.response)), tc.response)
	}
}

func TestCustomAndImageProbeClientsBlockPrivateDialTargets(t *testing.T) {
	for _, timeout := range []time.Duration{monitorRequestTimeout, monitorImageRequestTimeout} {
		client := newSSRFSafeHTTPClient(timeout)
		for _, target := range []string{"https://127.0.0.1/health", "https://10.0.0.1/health", "https://[::1]/health"} {
			_, err := client.Get(target)
			require.Error(t, err)
			require.Contains(t, err.Error(), "blocked by SSRF policy")
		}
	}
}

func TestImageProbeUpstreamErrors(t *testing.T) {
	swapMonitorProbeClients(t)
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"provider rejected request"}}`))
		}))
		result := runCheckForModel(context.Background(), MonitorProviderOpenAI, server.URL, "key", "image-model", &CheckOptions{ProbeType: MonitorProbeTypeImage})
		server.Close()
		require.Equal(t, MonitorStatusError, result.Status)
		require.Contains(t, result.Message, "provider rejected request")
	}
}

func TestCustomProbeInvalidJSONAndPlainTextErrors(t *testing.T) {
	swapMonitorProbeClients(t)
	for _, tc := range []struct {
		status        int
		body, message string
	}{
		{http.StatusOK, `{"data":`, "not valid JSON"},
		{http.StatusServiceUnavailable, "provider is unavailable", "upstream HTTP 503: provider is unavailable"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		result := runCheckForModel(context.Background(), MonitorProviderOpenAI, server.URL, "key", "custom", &CheckOptions{
			ProbeType: MonitorProbeTypeCustom, CustomPath: "/health", CustomResponseJSONPath: "data.text", CustomResponseRequireNonEmpty: true,
		})
		server.Close()
		require.Equal(t, MonitorStatusError, result.Status)
		require.Contains(t, result.Message, tc.message)
	}
}
