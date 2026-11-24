package service

import (
	"context"
	"errors"
	"hash/fnv"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	TextRouteMaxAttempts     = 3
	textRouteFailureWindow   = 90 * time.Second
	textRouteInitialCooldown = time.Minute
	textRouteMaxCooldown     = 5 * time.Minute
	textRouteMaxEntries      = 4096
)

var ErrTextRouteAttemptBudget = errors.New("text inference upstream attempt budget exhausted")

type textRouteKey struct {
	accountID int64
	model     string
	endpoint  string
}

type textRouteHealth struct {
	firstFailure time.Time
	failures     int
	until        time.Time
	cooldown     time.Duration
	probe        uint64
}

// TextRoutePolicy stores the short-lived health of model routes shared by text
// gateway handlers. Admission and recovery are serialized with observations.
type textRouteHealthPolicy struct {
	mu        sync.Mutex
	entries   map[textRouteKey]textRouteHealth
	nextProbe uint64
}

var textRoutePolicy = textRouteHealthPolicy{entries: make(map[textRouteKey]textRouteHealth)}
var observedTextRoutePolicy = textRouteHealthPolicy{entries: make(map[textRouteKey]textRouteHealth)}

type textRouteContextKey struct{}
type preferIndependentTextRouteKey struct{}

type textRouteRequest struct {
	endpoint        string
	enforce         bool
	pinned          bool
	sends           atomic.Int32
	failedOrigin    string
	failedAccount   int64
	coolingAccounts map[int64]bool
	sticky          map[string]textStickyWrite
}

func WithTextRoutePolicy(ctx context.Context, mode string, percent int, session, endpoint string, pinned bool) context.Context {
	if !IsTextInference(ctx) || (mode != "observe" && mode != "enforce") {
		return ctx
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(session))
	enforce := mode == "enforce" && int(hash.Sum32()%100) < percent
	return context.WithValue(ctx, textRouteContextKey{}, &textRouteRequest{
		endpoint: endpoint,
		enforce:  enforce,
		pinned:   pinned,
	})
}

func textRouteFromContext(ctx context.Context) *textRouteRequest {
	if !IsTextInference(ctx) {
		return nil
	}
	route, _ := ctx.Value(textRouteContextKey{}).(*textRouteRequest)
	return route
}

func TextRouteEnforced(ctx context.Context) bool {
	route := textRouteFromContext(ctx)
	return route != nil && route.enforce
}

// ClaimTextRouteSend is called where the HTTP request is actually dispatched,
// so provider-specific retries also consume the shared budget.
func ClaimTextRouteSend(ctx context.Context) bool {
	route := textRouteFromContext(ctx)
	if route == nil {
		return true
	}
	if !route.enforce {
		route.sends.Add(1)
		return true
	}
	for count := route.sends.Load(); count < TextRouteMaxAttempts; count = route.sends.Load() {
		if route.sends.CompareAndSwap(count, count+1) {
			return true
		}
	}
	return false
}

func TextRouteAllowsReplay(ctx context.Context) bool {
	route := textRouteFromContext(ctx)
	return route == nil || !route.enforce || !route.pinned && route.sends.Load() < TextRouteMaxAttempts
}

func (r *textRouteRequest) key(account *Account, model string) textRouteKey {
	if account.IsOpenAICompatible() {
		if r.endpoint == "/v1/responses/compact" && account.IsOpenAI() {
			model = resolveOpenAIAccountUpstreamModelForRequest(account, model, true)
		} else {
			model = canonicalOpenAIAccountSchedulingModel(account, model)
		}
	} else {
		model = account.GetMappedModel(model)
	}
	return textRouteKey{accountID: account.ID, model: strings.ToLower(strings.TrimSpace(model)), endpoint: r.protocol(account, model)}
}

func (r *textRouteRequest) protocol(account *Account, model string) string {
	inbound := r.endpoint
	switch r.endpoint {
	case "/v1/messages":
		inbound = APIProtocolAnthropic
	case "/v1/chat/completions":
		inbound = APIProtocolChatCompletions
	case "/v1/responses/compact":
		return r.endpoint
	case "/v1/responses":
		inbound = APIProtocolResponses
	}
	if account.IsOpenAICompatible() && account.Platform != PlatformGrok {
		return resolveUpstreamProtocol(account, inbound, model, nil)
	}
	return account.Platform
}

