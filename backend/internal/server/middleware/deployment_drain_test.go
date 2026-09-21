package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDeploymentDrainRejectsNewWorkButKeepsHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	marker := filepath.Join(t.TempDir(), "drain")
	require.NoError(t, os.WriteFile(marker, nil, 0600))
	r := gin.New()
	r.Use(DeploymentDrain(marker))
	r.GET("/health", func(c *gin.Context) { c.Status(200) })
	r.GET("/v1/test", func(c *gin.Context) { t.Fatal("business handler must not run") })
	for _, tc := range []struct {
		path, remote string
		status       int
	}{
		{"/v1/test", "192.0.2.1:1234", 503},
		{"/health", "192.0.2.1:1234", 200},
		{"/internal/deployment/status", "192.0.2.1:1234", 404},
		{"/internal/deployment/status", "127.0.0.1:1234", 200},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.RemoteAddr = tc.remote
		r.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code)
	}
}

func TestDeploymentDrainCountsInFlightUntilHandlerReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(DeploymentDrain(filepath.Join(t.TempDir(), "drain")))
	entered, finish, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	r.GET("/stream", func(c *gin.Context) { close(entered); <-finish; c.Status(200) })
	go func() {
		defer close(done)
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/stream", nil))
	}()
	<-entered
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/internal/deployment/status", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	r.ServeHTTP(w, req)
	require.JSONEq(t, `{"draining":false,"active":1}`, w.Body.String())
	close(finish)
	<-done
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.JSONEq(t, `{"draining":false,"active":0}`, w.Body.String())
}
