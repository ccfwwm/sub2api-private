package repository

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
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

func TestMediaAccountCircuitUsesDistinctConsecutiveFailures(t *testing.T) {
	server := miniredis.RunT(t)
	now := time.Unix(1000, 0)
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	health := NewMediaRouteHealth(client)
	ctx := t.Context()
	key := "media:route:account:1"
	keys := []string{key}
	observe := func(attempt string, outcome media.RouteOutcome) {
		t.Helper()
		require.NoError(t, health.Observe(ctx, key, attempt, outcome, time.Minute))
	}
	retryIn := func() time.Duration {
		t.Helper()
		delay, err := health.RetryIn(ctx, keys)
		require.NoError(t, err)
		return delay
	}
	observe("earlier", media.RouteFailed)
	observe("initial-success", media.RouteSucceeded)
	observe("job-1", media.RouteFailed)
	observe("job-1", media.RouteFailed)
	observe("job-2", media.RouteFailed)
	observe("policy-refusal", media.RouteNeutral)
	require.Zero(t, retryIn())
	observe("job-3", media.RouteFailed)
	require.Equal(t, time.Minute, retryIn())
	observe("finishing-old-job", media.RouteSucceeded)
	require.Equal(t, time.Minute, retryIn())

	server.SetTime(now.Add(time.Minute))
	delay, err := health.Acquire(ctx, keys, "failed-probe")
	require.NoError(t, err)
	require.Zero(t, delay)
	require.Positive(t, retryIn())
	server.FastForward(3 * time.Minute)
	observe("failed-probe", media.RouteRunning)
	server.FastForward(time.Minute)
	require.Positive(t, retryIn())
	observe("failed-probe", media.RouteFailed)
	require.Equal(t, time.Minute, retryIn())

	server.SetTime(now.Add(2 * time.Minute))
	delay, err = health.Acquire(ctx, keys, "successful-probe")
	require.NoError(t, err)
	require.Zero(t, delay)
	observe("successful-probe", media.RouteSucceeded)
	require.Zero(t, retryIn())
	observe("new-failure", media.RouteFailed)
	require.Zero(t, retryIn())
}
