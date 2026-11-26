package media

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const MaxSubmissionAttempts = 3

type RejectionScope string

const (
	RejectionAccount RejectionScope = "account"
	RejectionOrigin  RejectionScope = "origin"
)

type RouteOutcome string

const (
	RouteNeutral   RouteOutcome = "neutral"
	RouteRunning   RouteOutcome = "running"
	RouteFailed    RouteOutcome = "failed"
	RouteSucceeded RouteOutcome = "succeeded"
)

type RouteHealth interface {
	RetryIn(context.Context, []string) (time.Duration, error)
	Acquire(context.Context, []string, string) (time.Duration, error)
	Finish(context.Context, []string, string, bool) error
	Reject(context.Context, string, time.Duration) error
	Observe(context.Context, string, string, RouteOutcome, time.Duration) error
}

type RoutePolicy struct {
	Health RouteHealth
	groups map[int64]bool
	active bool
}

func NewRoutePolicy(mode, groupIDs string, health RouteHealth) (*RoutePolicy, error) {
	policy := &RoutePolicy{Health: health, groups: make(map[int64]bool)}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "off":
	case "enforce":
		policy.active = true
	default:
		return nil, fmt.Errorf("media failover mode must be off or enforce")
	}
	if strings.TrimSpace(groupIDs) != "" {
		for _, group := range strings.Split(groupIDs, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(group), 10, 64)
			if err != nil || id <= 0 {
				return nil, fmt.Errorf("media failover group IDs must be positive integers")
			}
			policy.groups[id] = true
		}
	}
	return policy, nil
}

func (p *RoutePolicy) Enabled(group int64) bool {
	return p != nil && p.active && (len(p.groups) == 0 || p.groups[group])
}

func RouteOrigin(baseURL, fallback string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Hostname() == "" {
		return fallback
	}
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
}

type RouteTarget struct {
	GroupID, AccountID       int64
	Origin, Model, Operation string
}

func (t RouteTarget) key(scope RejectionScope) string {
	if scope == RejectionOrigin {
		return fmt.Sprintf("media:route:origin:%x", sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s", t.GroupID, t.Origin, t.Model, t.Operation))))
	}
	return fmt.Sprintf("media:route:account:%d:%x", t.AccountID, sha256.Sum256([]byte(t.Origin)))
}

func (t RouteTarget) keys() []string {
	return []string{t.key(RejectionAccount), t.key(RejectionOrigin)}
}

func (p *RoutePolicy) RetryIn(ctx context.Context, t RouteTarget) time.Duration {
	if p == nil || p.Health == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	delay, _ := p.Health.RetryIn(ctx, t.keys())
	return delay
}

func (p *RoutePolicy) Acquire(ctx context.Context, t RouteTarget, owner string) time.Duration {
	if p == nil || p.Health == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	delay, _ := p.Health.Acquire(ctx, t.keys(), owner)
	return delay
}

func (p *RoutePolicy) Finish(ctx context.Context, t RouteTarget, owner string, scope RejectionScope, delay time.Duration, accepted bool) {
	if p == nil || p.Health == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
	defer cancel()
	if scope != "" {
		if delay <= 0 {
			delay = 30 * time.Second
		}
		_ = p.Health.Reject(ctx, t.key(scope), delay)
	}
	_ = p.Health.Finish(ctx, t.keys(), owner, accepted)
}

// Observe records one definitive outcome per attempt. A provider Retry-After
// can extend the cooldown after the account circuit opens.
func (p *RoutePolicy) Observe(ctx context.Context, t RouteTarget, owner string, outcome RouteOutcome, delay time.Duration) {
	if p == nil || p.Health == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
	defer cancel()
	delay = max(delay, time.Minute)
	_ = p.Health.Observe(ctx, t.key(RejectionAccount), owner, outcome, delay)
}
