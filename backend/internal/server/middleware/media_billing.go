package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

// Configured media submits use their atomic task reservation. Authentication,
// user status, group eligibility and IP restrictions always remain enforced.
func delegatedMediaBilling(c *gin.Context, key *service.APIKey) bool {
	callback, ok := c.Get("media_billing_delegate")
	if !ok {
		return false
	}
	check, ok := callback.(func(*service.APIKey) bool)
	return ok && check(key)
}
func isMediaTaskRead(method, path string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	for _, prefix := range []string{"/v1/videos/", "/v1/tasks/", "/v1/audio/generations/"} {
		if strings.HasPrefix(path, prefix+"mt_") {
			return true
		}
	}
	return false
}
