//go:build unit

package admin

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMalformedAccountTestDoesNotStartGeneration(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	c.Request = httptest.NewRequest("POST", "/accounts/42/test", strings.NewReader(`{"test_kind":"custom","parameters":`))
	c.Request.Header.Set("Content-Type", "application/json")
	// A nil service proves malformed input is rejected before any generation.
	(&AccountHandler{}).Test(c)
	require.Equal(t, 400, w.Code)
	require.Contains(t, w.Body.String(), "Invalid test request")
}
