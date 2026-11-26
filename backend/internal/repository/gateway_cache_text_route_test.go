package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestTextRouteStickyWritesOnlyAfterSuccess(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewGatewayCache(client)
	plain := t.Context()
	textContext := func() context.Context {
		return service.WithTextRoutePolicy(service.WithTextInference(plain, true), "enforce", 100, "session", "/v1/responses", false)
	}
	account := func(id int64) *service.Account {
		return &service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	}
	store := func(ctx context.Context, id int64) {
		t.Helper()
		require.NoError(t, cache.SetSessionAccountID(ctx, 33, "session", id, time.Hour))
	}
	load := func(ctx context.Context, expected int64) {
		t.Helper()
		id, err := cache.GetSessionAccountID(ctx, 33, "session")
		require.NoError(t, err)
		require.Equal(t, expected, id)
	}
	finish := func(ctx context.Context, id int64, success bool) {
		t.Helper()
		attempt, ok := service.BeginTextRouteAttempt(ctx, account(id), "gpt-6.1-sol")
		require.True(t, ok)
		require.True(t, service.ClaimTextRouteSend(ctx))
		attempt.Finish(ctx, success, nil)
	}

	store(plain, 1)
	temporary := textContext()
	load(temporary, 1)
	store(temporary, 2)
	finish(temporary, 2, true)
	load(plain, 1)

	failed := textContext()
	load(failed, 1)
	require.NoError(t, cache.DeleteSessionAccountID(failed, 33, "session"))
	store(failed, 2)
	finish(failed, 2, false)
	load(plain, 1)

	// A newer write of the same owner is also a new revision.
	stale := textContext()
	load(stale, 1)
	require.NoError(t, cache.DeleteSessionAccountID(stale, 33, "session"))
	store(stale, 2)
	store(plain, 1)
	finish(stale, 2, true)
	load(plain, 1)

	migration := textContext()
	load(migration, 1)
	require.NoError(t, cache.DeleteSessionAccountID(migration, 33, "session"))
	store(migration, 2)
	load(plain, 1)
	finish(migration, 2, true)
	load(plain, 2)
}
