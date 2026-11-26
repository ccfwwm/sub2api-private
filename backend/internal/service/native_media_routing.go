package service

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/google/uuid"
)

type mediaRoutePolicyKey struct{}
type nativeMediaSelectionKey struct{}
type independentNativeMediaKey struct{}

func WithMediaRoutePolicy(ctx context.Context, policy *media.RoutePolicy) context.Context {
	return context.WithValue(ctx, mediaRoutePolicyKey{}, policy)
}

type NativeMediaRouting struct {
	policy    *media.RoutePolicy
	group     int64
	operation string
	model     string
	origins   map[string]bool
	sends     int
}

func StartNativeMediaRouting(ctx context.Context, group int64, operation, model string) (context.Context, *NativeMediaRouting) {
	policy, _ := ctx.Value(mediaRoutePolicyKey{}).(*media.RoutePolicy)
	if !policy.Enabled(group) {
		return ctx, nil
	}
	route := &NativeMediaRouting{policy: policy, group: group, operation: operation, model: model, origins: make(map[string]bool)}
	return context.WithValue(ctx, nativeMediaSelectionKey{}, route), route
}

func (r *NativeMediaRouting) target(account *Account) media.RouteTarget {
	base := account.GetCustomBaseURL()
	if base == "" {
		switch account.Platform {
		case PlatformOpenAI:
			base = account.GetOpenAIBaseURL()
		case PlatformGemini:
			base = account.GetGeminiBaseURL("https://generativelanguage.googleapis.com")
		default:
			base = account.GetCredential("base_url")
		}
	}
	return media.RouteTarget{GroupID: r.group, AccountID: account.ID, Origin: media.RouteOrigin(base, account.Platform),
		Model: r.model, Operation: r.operation}
}

func mediaRouteCandidateAllowed(ctx context.Context, account *Account) bool {
	return nativeMediaRouteBlockReason(ctx, account) == ""
}

func nativeMediaRouteBlockReason(ctx context.Context, account *Account) string {
	route, _ := ctx.Value(nativeMediaSelectionKey{}).(*NativeMediaRouting)
	if route == nil {
		return ""
	}
	target := route.target(account)
	if ctx.Value(independentNativeMediaKey{}) == true && route.origins[target.Origin] {
		return "media_same_origin"
	}
	if route.policy.RetryIn(ctx, target) > 0 {
		return "media_cooldown"
	}
	return ""
}

func preferIndependentNativeMedia(ctx context.Context) (context.Context, bool) {
	route, _ := ctx.Value(nativeMediaSelectionKey{}).(*NativeMediaRouting)
	if route == nil || len(route.origins) == 0 {
		return ctx, false
	}
	return context.WithValue(ctx, independentNativeMediaKey{}, true), true
}

func nativeMediaRoutingActive(ctx context.Context) bool {
	route, _ := ctx.Value(nativeMediaSelectionKey{}).(*NativeMediaRouting)
	return route != nil
}

func (r *NativeMediaRouting) Begin(ctx context.Context, account *Account) (finish func(bool, error), admitted bool) {
	if r == nil {
		return func(bool, error) {}, true
	}
	target, owner := r.target(account), uuid.NewString()
	if r.sends >= media.MaxSubmissionAttempts || r.policy.Acquire(ctx, target, owner) > 0 {
		return nil, false
	}
	r.sends++
	return func(success bool, err error) {
		var failure *UpstreamFailoverError
		var scope media.RejectionScope
		var delay time.Duration
		if errors.As(err, &failure) {
			scope = nativeMediaRejection(failure)
			if scope != "" {
				r.origins[target.Origin] = true
				delay = media.MediaRetryAfter(failure.ResponseHeaders, failure.ResponseBody, time.Now())
			}
		}
		r.policy.Finish(ctx, target, owner, scope, delay, success)
	}, true
}

func (r *NativeMediaRouting) CanSwitch(failure *UpstreamFailoverError) bool {
	return r == nil || r.sends < media.MaxSubmissionAttempts && nativeMediaRejection(failure) != ""
}

func nativeMediaRejection(failure *UpstreamFailoverError) media.RejectionScope {
	if failure == nil || !failure.ShouldRetryNextAccount() || failure.Scope == GatewayFailureScopeRequest ||
		failure.StreamFailedFrame != nil || failure.SafeToFailoverAfterWrite {
		return ""
	}
	if failure.ResponseHeaders.Get("Location") != "" || failure.ResponseHeaders.Get("X-Task-ID") != "" ||
		failure.ResponseHeaders.Get("X-Job-ID") != "" {
		return ""
	}
	return media.MediaRejectionScope(failure.StatusCode, failure.ResponseBody)
}
