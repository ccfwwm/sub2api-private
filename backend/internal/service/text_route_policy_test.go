package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTextRouteCooldownAndSingleRecovery(t *testing.T) {
	policy := textRouteHealthPolicy{entries: make(map[textRouteKey]textRouteHealth)}
	key := textRouteKey{accountID: 2217, model: "gpt-6.1-sol", endpoint: "/v1/responses"}
	other := textRouteKey{accountID: 2217, model: "gpt-6.2-sol", endpoint: "/v1/responses"}
	now := time.Unix(1, 0)
	require.False(t, policy.finish(key, 0, now, false, true))
	require.True(t, policy.finish(key, 0, now.Add(time.Second), false, true))
	require.True(t, policy.blocked(key, now.Add(2*time.Second), false))
	require.False(t, policy.blocked(other, now.Add(2*time.Second), false))
	require.False(t, policy.finish(key, 0, now.Add(3*time.Second), true, false))
	require.True(t, policy.blocked(key, now.Add(3*time.Second), false))

	recoveryAt := now.Add(62 * time.Second)
	var admitted atomic.Int32
	var lease atomic.Uint64
	var callers sync.WaitGroup
	for range 16 {
		callers.Go(func() {
			if probe, ok := policy.admit(key, recoveryAt, true, false); ok {
				admitted.Add(1)
				lease.Store(probe)
			}
		})
	}
	callers.Wait()
	require.EqualValues(t, 1, admitted.Load())
	require.NotZero(t, lease.Load())
	require.True(t, policy.finish(key, lease.Load(), recoveryAt, false, true))
	require.True(t, policy.blocked(key, recoveryAt.Add(119*time.Second), false))
	retryAt := recoveryAt.Add(120 * time.Second)
	probe, ok := policy.admit(key, retryAt, true, false)
	require.True(t, ok)
	require.NotZero(t, probe)
	require.False(t, policy.finish(key, probe, retryAt, true, false))
	require.False(t, policy.blocked(key, retryAt, false))
}

func TestTextRoutePrefersIndependentOriginOnlyOnSwitch(t *testing.T) {
	account := func(id int64, baseURL string) *Account {
		return &Account{ID: id, Type: AccountTypeAPIKey, Platform: PlatformOpenAI,
			Credentials: map[string]any{"base_url": baseURL}}
	}
	const model = "gpt-6.1-sol"
	original := account(990031, "https://first.example/v1")
	sibling := account(990032, "https://first.example/v1")
	independent := account(990033, "https://second.example/v1")
	ctx := WithTextRoutePolicy(WithTextInference(context.Background(), true), "enforce", 100, "session", "/v1/responses", false)
	attempt, ok := BeginTextRouteAttempt(ctx, original, model)
	require.True(t, ok)
	require.True(t, ClaimTextRouteSend(ctx))
	attempt.Finish(ctx, false, markTextRouteStreamFailure(io.ErrUnexpectedEOF))
	_, prefers := preferIndependentTextRoute(ctx, nil)
	require.False(t, prefers)
	preferred, prefers := preferIndependentTextRoute(ctx, map[int64]struct{}{original.ID: {}})
	require.True(t, prefers)
	require.False(t, textRouteCandidateAllowed(preferred, sibling, model))
	require.True(t, textRouteCandidateAllowed(preferred, independent, model))
	require.True(t, textRouteCandidateAllowed(ctx, sibling, model))
}

func TestTextRouteAccessFailurePrefersAnotherOriginWithoutCooldown(t *testing.T) {
	const model = "gpt-6.1-sol"
	account := &Account{ID: 990034, Type: AccountTypeAPIKey, Platform: PlatformOpenAI,
		Credentials: map[string]any{"base_url": "https://first.example/v1"}}
	failure := &UpstreamFailoverError{StatusCode: http.StatusForbidden, RetryableOnSameAccount: true}
	for range 2 {
		ctx := WithTextRoutePolicy(WithTextInference(context.Background(), true), "enforce", 100, "session", "/v1/responses", false)
		attempt, ok := BeginTextRouteAttempt(ctx, account, model)
		require.True(t, ok)
		require.True(t, ClaimTextRouteSend(ctx))
		attempt.Finish(ctx, false, failure)
		preferred, ok := preferIndependentTextRoute(ctx, map[int64]struct{}{account.ID: {}})
		require.True(t, ok)
		require.False(t, textRouteCandidateAllowed(preferred, account, model))
		require.False(t, textRouteBlocked(ctx, account, model))
	}
}

func TestTextRouteResponsesReportedHTTP2Failure(t *testing.T) {
	early := "event: response.created\ndata: {\"type\":\"response.created\"}\n\n"
	delta := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"
	failed := "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"upstream_error\",\"message\":\"Upstream HTTP/2 stream failed\"}}}\n\n"
	standalone := "event: error\ndata: {\"type\":\"error\",\"code\":\"upstream_http2_stream_error\",\"message\":\"Upstream HTTP/2 stream failed\"}\n\n"
	for _, passthrough := range []bool{false, true} {
		for _, payload := range []struct {
			name     string
			body     string
			switches bool
		}{
			{"first event", failed, true},
			{"metadata", early + failed, true},
			{"error event", early + standalone, true},
			{"partial output", delta + failed, false},
		} {
			path := "ordinary"
			if passthrough {
				path = "passthrough"
			}
			t.Run(path+"/"+payload.name, func(t *testing.T) {
				c, rec := newPassthroughKeepaliveTestContext(t)
				ctx := WithTextRoutePolicy(WithTextInference(c.Request.Context(), true), "enforce", 100, "session", "/v1/responses", false)
				c.Request = c.Request.WithContext(ctx)
				n, err := c.Writer.WriteString(": keepalive\n\n")
				require.NoError(t, err)
				RecordOpenAIQueueHeartbeat(c, n)
				resp := &http.Response{
					StatusCode: 200,
					Header:     http.Header{"X-Request-Id": {"failed-attempt"}, "X-Codex-Turn-State": {"failed-state"}},
					Body:       io.NopCloser(strings.NewReader(payload.body)),
				}
				account := &Account{ID: 2217, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
				s := &OpenAIGatewayService{}
				if passthrough {
					_, err = s.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-6.1-sol", "gpt-6.1-sol")
				} else {
					_, err = s.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-6.1-sol", "gpt-6.1-sol")
				}
				require.True(t, IsTextRouteFailure(err))
				var failover *UpstreamFailoverError
				if payload.switches {
					require.ErrorAs(t, err, &failover)
					require.False(t, failover.RetryableOnSameAccount)
					require.Equal(t, ": keepalive\n\n", rec.Body.String())
					require.Empty(t, rec.Header().Get("X-Codex-Turn-State"))
					require.Empty(t, rec.Header().Get("X-Request-Id"))
				} else {
					require.NotErrorAs(t, err, &failover)
					require.Contains(t, rec.Body.String(), "hello")
					require.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.failed"))
				}
			})
		}
	}
}
