package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaRejectionScope(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   RejectionScope
	}{
		{"capacity", 429, `{"error":{"code":"fail_to_fetch_task","message":"当前分组上游负载已饱和"}}`, RejectionOrigin},
		{"ordinary quota", 429, `{"error":{"code":"rate_limit_exceeded"}}`, RejectionAccount},
		{"503 capacity", 503, `{"error":{"code":"overloaded_error"}}`, RejectionOrigin},
		{"ambiguous 503", 503, `{"error":{"message":"unavailable"}}`, ""},
		{"known credential error", 403, `{"error":{"code":"invalid_api_key"}}`, RejectionAccount},
		{"unclassified forbidden", 403, `{"error":{"message":"forbidden"}}`, ""},
		{"policy refusal", 429, `{"error":{"code":"content_policy_violation"}}`, ""},
		{"job identifier in rejection", 429, `{"job_id":"job-123","error":{"code":"overloaded"}}`, ""},
		{"artifact in rejection", 429, `{"data":[{"b64_json":"abcdef"}]}`, ""},
		{"validation failure", 400, `{"error":{"code":"invalid_size"}}`, ""},
		{"gateway timeout", http.StatusGatewayTimeout, `{}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, MediaRejectionScope(tc.status, []byte(tc.body)))
		})
	}
}

func TestMediaSubmissionDoesNotReplayUncertainAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"gateway timeout", 504, `{}`},
		{"unclassified service unavailable", 503, `{"error":{"message":"unavailable"}}`},
		{"job evidence in error response", 429, `{"job_id":"job-123","error":{"code":"overloaded"}}`},
		{"incomplete response", 200, `{"id":`},
		{"redirected submission", 307, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/videos" {
					t.Error("followed a generation redirect")
				}
				if tc.status == 307 {
					w.Header().Set("Location", "/other")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			adapter := &Adapter{Client: SafeClient(true)}
			result, err := adapter.call(context.Background(), Account{BaseURL: server.URL, Key: "fixture", RouteSubmission: true},
				Capability{MediaType: "video"}, http.MethodPost, "/v1/videos", strings.NewReader("{}"), "application/json", true)
			require.NoError(t, err)
			require.Equal(t, "unknown", result.Status)
			require.False(t, result.Rejected)
			require.Empty(t, result.Failover)
		})
	}
}
