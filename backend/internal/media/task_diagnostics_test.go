package media

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaResultDiagnosticIncludesSanitizedUpstreamMetadata(t *testing.T) {
	result := UpstreamResult{
		Diagnostic:         "upstream_rate_limit: HTTP 429",
		UpstreamCode:       "rate_limit",
		UpstreamMessage:    "Bearer provider-secret retry later",
		UpstreamRequestID:  "provider-request-7",
		UpstreamStatusCode: 429,
	}
	diagnostic := mediaResultDiagnostic(result)
	require.Contains(t, diagnostic, "upstream_code=rate_limit")
	require.Contains(t, diagnostic, "upstream_request_id=provider-request-7")
	require.Contains(t, diagnostic, "upstream_status_code=429")
	require.NotContains(t, diagnostic, "provider-secret")
}
