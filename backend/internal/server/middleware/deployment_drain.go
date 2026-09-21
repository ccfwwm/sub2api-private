package middleware

import (
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// DeploymentDrain blocks new work only when the host controller explicitly
// creates its marker. Every admitted HTTP/SSE/WebSocket handler stays counted
// until it returns. Status is accessible only from within the container.
func DeploymentDrain(marker string) gin.HandlerFunc {
	var active atomic.Int64
	var draining atomic.Bool
	var checked atomic.Int64
	return func(c *gin.Context) {
		if marker == "" {
			c.Next()
			return
		}
		now := time.Now().UnixMilli()
		previous := checked.Load()
		if now-previous > 200 && checked.CompareAndSwap(previous, now) {
			_, err := os.Stat(marker)
			draining.Store(err == nil || !os.IsNotExist(err))
		}
		if c.Request.URL.Path == "/internal/deployment/status" {
			host, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"draining": draining.Load(), "active": active.Load()})
			return
		}
		if c.Request.URL.Path == "/health" {
			c.Next()
			return
		}
		if draining.Load() {
			c.Header("Retry-After", "30")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Maintenance in progress; retry shortly"})
			return
		}
		// The admin dashboard keeps a read-only telemetry socket open forever.
		// It may reconnect after maintenance and must not prevent business drain.
		if c.Request.URL.Path == "/api/v1/admin/ops/ws/qps" {
			c.Next()
			return
		}
		active.Add(1)
		defer active.Add(-1)
		c.Next()
	}
}
