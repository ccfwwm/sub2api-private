package server

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const upstreamAllowlistStatusPath = "/internal/upstream-allowlist/status"

func withUpstreamAllowlistStatus(next http.Handler, cfg *config.URLAllowlistConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != upstreamAllowlistStatusPath {
			next.ServeHTTP(w, r)
			return
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !net.ParseIP(host).IsLoopback() || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		snapshot, ok := cfg.UpstreamAllowlistStatus()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot)
	})
}
