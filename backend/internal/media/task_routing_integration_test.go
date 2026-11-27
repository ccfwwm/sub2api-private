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
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestManagedMediaFailoverKeepsTaskAndLedger(t *testing.T) {
	for _, operation := range []string{"video", "image", "audio"} {
		t.Run(operation, func(t *testing.T) {
			db := testDB(t)
			snap := fixture(t, db, "10")
			defer cleanupMediaTasks(t, db, snap.Identity.UserID)
			capability := snap.Capability
			capability.Enabled, capability.Status = true, "enabled"
			capability.MediaType, capability.Operations = operation, []string{operation}
			capability.Formats = []string{"json"}
			templateID := map[string]string{"video": "video-reference-arrays", "image": "image-json-multipart", "audio": "music-chat-json"}[operation]
			capability.TemplateRef = &media.TemplateRef{ID: templateID, Version: 1}
			capability.Duration = media.Duration{Mode: "enum", Values: []int{15}}
			capability.Limits.MaxRequestBytes, capability.Limits.ObservationSeconds = ptr(int64(1<<20)), ptr(600)
			accounts := []int64{capability.AccountIDs[0]}
			for range 2 {
				var id int64
				require.NoError(t, db.QueryRow(`INSERT INTO accounts(name,platform,type,credentials,concurrency)
					VALUES('media-route-'||gen_random_uuid(),'openai','apikey','{}',10) RETURNING id`).Scan(&id))
				accounts = append(accounts, id)
			}
			caps := []media.Capability{}
			for index, account := range accounts {
				_, err := db.Exec(`INSERT INTO account_groups(account_id,group_id) VALUES ($1,$2)`, account, snap.Identity.GroupID)
				require.NoError(t, err)
				cap := capability
				cap.ID, cap.UpstreamModel, cap.AccountIDs = fmt.Sprint(account), fmt.Sprintf("upstream-model-%d", index), []int64{account}
				caps = append(caps, cap)
			}
			calls := []string{}
			var callsMu sync.Mutex
			var taskID string
			mode := ""
			png := pngFixture(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				model := fmt.Sprint(body["model"])
				callsMu.Lock()
				calls = append(calls, model)
				currentMode := mode
				currentTaskID := taskID
				callsMu.Unlock()
				wantKey := currentTaskID
				if model == "upstream-model-1" {
					wantKey += fmt.Sprintf("-a%d", accounts[1])
				} else if model == "upstream-model-2" {
					wantKey += fmt.Sprintf("-a%d", accounts[2])
				}
				if request.Header.Get("Idempotency-Key") != wantKey {
					t.Error("wrong idempotency key for account attempt")
				}
				w.Header().Set("Content-Type", "application/json")
				switch currentMode {
				case "uncertain":
					w.WriteHeader(http.StatusGatewayTimeout)
					_, _ = fmt.Fprint(w, `{}`)
					return
				case "accepted_failure":
					_, _ = fmt.Fprint(w, `{"id":"failed-after-acceptance","status":"failed"}`)
					return
				}
				if model != "upstream-model-1" || currentMode == "exhausted" {
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = fmt.Fprint(w, `{"error":{"code":"fail_to_fetch_task","message":"upstream capacity exhausted"}}`)
					return
				}
				if operation == "image" {
					_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
				} else {
					_, _ = fmt.Fprint(w, `{"id":"backup-job","status":"queued"}`)
				}
			}))
			defer upstream.Close()
			blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("r", 32))
			require.NoError(t, err)
			store := &media.Store{DB: db}
			lookup := func(id, group int64) (media.Account, error) {
				base := upstream.URL
				if id == accounts[2] {
					base = strings.Replace(base, "127.0.0.1", "localhost", 1)
				}
				return media.Account{ID: id, Priority: map[int64]int{accounts[0]: 1, accounts[1]: 10, accounts[2]: 20}[id], BaseURL: base, Allowed: group == snap.Identity.GroupID, Key: "fixture"}, nil
			}
			createEngine := func() *media.Engine {
				engine := media.NewEngine(store, blobs, lookup, nil)
				engine.Adapter.Client = media.SafeClient(true)
				engine.Routes, err = media.NewRoutePolicy("enforce", "", nil)
				require.NoError(t, err)
				price := saleCardForIntegration(capability, snap.SaleRule, snap.Identity.GroupID)
				price.Operation = operation
				engine.Config = func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
					return []media.Capability{caps[0], caps[2], caps[1]}, []media.PriceCard{price}, nil
				}
				return engine
			}
			engine := createEngine()
			ctx := context.Background()
			input := map[string]any{"model": capability.Model, "prompt": "fixture"}
			if operation == "video" {
				input["seconds"] = 15
			}
			selected, err := engine.SelectCapability(ctx, capability.Model, operation, input, nil, snap.Identity)
			require.NoError(t, err)
			require.NotNil(t, selected)
			task, err := engine.Create(ctx, *selected, operation, input, nil, snap.Identity, "single-reserve")
			require.NoError(t, err)
			callsMu.Lock()
			taskID = task.ID
			callsMu.Unlock()
			require.Len(t, task.Snapshot.Routing.Candidates, 3, "compatible accounts should be frozen at admission")
			require.Equal(t, accounts[0], task.AccountID)
			first, err := store.ClaimTask(ctx, task.ID)
			require.NoError(t, err)
			require.NotNil(t, first)
			require.NoError(t, engine.Process(ctx, *first))
			task, err = store.GetTask(ctx, task.ID)
			require.NoError(t, err)
			require.Equal(t, "queued", task.State)
			require.Equal(t, accounts[1], task.AccountID)
			require.Len(t, task.Snapshot.Routing.Attempts, 1)
			require.Equal(t, "rejected", task.Snapshot.Routing.Attempts[0].Outcome)
			engine.Stop()
			engine = createEngine()
			defer engine.Stop()
			duplicate, err := engine.Create(ctx, *selected, operation, input, nil, snap.Identity, "single-reserve")
			require.NoError(t, err)
			require.Equal(t, task.ID, duplicate.ID)
			next, err := store.ClaimTask(ctx, task.ID)
			require.NoError(t, err)
			require.NotNil(t, next)
			require.NoError(t, engine.Process(ctx, *next))
			task, err = store.GetTask(ctx, task.ID)
			require.NoError(t, err)
			callsMu.Lock()
			sentModels := append([]string(nil), calls...)
			callsMu.Unlock()
			require.Equal(t, []string{"upstream-model-0", "upstream-model-1"}, sentModels)
			require.Len(t, task.Snapshot.Routing.Attempts, 2)
			var reserves int
			require.NoError(t, db.QueryRow(`SELECT count(*) FROM media_ledger_entries WHERE task_id=$1 AND kind='reserve'`, task.ID).Scan(&reserves))
			require.Equal(t, 1, reserves)
			if operation == "image" {
				require.Equal(t, "settled", task.Settlement)
				require.Equal(t, "1.00000000", task.Captured)
			} else {
				require.Equal(t, "in_progress", task.State)
				require.Equal(t, "backup-job", task.UpstreamID)
			}
			if operation != "video" {
				return
			}
			for _, scenario := range []struct {
				mode, terminal, settlement string
				attempts                   int
			}{
				{"uncertain", "unknown", "manual_review", 1},
				{"accepted_failure", "failed", "released", 1},
				{"exhausted", "failed", "released", 3},
			} {
				callsMu.Lock()
				mode, calls = scenario.mode, nil
				callsMu.Unlock()
				fresh, createErr := engine.Create(ctx, *selected, operation, input, nil, snap.Identity, scenario.mode)
				require.NoError(t, createErr)
				callsMu.Lock()
				taskID = fresh.ID
				callsMu.Unlock()
				for range scenario.attempts {
					leased, claimErr := store.ClaimTask(ctx, fresh.ID)
					require.NoError(t, claimErr)
					require.NotNil(t, leased)
					require.NoError(t, engine.Process(ctx, *leased))
				}
				fresh, err = store.GetTask(ctx, fresh.ID)
				require.NoError(t, err)
				require.Equal(t, scenario.terminal, fresh.State)
				require.Equal(t, scenario.settlement, fresh.Settlement)
				require.Len(t, fresh.Snapshot.Routing.Attempts, scenario.attempts)
				if scenario.mode == "accepted_failure" {
					require.Equal(t, "failed-after-acceptance", fresh.UpstreamID)
				}
				callsMu.Lock()
				sent := len(calls)
				callsMu.Unlock()
				require.Equal(t, scenario.attempts, sent)
			}
		})
	}
}

