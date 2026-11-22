package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/stretchr/testify/require"
)

func TestUpstreamAllowlistReplacesPolicyAndRetainsLastGood(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.json")
	write := func(payload string) {
		t.Helper()
		staged := path + ".tmp"
		require.NoError(t, os.WriteFile(staged, []byte(payload), 0600))
		require.NoError(t, os.Rename(staged, path))
	}
	cfg := URLAllowlistConfig{Enabled: true, UpstreamHostsFile: path, UpstreamHosts: []string{"static.example"}}
	write(`["old.example"]`)
	require.NoError(t, cfg.loadUpstreamAllowlist())
	checkHost := func(host string, allowed bool) {
		t.Helper()
		_, err := urlvalidator.ValidateHTTPSURL("https://"+host, urlvalidator.ValidationOptions{
			AllowedHosts: cfg.EffectiveUpstreamHosts(), RequireAllowlist: true,
		})
		require.Equal(t, allowed, err == nil)
	}
	checkHost("old.example", true)
	checkHost("new.example", false)
	checkHost("static.example", false)

	write(`["new.example"]`)
	require.NoError(t, cfg.upstreamAllowlist.reload())
	current, ok := cfg.UpstreamAllowlistStatus()
	require.True(t, ok)
	checkHost("old.example", false)
	checkHost("new.example", true)

	for _, invalid := range []string{`null`, `["new.example"`, `["*.example"]`, `["127.0.0.1"]`, `["new.example","new.example"]`, `["https://new.example"]`} {
		write(invalid)
		require.Error(t, cfg.upstreamAllowlist.reload())
		got, _ := cfg.UpstreamAllowlistStatus()
		require.Equal(t, current, got)
	}
	require.NoError(t, os.Remove(path))
	require.Error(t, cfg.upstreamAllowlist.reload())
	checkHost("new.example", true)
	write(`[]`)
	require.NoError(t, cfg.upstreamAllowlist.reload())
	checkHost("new.example", false)
}
