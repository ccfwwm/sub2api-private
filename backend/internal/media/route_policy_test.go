package media

import (
	"net/http"
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
