package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"
)

const maxUpstreamAllowlistBytes = 256 << 10

var upstreamDNSLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

type UpstreamAllowlistSnapshot struct {
	Version string   `json:"version"`
	Hosts   []string `json:"hosts"`
}

type upstreamAllowlist struct {
	path    string
	current atomic.Pointer[UpstreamAllowlistSnapshot]
}

func (c *URLAllowlistConfig) loadUpstreamAllowlist() error {
	if c.UpstreamHostsFile == "" {
		return nil
	}
	if !c.Enabled {
		return errors.New("security.url_allowlist.upstream_hosts_file requires enabled URL validation")
	}
	policy := &upstreamAllowlist{path: c.UpstreamHostsFile}
	if err := policy.reload(); err != nil {
		return fmt.Errorf("initial upstream allowlist: %w", err)
	}
	c.upstreamAllowlist = policy
	return nil
}

// EffectiveUpstreamHosts returns the current read-only policy for URL validation.
func (c *URLAllowlistConfig) EffectiveUpstreamHosts() []string {
	if c.upstreamAllowlist != nil {
		return c.upstreamAllowlist.current.Load().Hosts
	}
	return c.UpstreamHosts
}

func (c *URLAllowlistConfig) UpstreamAllowlistStatus() (UpstreamAllowlistSnapshot, bool) {
	if c.upstreamAllowlist == nil {
		return UpstreamAllowlistSnapshot{}, false
	}
	snapshot := c.upstreamAllowlist.current.Load()
	return UpstreamAllowlistSnapshot{Version: snapshot.Version, Hosts: slices.Clone(snapshot.Hosts)}, true
}

func (c *URLAllowlistConfig) WatchUpstreamAllowlist(ctx context.Context) {
	policy := c.upstreamAllowlist
	if policy == nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	failed := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := policy.reload()
			if err != nil && !failed {
				slog.Warn("upstream allowlist reload failed; retaining last applied version", "error", err)
			}
			if err == nil && failed {
				slog.Info("upstream allowlist file is valid again")
			}
			failed = err != nil
		}
	}
}

func (p *upstreamAllowlist) reload() error {
	info, err := os.Lstat(p.path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxUpstreamAllowlistBytes {
		return errors.New("upstream allowlist must be a regular file within the size limit")
	}
	file, err := os.Open(p.path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxUpstreamAllowlistBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxUpstreamAllowlistBytes {
		return errors.New("upstream allowlist exceeds the size limit")
	}
	digest := sha256.Sum256(data)
	version := hex.EncodeToString(digest[:])
	if current := p.current.Load(); current != nil && current.Version == version {
		return nil
	}
	var hosts []string
	if err := json.Unmarshal(data, &hosts); err != nil || hosts == nil {
		return errors.New("upstream allowlist must be a JSON array of exact DNS hostnames")
	}
	seen := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		if !validUpstreamHostname(host) {
			return errors.New("invalid exact DNS hostname in upstream allowlist")
		}
		if _, duplicate := seen[host]; duplicate {
			return errors.New("duplicate hostname in upstream allowlist")
		}
		seen[host] = struct{}{}
	}
	p.current.Store(&UpstreamAllowlistSnapshot{Version: version, Hosts: hosts})
	return nil
}

func validUpstreamHostname(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") || net.ParseIP(host) != nil ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if !upstreamDNSLabel.MatchString(label) {
			return false
		}
	}
	return true
}
