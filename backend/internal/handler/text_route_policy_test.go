package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestTextRoutingReservesAccessFailureRetriesForOtherAccounts(t *testing.T) {
	ctx := service.WithTextInference(context.Background(), true)
	enforced := service.WithTextRoutePolicy(ctx, "enforce", 100, "session", "/v1/responses", false)
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		failure := &service.UpstreamFailoverError{StatusCode: status, RetryableOnSameAccount: true}
		require.True(t, allowTextSameAccountRetry(ctx, failure, 0, 3))
		require.False(t, allowTextSameAccountRetry(enforced, failure, 0, 3))
	}
	rateLimit := &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, RetryableOnSameAccount: true}
	require.True(t, allowTextSameAccountRetry(enforced, rateLimit, 0, 3))
	require.False(t, allowTextSameAccountRetry(enforced, rateLimit, 1, 3))
}
