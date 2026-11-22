package server

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpstreamAllowlistWatchKeepsStreamOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.json")
	publish := func(payload string) {
		t.Helper()
		require.NoError(t, os.WriteFile(path+".tmp", []byte(payload), 0600))
		require.NoError(t, os.Rename(path+".tmp", path))
	}
	publish(`["before.example"]`)
	t.Setenv("SECURITY_URL_ALLOWLIST_ENABLED", "true")
	t.Setenv("SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS_FILE", path)
	cfg, err := config.ProvideConfig()
	require.NoError(t, err)

	router := gin.New()
	release := make(chan struct{})
	router.GET("/stream", func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(c.Writer, "data: before\n\n")
		c.Writer.Flush()
		select {
		case <-release:
			_, _ = fmt.Fprint(c.Writer, "data: after\n\n")
			c.Writer.Flush()
		case <-c.Request.Context().Done():
		}
	})
	server := ProvideHTTPServer(cfg, router)
	httpServer := httptest.NewServer(server.Handler)
	defer httpServer.Close()

	recorder := httptest.NewRecorder()
	untrusted := httptest.NewRequest(http.MethodGet, upstreamAllowlistStatusPath, nil)
	untrusted.Header.Set("X-Forwarded-For", "127.0.0.1")
	server.Handler.ServeHTTP(recorder, untrusted)
	require.Equal(t, http.StatusNotFound, recorder.Code)

	client := httpServer.Client()
	client.Timeout = 8 * time.Second
	stream, err := client.Get(httpServer.URL + "/stream")
	require.NoError(t, err)
	defer func() { _ = stream.Body.Close() }()
	reader := bufio.NewReader(stream.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "data: before\n", line)
	_, err = reader.ReadString('\n')
	require.NoError(t, err)

	payload := `["after.example"]`
	publish(payload)
	version := fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
	require.Eventually(t, func() bool {
		snapshot, ok := cfg.Security.URLAllowlist.UpstreamAllowlistStatus()
		return ok && snapshot.Version == version
	}, 5*time.Second, 20*time.Millisecond)
	recorder = httptest.NewRecorder()
	local := httptest.NewRequest(http.MethodGet, upstreamAllowlistStatusPath, nil)
	local.RemoteAddr = "127.0.0.1:12345"
	server.Handler.ServeHTTP(recorder, local)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, fmt.Sprintf(`{"version":"%s","hosts":["after.example"]}`, version), recorder.Body.String())
	close(release)

	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "data: after\n", line)
	require.NoError(t, server.Shutdown(t.Context()))
}
