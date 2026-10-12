package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMediaPlatformNeverFallsBackToTextGateway(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformImageVideo)
	for _, path := range []string{"/v1/messages", "/v1/responses", "/responses", "/v1/responses/compact", "/backend-api/codex/responses", "/v1/embeddings", "/api/v3/contents/generations/tasks"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-image-2"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code, path)
		require.Contains(t, w.Body.String(), "unsupported_media_endpoint", path)
	}
}
