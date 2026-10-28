package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.White)
	require.NoError(t, png.Encode(&b, im))
	return b.Bytes()
}

func TestAdapterClassifiesWrappedUpstream429AndRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "13")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"code":"service_busy","message":"upstream rejected"},"upstream_status_code":429}`))
	}))
	defer server.Close()

	result, err := (&Adapter{SafeClient(true)}).Submit(context.Background(), Capability{MediaType: "video"},
		Request{Operation: "video", Parameters: map[string]any{"model": "mock"}}, Account{BaseURL: server.URL, Key: "secret"})
	require.NoError(t, err)
	require.Equal(t, "failed", result.Status)
	require.True(t, result.Rejected)
	require.True(t, result.Retryable)
	require.Equal(t, 13*time.Second, result.RetryAfter)
	require.Contains(t, result.Diagnostic, "HTTP 429")
}

func TestAdapterPreservesSanitizedUpstreamErrorMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "provider-req-42")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_media","message":"Bearer secret-provider-key: invalid input"}}`))
	}))
	defer server.Close()

	result, err := (&Adapter{SafeClient(true)}).Submit(context.Background(), Capability{MediaType: "video"},
		Request{Operation: "video", Parameters: map[string]any{"model": "mock"}}, Account{BaseURL: server.URL, Key: "secret-provider-key"})
	require.NoError(t, err)
	require.Equal(t, "invalid_media", result.UpstreamCode)
	require.Equal(t, "[credential omitted] invalid input", result.UpstreamMessage)
	require.Equal(t, "provider-req-42", result.UpstreamRequestID)
	require.Equal(t, http.StatusBadRequest, result.UpstreamStatusCode)
	require.NotContains(t, result.Diagnostic, "secret-provider-key")
}

