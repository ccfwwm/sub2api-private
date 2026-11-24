package repository

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestMediaRouteCooldownAllowsOneSharedRecoveryProbe(t *testing.T) {
	server := miniredis.RunT(t)
	now := time.Unix(1000, 0)
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	health := NewMediaRouteHealth(client)
	keys := []string{"media:route:account:1", "media:route:origin:test"}
	ctx := t.Context()
	require.NoError(t, health.Reject(ctx, keys[1], 30*time.Second))
	wait, err := health.RetryIn(ctx, keys)
	require.NoError(t, err)
	require.Equal(t, 30*time.Second, wait)
	server.SetTime(now.Add(31 * time.Second))
	var probes atomic.Int32
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			delay, acquireErr := health.Acquire(ctx, keys, "worker")
			if acquireErr != nil {
				t.Error(acquireErr)
			}
			if delay == 0 {
				probes.Add(1)
			}
		})
	}
	workers.Wait()
	require.EqualValues(t, 1, probes.Load())
	require.NoError(t, health.Finish(ctx, keys, "other-worker", true))
	wait, err = health.RetryIn(ctx, keys)
	require.NoError(t, err)
	require.Positive(t, wait)
	require.NoError(t, health.Finish(ctx, keys, "worker", true))
	wait, err = health.RetryIn(ctx, keys)
	require.NoError(t, err)
	require.Zero(t, wait)
}
