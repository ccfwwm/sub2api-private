package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMediaErrorWritesStructuredAccountConcurrency429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/videos", nil)
	mediaError(c, &media.Error{
		Status:     429,
		Code:       "account_concurrency_exhausted",
		Message:    "Account media concurrency exhausted",
		RetryAfter: 1,
		Details: map[string]any{
			"account_id":          int64(2114),
			"account_concurrency": 100,
			"in_flight":           100,
			"queued":              96,
			"manual_review":       3,
		},
	})

	require.Equal(t, 429, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	require.Contains(t, w.Body.String(), `"type":"account_concurrency_exhausted"`)
	require.Contains(t, w.Body.String(), `"code":"account_concurrency_exhausted"`)
	require.Contains(t, w.Body.String(), `"account_concurrency":100`)
	require.Contains(t, w.Body.String(), `"manual_review":3`)
	value, ok := c.Get(opsMediaAccountConcurrencyKey)
	require.True(t, ok)
	require.Equal(t, 100, value)
}
