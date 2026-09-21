package middleware

import (
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// DeploymentDrain blocks new work only when the host explicitly creates the
// marker. Admission and drain observations share a mutex so a zero-active
// observation cannot race with a request admitted under stale marker state.
func DeploymentDrain(marker string) gin.HandlerFunc {
	var mu sync.Mutex
	var active int64
	var draining bool
	var checked time.Time
	return func(c *gin.Context) {
		if marker == "" {
			c.Next()
			return
		}
		mu.Lock()
		if time.Since(checked) > 200*time.Millisecond {
			_, err := os.Stat(marker)
			draining = err == nil || !os.IsNotExist(err)
			checked = time.Now()
		}
		if c.Request.URL.Path == "/internal/deployment/status" {
			count, paused := active, draining
			mu.Unlock()
			host, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"draining": paused, "active": count})
			return
		}
		if c.Request.URL.Path == "/health" {
			mu.Unlock()
			c.Next()
			return
		}
		if draining {
			mu.Unlock()
			c.Header("Retry-After", "30")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Maintenance in progress; retry shortly"})
			return
		}
		// This read-only dashboard socket stays open indefinitely. Business
		// WebSockets remain counted; telemetry can reconnect after maintenance.
		if c.Request.URL.Path == "/api/v1/admin/ops/ws/qps" {
			mu.Unlock()
			c.Next()
			return
		}
		active++
		mu.Unlock()
		defer func() { mu.Lock(); active--; mu.Unlock() }()
		c.Next()
	}
}
