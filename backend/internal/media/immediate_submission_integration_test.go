//go:build integration

package media_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/stretchr/testify/require"
)

func TestNewRequestsSubmitImmediatelyAndIdempotentRetriesDoNotResubmit(t *testing.T) {
	db := testDB(t)
	snap := fixture(t, db, "100")
	defer cleanupMediaTasks(t, db, snap.Identity.UserID)
	ctx := context.Background()
	cap := snap.Capability
	cap.RequestFormat, cap.Adapter = "v6k_url", "v6k_video_gateway"
	cap.Duration = media.Duration{Mode: "enum", Values: []int{5}}
	cap.Limits.ObservationSeconds = ptr(600)
	cap.Delivery.PollSeconds = 600
	card := media.PriceCard{ID: "immediate-sale", Version: "1", Status: "enabled", Purpose: "sale", MediaType: "video", Operation: "video", Currency: "USD", Rules: []media.Rule{snap.SaleRule}, MultiplierPolicy: "effective_media_then_time", SuccessPolicy: "all_outputs"}
	card.ModelMatch.Kind, card.ModelMatch.Value = "exact", cap.Model
	card.Scope.Kind, card.Scope.Reference = "group", fmt.Sprint(snap.Identity.GroupID)
	var submits atomic.Int32
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	var once sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := submits.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": fmt.Sprintf("upstream-%d", n), "status": "queued"})
	}))
	defer upstream.Close()
	defer once.Do(func() { close(release) })
	blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("s", 32))
	require.NoError(t, err)
	engine := media.NewEngine(&media.Store{DB: db}, blobs, func(id, group int64) (media.Account, error) {
		return media.Account{ID: id, BaseURL: upstream.URL, Key: "isolated-test", Allowed: true}, nil
	}, nil)
	defer engine.Stop()
	engine.Adapter.Client = media.SafeClient(true)
	engine.Config = func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
		return []media.Capability{cap}, []media.PriceCard{card}, nil
	}
	input := map[string]any{"model": cap.Model, "prompt": "fixture", "seconds": 5}
	type result struct {
		task media.StoredTask
		err  error
	}
	results := make(chan result, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			task, err := engine.Create(ctx, cap, "video", input, nil, snap.Identity, fmt.Sprintf("immediate-%d", i))
			results <- result{task, err}
		}(i)
	}
	for i := 0; i < 10; i++ {
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("new requests waited for a local worker instead of reaching the upstream concurrently")
		}
	}
	var queued int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM media_tasks WHERE account_id=$1 AND state='queued'`, snap.Capability.AccountIDs[0]).Scan(&queued))
	require.Zero(t, queued)
	// The same idempotency key returns its original leased task while the POST
	// is in flight and does not create another supplier job or another hold.
	again, err := engine.Create(ctx, cap, "video", input, nil, snap.Identity, "immediate-0")
	require.NoError(t, err)
	require.Equal(t, "submitting", again.State)
	require.EqualValues(t, 10, submits.Load())
	once.Do(func() { close(release) })
	for i := 0; i < 10; i++ {
		select {
		case result := <-results:
			require.NoError(t, result.err)
			require.Equal(t, "in_progress", result.task.State)
			require.NotEmpty(t, result.task.UpstreamID)
		case <-time.After(10 * time.Second):
			t.Fatal("immediate request did not return the accepted task")
		}
	}
	var held string
	require.NoError(t, db.QueryRow(`SELECT frozen_balance::text FROM users WHERE id=$1`, snap.Identity.UserID).Scan(&held))
	require.Equal(t, "10.00000000", held)
}

func TestBackgroundSchedulerDoesNotSerializeBehindTwoSlowSubmissions(t *testing.T) {
	db := testDB(t)
	snap := fixture(t, db, "100")
	parkOtherMediaTasks(t, db, snap.Identity.UserID)
	defer cleanupMediaTasks(t, db, snap.Identity.UserID)
	snap.Capability.Adapter, snap.Capability.RequestFormat = "v6k_video_gateway", "v6k_url"
	snap.Capability.Limits.ObservationSeconds = ptr(600)
	snap.Capability.Delivery.PollSeconds = 600
	var submits atomic.Int32
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	var once sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := submits.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": fmt.Sprintf("upstream-%d", n), "status": "queued"})
	}))
	defer upstream.Close()
	defer once.Do(func() { close(release) })
	blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("s", 32))
	require.NoError(t, err)
	store := &media.Store{DB: db}
	for i := 0; i < 10; i++ {
		id := media.NewTaskID()
		path, err := blobs.WriteJSON(id, "request", media.Request{Operation: "video", Parameters: map[string]any{"model": snap.Capability.Model, "prompt": "fixture", "seconds": 5}})
		require.NoError(t, err)
		snap.Request.Parameters = map[string]any{"request_blob": path}
		_, err = store.CreateTask(context.Background(), snap, snap.Capability.AccountIDs[0], "", "", id)
		require.NoError(t, err)
	}
	engine := media.NewEngine(store, blobs, func(id, group int64) (media.Account, error) {
		return media.Account{ID: id, BaseURL: upstream.URL, Key: "isolated-test", Allowed: true}, nil
	}, nil)
	engine.Adapter.Client = media.SafeClient(true)
	engine.Start()
	defer engine.Stop()
	for i := 0; i < 10; i++ {
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("scheduler is still blocked by a fixed worker count")
		}
	}
	once.Do(func() { close(release) })
	require.Eventually(t, func() bool {
		var accepted int
		err := db.QueryRow(`SELECT count(*) FROM media_tasks WHERE account_id=$1 AND state='in_progress' AND upstream_id IS NOT NULL`, snap.Capability.AccountIDs[0]).Scan(&accepted)
		return err == nil && accepted == 10
	}, 10*time.Second, 20*time.Millisecond)
	engine.Stop()
	require.EqualValues(t, 10, submits.Load())
}

func TestBackgroundSchedulerDoesNotClaimBeyondConfiguredWorkers(t *testing.T) {
	db := testDB(t)
	snap := fixture(t, db, "100")
	parkOtherMediaTasks(t, db, snap.Identity.UserID)
	defer cleanupMediaTasks(t, db, snap.Identity.UserID)
	snap.Capability.Adapter, snap.Capability.RequestFormat = "v6k_video_gateway", "v6k_url"
	snap.Capability.Limits.ObservationSeconds = ptr(600)
	snap.Capability.Delivery.PollSeconds = 600

	const workerLimit = 3
	const taskCount = 8
	var submits atomic.Int32
	entered := make(chan struct{}, taskCount)
	release := make(chan struct{})
	var once sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := submits.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": fmt.Sprintf("upstream-%d", n), "status": "queued"})
	}))
	defer upstream.Close()
	defer once.Do(func() { close(release) })
	blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("w", 32))
	require.NoError(t, err)
	store := &media.Store{DB: db}
	for i := 0; i < taskCount; i++ {
		id := media.NewTaskID()
		path, err := blobs.WriteJSON(id, "request", media.Request{Operation: "video", Parameters: map[string]any{"model": snap.Capability.Model, "prompt": "fixture", "seconds": 5}})
		require.NoError(t, err)
		snap.Request.Parameters = map[string]any{"request_blob": path}
		_, err = store.CreateTask(context.Background(), snap, snap.Capability.AccountIDs[0], "", "", id)
		require.NoError(t, err)
	}
	engine := media.NewEngine(store, blobs, func(id, group int64) (media.Account, error) {
		return media.Account{ID: id, BaseURL: upstream.URL, Key: "isolated-test", Allowed: true}, nil
	}, nil)
	engine.BackgroundWorkers = workerLimit
	engine.Adapter.Client = media.SafeClient(true)
	engine.Start()
	defer engine.Stop()

	for i := 0; i < workerLimit; i++ {
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("configured workers did not concurrently reach the upstream")
		}
	}
	select {
	case <-entered:
		t.Fatal("scheduler claimed more tasks than its configured worker slots")
	case <-time.After(250 * time.Millisecond):
	}
	var submitting, queued int
	require.NoError(t, db.QueryRow(`SELECT count(*) FILTER (WHERE state='submitting'), count(*) FILTER (WHERE state='queued') FROM media_tasks WHERE account_id=$1`, snap.Capability.AccountIDs[0]).Scan(&submitting, &queued))
	require.Equal(t, workerLimit, submitting)
	require.Equal(t, taskCount-workerLimit, queued)

	once.Do(func() { close(release) })
	require.Eventually(t, func() bool {
		var accepted int
		err := db.QueryRow(`SELECT count(*) FROM media_tasks WHERE account_id=$1 AND state='in_progress' AND upstream_id IS NOT NULL`, snap.Capability.AccountIDs[0]).Scan(&accepted)
		return err == nil && accepted == taskCount
	}, 15*time.Second, 25*time.Millisecond)
	require.EqualValues(t, taskCount, submits.Load())
}
