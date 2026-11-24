package service

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/stretchr/testify/require"
)

func TestNativeImageRoutingSharesRejectionRulesAndLimits(t *testing.T) {
	policy, err := media.NewRoutePolicy("enforce", "53,69", nil)
	require.NoError(t, err)
	ctx := WithMediaRoutePolicy(t.Context(), policy)
	_, disabled := StartNativeMediaRouting(ctx, 57, "image", "image-model")
	require.Nil(t, disabled)
	ctx, routing := StartNativeMediaRouting(ctx, 53, "image", "image-model")
	account := func(id int64, origin string) *Account {
		return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": origin}}
	}
	rejection := &UpstreamFailoverError{StatusCode: 429, ResponseBody: []byte(`{"error":{"code":"rate_limit_exceeded"}}`)}
	uncertain := &UpstreamFailoverError{StatusCode: 504}
	accepted := &UpstreamFailoverError{StatusCode: 429, ResponseHeaders: http.Header{"Location": {"/jobs/accepted"}}}
	require.False(t, routing.CanSwitch(uncertain))
	require.False(t, routing.CanSwitch(accepted))
	first := account(1, "https://primary.example")
	second := account(2, "https://backup.example")
	finish, ok := routing.Begin(ctx, first)
	require.True(t, ok)
	finish(false, rejection)
	require.True(t, routing.CanSwitch(rejection))
	preferred, ok := preferIndependentNativeMedia(ctx)
	require.True(t, ok)
	require.False(t, mediaRouteCandidateAllowed(preferred, account(3, "https://primary.example")))
	require.True(t, mediaRouteCandidateAllowed(preferred, second))
	for _, next := range []*Account{second, account(4, "https://third.example")} {
		finish, ok = routing.Begin(ctx, next)
		require.True(t, ok)
		finish(false, rejection)
	}
	require.False(t, routing.CanSwitch(rejection))
	_, admitted := routing.Begin(ctx, account(5, "https://fourth.example"))
	require.False(t, admitted)
}