func TestAdapterOrdinary502IsAcceptanceUncertain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"gateway failure"}}`))
	}))
	defer server.Close()

	result, err := (&Adapter{SafeClient(true)}).Submit(context.Background(), Capability{MediaType: "video"},
		Request{Operation: "video", Parameters: map[string]any{"model": "mock"}}, Account{BaseURL: server.URL, Key: "secret"})
	require.NoError(t, err)
	require.Equal(t, "unknown", result.Status)
	require.False(t, result.Rejected)
	require.False(t, result.Retryable)
}
func TestAdapterAcceptanceAndUnknownResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body, state string
		code              int
	}{
		{"html", "<html>interstitial</html>", "unknown", 200},
		{"missing_id", `{"status":"queued"}`, "unknown", 202},
		{"failed", `{"error":{"message":"rejected"}}`, "failed", 400},
		{"service_unavailable", `{}`, "failed", 503},
		{"gateway_timeout", `{}`, "unknown", 504},
		{"rate_limited", `{}`, "failed", 429},
		{"accepted", `{"id":"remote","status":"queued"}`, "in_progress", 202},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count++
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			a := Adapter{SafeClient(true)}
			res, err := a.Submit(context.Background(), Capability{MediaType: "video"}, Request{Operation: "video", Parameters: map[string]any{"model": "mock"}}, Account{BaseURL: server.URL, Key: "secret"})
			require.NoError(t, err)
			require.Equal(t, tc.state, res.Status)
			require.Equal(t, 1, count)
			if tc.name == "missing_id" {
				require.False(t, res.Accepted)
			}
		})
	}
}
func TestEscapedModelAndTaskIDsCannotChangeOrigin(t *testing.T) {
	target, err := originURL(Account{BaseURL: "https://example.com/prefix"}, "/v1/videos/doubao%2F65Z")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/prefix/v1/videos/doubao%2F65Z", target)
}

func TestArkAdapterUsesNativeSubmissionQueryAndArtifacts(t *testing.T) {
	for _, prefix := range []string{"", "/api/v3", "/proxy/v3"} {
		t.Run(prefix, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				expected := prefix + "/contents/generations/tasks"
				if prefix == "" {
					expected = "/api/v3/contents/generations/tasks"
				}
				if r.Method == http.MethodPost {
					require.Equal(t, expected, r.URL.Path)
					var payload map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
					require.Equal(t, float64(30), payload["duration"])
					require.NotContains(t, payload, "seconds")
					require.NotContains(t, payload, "metadata")
					items, ok := payload["content"].([]any)
					require.True(t, ok)
					require.Len(t, items, 3)
					first, firstOK := items[1].(map[string]any)
					last, lastOK := items[2].(map[string]any)
					require.True(t, firstOK && lastOK)
					require.Equal(t, "first_frame", first["role"])
					require.Equal(t, "last_frame", last["role"])
					_, _ = w.Write([]byte(`{"id":"task-123","status":"queued"}`))
				} else {
					require.Equal(t, expected+"/task-123", r.URL.Path)
					_, _ = w.Write([]byte(`{"id":"task-123","status":"succeeded","content":{"video_url":"https://example.com/video.mp4"},"usage":{"total_tokens":123}}`))
				}
			}))
			defer server.Close()
			adapter := Adapter{SafeClient(true)}
			cap := Capability{Adapter: "ark_native", MediaType: "video", UpstreamModel: "ark-model", UsageFields: map[string]string{"provider_tokens.total_tokens": "usage.total_tokens"}}
			account := Account{BaseURL: server.URL + prefix, Key: "isolated"}
			request := Request{Operation: "video", Duration: "30", Parameters: map[string]any{"prompt": "Valley", "seconds": 30, "metadata": map[string]any{"content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}, "role": "first_frame"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/b.png"}, "role": "last_frame"},
			}}}}
			result, err := adapter.Submit(context.Background(), cap, request, account)
			require.NoError(t, err)
			require.True(t, result.Accepted)
			result, err = adapter.Poll(context.Background(), StoredTask{UpstreamID: result.ID, Snapshot: Snapshot{Capability: cap}}, account)
			require.NoError(t, err)
			require.Equal(t, "completed", result.Status)
			require.Len(t, result.Artifacts, 1)
			require.Equal(t, "https://example.com/video.mp4", result.Artifacts[0].Source)
			require.Equal(t, []string{"123"}, result.Quantities["provider_tokens.total_tokens"])
			require.Equal(t, 2, calls)
		})
	}
}
func TestArtifactIntegrityAndRedirectCredentials(t *testing.T) {
	b, err := NewBlobStore(t.TempDir(), strings.Repeat("s", 32))
	require.NoError(t, err)
	secret, err := b.Seal([]byte("payload"))
	require.NoError(t, err)
	decoded, err := b.Open(secret)
	require.NoError(t, err)
	require.Equal(t, []byte("payload"), decoded)
	secret[len(secret)-1] ^= 1
	_, err = b.Open(secret)
	require.Error(t, err)
	_, err = b.Path(NewTaskID(), "../escape")
	require.Error(t, err)
	good := tinyPNG(t)
	path, err := b.Path(NewTaskID(), "image")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, good, 0600))
	a := Artifact{Type: "image"}
	require.NoError(t, validateArtifact(context.Background(), path, &a))
	require.Equal(t, "image/png", a.MIME)
	require.NoError(t, os.WriteFile(path, good[:len(good)-10], 0600))
	require.Error(t, validateArtifact(context.Background(), path, &a))
	dst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(good)
	}))
	defer dst.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		http.Redirect(w, r, dst.URL, http.StatusFound)
	}))
	defer src.Close()
	task := StoredTask{ID: NewTaskID()}
	_, _, _, err = b.Download(context.Background(), SafeClient(true), task, Artifact{Key: "img", Type: "image", Source: src.URL}, Account{BaseURL: src.URL, Key: "secret"}, 1024)
	require.NoError(t, err)
	parsed := parseResult(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(good)}}}, Capability{MediaType: "image"})
	require.Len(t, parsed.Artifacts, 1)
	raw, _ := json.Marshal(parsed.Artifacts[0])
	require.NotContains(t, string(raw), "base64")
}