func TestManagedMediaTerminalFailureRetriesAndOpensCircuitAfterThree(t *testing.T) {
	db := testDB(t)
	snapshot := fixture(t, db, "10")
	defer cleanupMediaTasks(t, db, snapshot.Identity.UserID)
	primary := snapshot.Capability.AccountIDs[0]
	var backup int64
	require.NoError(t, db.QueryRow(`INSERT INTO accounts(name,platform,type,credentials,concurrency)
		VALUES('media-backup-'||gen_random_uuid(),'openai','apikey','{}',10) RETURNING id`).Scan(&backup))
	_, err := db.Exec(`INSERT INTO account_groups(account_id,group_id) VALUES ($1,$3),($2,$3)`, primary, backup, snapshot.Identity.GroupID)
	require.NoError(t, err)
	capability := snapshot.Capability
	capability.Enabled, capability.Status = true, "enabled"
	capability.Model, capability.UpstreamModel = "gemini-omni-1.2-flash", "gemini-omni-1.2-flash"
	capability.Operations, capability.Formats = []string{"video"}, []string{"json"}
	capability.Duration = media.Duration{Mode: "range", Min: ptr(3), Max: ptr(15), Default: ptr(15)}
	capability.TemplateRef = &media.TemplateRef{ID: "foxrouter-video-base64-image-strings", Version: 1}
	capability.Limits.MaxRequestBytes, capability.Limits.ObservationSeconds = ptr(int64(1<<20)), ptr(600)
	alternate := capability
	alternate.ID, alternate.AccountIDs = "backup-video", []int64{backup}
	alternate.TemplateRef = &media.TemplateRef{ID: "video-reference-arrays", Version: 1}
	alternate.Duration.Default = ptr(10)

	var primaryPosts, backupPosts atomic.Int32
	var policyFailure atomic.Bool
	video := mp4Fixture(t)
	reference := "data:image/png;base64," + pngFixture(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			var body struct {
				Duration        int      `json:"duration"`
				Images          []string `json:"images"`
				ReferenceImages []string `json:"reference_images"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Duration != 15 {
				t.Errorf("wrong frozen duration: %d", body.Duration)
			}
			if strings.HasPrefix(r.Host, "localhost:") {
				if len(body.Images) != 1 || body.Images[0] != reference {
					t.Error("primary did not receive its reference image")
				}
				_, _ = fmt.Fprintf(w, `{"id":"primary-%d","status":"queued"}`, primaryPosts.Add(1))
			} else {
				if len(body.ReferenceImages) != 1 || body.ReferenceImages[0] != reference {
					t.Error("backup did not receive the same reference image in its own format")
				}
				_, _ = fmt.Fprintf(w, `{"id":"backup-%d","status":"queued"}`, backupPosts.Add(1))
			}
			return
		}
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if id == "content" {
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write(video)
			return
		}
		if id == "backup-3" {
			_, _ = fmt.Fprintf(w, `{"id":%q,"status":"completed"}`, id)
			return
		}
		if strings.HasPrefix(id, "backup-") || policyFailure.Load() {
			_, _ = fmt.Fprintf(w, `{"id":%q,"status":"failed","error":{"code":"content_policy_violation","message":"Content policy refusal"}}`, id)
			return
		}
		w.Header().Set("Retry-After", "60")
		_, _ = fmt.Fprintf(w, `{"id":%q,"status":"failed","progress":100,"error":{"code":"video_generation_failed","message":"Upstream video generation service temporarily unavailable for requested model"}}`, id)
	}))
	defer upstream.Close()
	redisServer := miniredis.RunT(t)
	now := time.Now()
	redisServer.SetTime(now)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	store := &media.Store{DB: db}
	blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("r", 32))
	require.NoError(t, err)
	engine := media.NewEngine(store, blobs, func(id, group int64) (media.Account, error) {
		base := upstream.URL
		if id == primary {
			base = strings.Replace(base, "127.0.0.1", "localhost", 1)
		}
		return media.Account{ID: id, BaseURL: base, Key: "fixture", Allowed: group == snapshot.Identity.GroupID}, nil
	}, nil)
	defer engine.Stop()
	engine.Adapter.Client = media.SafeClient(true)
	engine.Routes, err = media.NewRoutePolicy("enforce", "", repository.NewMediaRouteHealth(redisClient))
	require.NoError(t, err)
	price := saleCardForIntegration(capability, snapshot.SaleRule, snapshot.Identity.GroupID)
	engine.Config = func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
		return []media.Capability{capability, alternate}, []media.PriceCard{price}, nil
	}
	ctx := t.Context()
	input := map[string]any{"model": capability.Model, "prompt": "fixture", "images": []any{reference}, "ratio": "9:16", "resolution": "1080p"}
	selected, err := engine.SelectCapability(ctx, capability.Model, "video", input, nil, snapshot.Identity)
	require.NoError(t, err)
	require.NotNil(t, selected)
	create := func(key string) media.StoredTask {
		t.Helper()
		task, createErr := engine.Create(ctx, *selected, "video", input, nil, snapshot.Identity, key)
		require.NoError(t, createErr)
		require.Len(t, task.Snapshot.Routing.Candidates, 2)
		return task
	}
	process := func(task media.StoredTask) media.StoredTask {
		t.Helper()
		_, updateErr := db.Exec(`UPDATE media_tasks SET next_poll_at=now() WHERE id=$1`, task.ID)
		require.NoError(t, updateErr)
		claim, claimErr := store.ClaimTask(ctx, task.ID)
		require.NoError(t, claimErr)
		require.NotNil(t, claim)
		require.NoError(t, engine.Process(ctx, *claim))
		updated, getErr := store.GetTask(ctx, task.ID)
		require.NoError(t, getErr)
		return updated
	}
	for i := range 3 {
		key := fmt.Sprintf("original-%d", i)
		first := process(create(key))
		primaryJob := fmt.Sprintf("primary-%d", i+1)
		require.Equal(t, primaryJob, first.UpstreamID)
		queued := process(first)
		require.Equal(t, first.ID, queued.ID)
		require.Equal(t, backup, queued.AccountID)
		require.Empty(t, queued.UpstreamID)
		require.Empty(t, queued.ResultPath)
		require.Equal(t, "queued", queued.State)
		require.Equal(t, "held", queued.Settlement)
		require.Equal(t, "1.00000000", queued.Reserved)
		require.Equal(t, []string{"1"}, queued.Quantities["accepted_input_images"])
		require.NotContains(t, queued.Quantities, "upstream_progress")
		require.Len(t, queued.Snapshot.Routing.Attempts, 1)
		attempt := queued.Snapshot.Routing.Attempts[0]
		require.Equal(t, "in_progress", attempt.Outcome)
		require.NotNil(t, attempt.Terminal)
		require.Equal(t, "video_generation_failed", attempt.Terminal.Code)
		require.Equal(t, primaryJob, attempt.Terminal.UpstreamID)
		require.Equal(t, first.ID, create(key).ID)

		second := process(queued)
		require.Equal(t, fmt.Sprintf("backup-%d", i+1), second.UpstreamID)
		require.NotEqual(t, attempt.IdempotencyKey, second.Snapshot.Routing.Attempts[1].IdempotencyKey)
		second = process(second)
		if i == 2 {
			require.Equal(t, "completed", second.State)
			require.Equal(t, "settled", second.Settlement, second.Diagnostic)
			require.Equal(t, "1.00000000", second.Captured)
			artifacts, artifactErr := store.Artifacts(ctx, second.ID)
			require.NoError(t, artifactErr)
			require.Len(t, artifacts, 1)
			require.True(t, artifacts[0].Available)
		} else {
			require.Equal(t, "failed", second.State)
			require.Equal(t, "released", second.Settlement)
			require.Equal(t, "1.00000000", second.Released)
		}
		require.Len(t, second.Snapshot.Routing.Attempts, 2)
		var reserves int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM media_ledger_entries WHERE task_id=$1 AND kind='reserve'`, first.ID).Scan(&reserves))
		require.Equal(t, 1, reserves)
	}
	require.EqualValues(t, 3, primaryPosts.Load())
	require.EqualValues(t, 3, backupPosts.Load())

	redisServer.SetTime(now.Add(31 * time.Second))
	require.Equal(t, backup, create("while-open").AccountID)
	redisServer.SetTime(now.Add(61 * time.Second))
	probe := process(create("recovery-probe"))
	require.Equal(t, "primary-4", probe.UpstreamID)
	require.Equal(t, backup, create("while-probing").AccountID)
	policyFailure.Store(true)
	probe = process(probe)
	require.Equal(t, "released", probe.Settlement)
	require.Len(t, probe.Snapshot.Routing.Attempts, 1)
	require.Equal(t, backup, create("after-inconclusive-probe").AccountID)
}