func (p *textRouteHealthPolicy) blocked(key textRouteKey, now time.Time, pinned bool) bool {
	if pinned {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	entry := p.entries[key]
	return entry.probe != 0 || now.Before(entry.until)
}

func textRouteBlocked(ctx context.Context, account *Account, model string) bool {
	route := textRouteFromContext(ctx)
	if route == nil || !route.enforce || account == nil {
		return false
	}
	blocked := textRoutePolicy.blocked(route.key(account, model), time.Now(), route.pinned)
	if blocked {
		if route.coolingAccounts == nil {
			route.coolingAccounts = make(map[int64]bool)
		}
		route.coolingAccounts[account.ID] = true
	}
	return blocked
}

func textRouteCandidateAllowed(ctx context.Context, account *Account, model string) bool {
	if account == nil || textRouteBlocked(ctx, account, model) {
		return false
	}
	route := textRouteFromContext(ctx)
	return route == nil || ctx.Value(preferIndependentTextRouteKey{}) != true ||
		textRouteOrigin(account, route.key(account, model).endpoint) != route.failedOrigin
}

func textStickyWaitExclusions(ctx context.Context, selection *AccountSelectionResult, excluded map[int64]struct{}) map[int64]struct{} {
	route := textRouteFromContext(ctx)
	if route == nil || !route.enforce || route.pinned || selection == nil || selection.Account == nil || selection.WaitPlan == nil {
		return nil
	}
	for _, binding := range route.sticky {
		if binding.snapshot.AccountID == selection.Account.ID {
			alternate := make(map[int64]struct{}, len(excluded)+1)
			for id := range excluded {
				alternate[id] = struct{}{}
			}
			alternate[selection.Account.ID] = struct{}{}
			return alternate
		}
	}
	return nil
}

func preferIndependentTextRoute(ctx context.Context, excluded map[int64]struct{}) (context.Context, bool) {
	route := textRouteFromContext(ctx)
	if route == nil || !route.enforce || route.failedOrigin == "" || route.pinned {
		return ctx, false
	}
	if _, failed := excluded[route.failedAccount]; !failed {
		return ctx, false
	}
	return context.WithValue(ctx, preferIndependentTextRouteKey{}, true), true
}

type TextRouteAttempt struct {
	request      *textRouteRequest
	policy       *textRouteHealthPolicy
	key          textRouteKey
	probe        uint64
	origin       string
	initialSends int32
}

// BeginTextRouteAttempt reserves the sole recovery request after a cooldown.
// Callers release an acquired account slot when admission loses a race.
func BeginTextRouteAttempt(ctx context.Context, account *Account, model string) (*TextRouteAttempt, bool) {
	route := textRouteFromContext(ctx)
	if route == nil || account == nil {
		return nil, true
	}
	if route.enforce && route.sends.Load() >= TextRouteMaxAttempts {
		return nil, false
	}
	policy := &textRoutePolicy
	if !route.enforce {
		policy = &observedTextRoutePolicy
	}
	key := route.key(account, model)
	probe, ok := policy.admit(key, time.Now(), route.enforce, route.pinned)
	if !ok {
		return nil, false
	}
	return &TextRouteAttempt{request: route, policy: policy, key: key, probe: probe, origin: textRouteOrigin(account, key.endpoint), initialSends: route.sends.Load()}, true
}

func (p *textRouteHealthPolicy) admit(key textRouteKey, now time.Time, enforce, pinned bool) (uint64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry := p.entries[key]
	if !enforce {
		if !entry.until.IsZero() && !now.Before(entry.until) {
			delete(p.entries, key)
		}
		return 0, true
	}
	if pinned {
		return 0, true
	}
	if entry.probe != 0 || now.Before(entry.until) {
		return 0, false
	}
	if entry.until.IsZero() {
		return 0, true
	}
	p.nextProbe++
	entry.probe = p.nextProbe
	p.entries[key] = entry
	return entry.probe, true
}

func (a *TextRouteAttempt) Finish(ctx context.Context, success bool, err error) {
	if a == nil {
		return
	}
	alive := ctx.Err() == nil
	success = alive && success
	sent := a.request.sends.Load() != a.initialSends
	upstreamFault := alive && (!a.request.enforce || sent) && IsTextRouteFailure(err)
	var failover *UpstreamFailoverError
	accessFault := alive && sent && errors.As(err, &failover) && IsTextRouteAccessFailure(failover)
	recovered := success && a.request.failedAccount != 0 && a.request.failedAccount != a.key.accountID
	if upstreamFault || accessFault || recovered || a.probe != 0 {
		logger.FromContext(ctx).Info("text_route_attempt_result",
			zap.Int64("account_id", a.key.accountID),
			zap.String("model", a.key.model),
			zap.String("upstream_protocol", a.key.endpoint),
			zap.Bool("success", success),
			zap.Bool("recovered", recovered),
			zap.Bool("upstream_fault", upstreamFault),
			zap.Bool("upstream_access_failure", accessFault),
			zap.Bool("probe", a.probe != 0),
			zap.Bool("enforced", a.request.enforce),
			zap.Int32("upstream_sends", a.request.sends.Load()),
		)
	}
	if upstreamFault || accessFault {
		a.request.failedOrigin = a.origin
		a.request.failedAccount = a.key.accountID
	}
	opened := a.policy.finish(a.key, a.probe, time.Now(), success, upstreamFault)
	if opened {
		if a.request.coolingAccounts == nil {
			a.request.coolingAccounts = make(map[int64]bool)
		}
		a.request.coolingAccounts[a.key.accountID] = true
		slog.Warn("text_route_cooldown", "account_id", a.key.accountID, "model", a.key.model,
			"endpoint", a.key.endpoint, "enforced", a.request.enforce)
	}
	if success && a.request.enforce {
		a.request.commitSticky(ctx, a.key.accountID)
	}
}

func (p *textRouteHealthPolicy) finish(key textRouteKey, probe uint64, now time.Time, success, fault bool) bool {
	if key.model == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	entry := p.entries[key]
	if entry.probe != probe || (probe == 0 && !entry.until.IsZero()) {
		// A result from an older request cannot interrupt a cooldown or probe.
		return false
	}
	if probe != 0 {
		entry.probe = 0
		switch {
		case success:
			delete(p.entries, key)
		case fault:
			entry.cooldown = min(entry.cooldown*2, textRouteMaxCooldown)
			entry.until = now.Add(entry.cooldown)
			p.entries[key] = entry
			return true
		default:
			p.entries[key] = entry
		}
		return false
	}
	// Successful traffic doesn't erase other failures in this rolling window.
	if !fault {
		return false
	}
	if len(p.entries) >= textRouteMaxEntries && entry.firstFailure.IsZero() {
		for oldKey, old := range p.entries {
			if old.probe == 0 && !now.Before(old.until) && now.Sub(old.firstFailure) > textRouteMaxCooldown {
				delete(p.entries, oldKey)
			}
		}
		if len(p.entries) >= textRouteMaxEntries {
			return false
		}
	}
	if entry.firstFailure.IsZero() || now.Sub(entry.firstFailure) > textRouteFailureWindow {
		entry.firstFailure = now
		entry.failures = 0
	}
	entry.failures++
	opened := entry.failures >= 2
	if opened {
		entry.cooldown = textRouteInitialCooldown
		entry.until = now.Add(entry.cooldown)
	}
	p.entries[key] = entry
	return opened
}

func textRouteOrigin(account *Account, protocol string) string {
	baseURL := account.GetCustomBaseURL()
	if account.IsAdaptiveAPIProtocol() {
		baseURL = account.GetCNProtocolBaseURL(protocol)
	}
	if baseURL == "" {
		baseURL = account.GetCredential("base_url")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return account.Platform
	}
	return strings.ToLower(parsed.Hostname())
}

// IsTextRouteAccessFailure identifies access failures already eligible for
// account failover, so they can use the remaining attempts on other accounts.
func IsTextRouteAccessFailure(failure *UpstreamFailoverError) bool {
	return failure != nil && failure.ShouldRetryNextAccount() &&
		(failure.StatusCode == http.StatusUnauthorized || failure.StatusCode == http.StatusForbidden)
}

func IsTextRouteFailure(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, ErrUpstreamResponseBodyTooLarge) ||
		errors.Is(err, ErrTextRouteAttemptBudget) {
		return false
	}
	var streamFailure *textRouteStreamFailure
	if errors.As(err, &streamFailure) {
		return true
	}
	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		if failoverErr.IsCredentialFailure() || failoverErr.RequestScopedTransient || failoverErr.Scope == GatewayFailureScopeRequest {
			return false
		}
		return failoverErr.StatusCode >= http.StatusInternalServerError && failoverErr.StatusCode != 529 &&
			shouldCooldownOpenAITransientUpstreamError(failoverErr.StatusCode, failoverErr.ResponseBody)
	}
	_, _, classified := OpenAIUpstreamStreamReadErrorDetails(err)
	return classified
}

type textRouteStreamFailure struct{ error }

func (e *textRouteStreamFailure) Unwrap() error { return e.error }

func markTextRouteStreamFailure(err error) error {
	return &textRouteStreamFailure{error: err}
}
