//go:build integration

package media_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("MEDIA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("Set MEDIA_TEST_DATABASE_URL to an isolated disposable PostgreSQL database")
	}
	db, e := sql.Open("postgres", dsn)
	require.NoError(t, e)
	require.NoError(t, db.Ping())
	require.NoError(t, repository.ApplyMigrations(context.Background(), db))
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMediaHoldAccountsForLegacyHTTPReservation(t *testing.T) {
	db := testDB(t)
	snap := fixture(t, db, "1.5")
	s := &media.Store{DB: db, LegacyBalanceHeld: func(context.Context, int64) (string, error) { return "1", nil }}
	_, err := s.CreateTask(context.Background(), snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.ErrorContains(t, err, "insufficient_funds")
	balance, frozen := moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "1.50000000", balance)
	require.Equal(t, "0.00000000", frozen)
}

func TestAdmissionReadsDurableHoldBeforeCacheRecovery(t *testing.T) {
	db := testDB(t)
	snap := fixture(t, db, "10")
	s := &media.Store{DB: db}
	ctx := context.Background()
	_, err := db.Exec(`UPDATE api_keys SET quota=1,rate_limit_5h=1 WHERE id=$1`, snap.Identity.KeyID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	scope, err := s.AdmissionQuota(ctx, snap.Identity.KeyID, 0)
	require.NoError(t, err)
	require.Equal(t, "key_total", scope)
	_, err = db.Exec(`UPDATE api_keys SET quota=0 WHERE id=$1`, snap.Identity.KeyID)
	require.NoError(t, err)
	scope, err = s.AdmissionQuota(ctx, snap.Identity.KeyID, 0)
	require.NoError(t, err)
	require.Equal(t, "key_5h", scope)
	_, err = db.Exec(`UPDATE media_tasks SET state='failed' WHERE id=$1`, task.ID)
	require.NoError(t, err)
	require.NoError(t, s.Settle(ctx, task.ID, media.Calculation{Total: "0.00000000", Uncapped: "0.00000000"}, "failure", nil))
	scope, err = s.AdmissionQuota(ctx, snap.Identity.KeyID, 0)
	require.NoError(t, err)
	require.Empty(t, scope)
}

func TestMediaReservationWaitsForLegacyAdmissionAndReadsItsNewHold(t *testing.T) {
	db := testDB(t)
	snap := fixture(t, db, "1.5")
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	name := fmt.Sprintf("billing-user:%d", snap.Identity.UserID)
	_, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, name)
	require.NoError(t, err)
	var held atomic.Bool
	store := &media.Store{DB: db, LegacyBalanceHeld: func(context.Context, int64) (string, error) {
		if held.Load() {
			return "1", nil
		}
		return "0", nil
	}}
	done := make(chan error, 1)
	go func() {
		_, e := store.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
		done <- e
	}()
	select {
	case e := <-done:
		t.Fatalf("media admission bypassed active legacy admission: %v", e)
	case <-time.After(50 * time.Millisecond):
	}
	held.Store(true)
	_, err = conn.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, name)
	require.NoError(t, err)
	select {
	case e := <-done:
		require.ErrorContains(t, e, "insufficient_funds")
	case <-time.After(5 * time.Second):
		t.Fatal("media admission did not resume after legacy reservation")
	}
	balance, frozen := moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "1.50000000", balance)
	require.Equal(t, "0.00000000", frozen)
}

func TestSubscriptionShortTermDoesNotResetUsageAtCalendarBoundary(t *testing.T) {
	db := testDB(t)
	snap := fixture(t, db, "10")
	ctx := context.Background()
	_, err := db.Exec(`UPDATE groups SET subscription_type='subscription',daily_limit_usd=1,weekly_limit_usd=1,monthly_limit_usd=1 WHERE id=$1`, snap.Identity.GroupID)
	require.NoError(t, err)
	start := time.Now().Add(-time.Hour)
	var subscription int64
	require.NoError(t, db.QueryRow(`INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status,daily_usage_usd,weekly_usage_usd,monthly_usage_usd,daily_window_start,weekly_window_start,monthly_window_start) VALUES($1,$2,$3,$4,'active',0.6,0.6,0.6,$3,$3,$3) RETURNING id`, snap.Identity.UserID, snap.Identity.GroupID, start, start.Add(2*time.Hour)).Scan(&subscription))
	snap.Identity.SubscriptionID = subscription
	snap.Maximum = "0.5"
	_, err = (&media.Store{DB: db}).CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.ErrorContains(t, err, "quota_exceeded")
	var used string
	require.NoError(t, db.QueryRow(`SELECT weekly_usage_usd::text FROM user_subscriptions WHERE id=$1`, subscription).Scan(&used))
	require.Equal(t, "0.6000000000", used)
}
func ptr[T any](v T) *T { return &v }
func fixture(t *testing.T, db *sql.DB, balance string) media.Snapshot {
	t.Helper()
	ctx := context.Background()
	var user, group, account, key int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,balance) VALUES('media-test-'||gen_random_uuid()||'@example.invalid','isolated-test-only',$1) RETURNING id`, balance).Scan(&user))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name,platform) VALUES('media-test-'||gen_random_uuid(),'openai') RETURNING id`).Scan(&group))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type,credentials,concurrency) VALUES('media-test-'||gen_random_uuid(),'openai','apikey','{}',100) RETURNING id`).Scan(&account))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO api_keys(user_id,key,name,group_id) VALUES($1,'media-test-'||gen_random_uuid(),'media-test',$2) RETURNING id`, user, group).Scan(&key))
	cap := media.Capability{ID: media.NewTaskID(), Model: "00sd2.5", UpstreamModel: "00sd2.5", MediaType: "video", AccountIDs: []int64{account}, GroupIDs: []int64{group}, Duration: media.Duration{Mode: "fixed", Values: []int{30}}}
	cap.Limits.MaxConcurrentTasks = ptr(30)
	cap.Delivery.PollSeconds = 10
	return media.Snapshot{Capability: cap, Identity: media.Identity{UserID: user, KeyID: key, GroupID: group, Platform: "openai", Multiplier: "1", TimeMultiplier: "1"}, Request: media.Request{Operation: "video", Quantities: map[string][]string{"task_count": {"1"}, "fixed_duration": {"30"}}}, Maximum: "1.00000000", SaleRule: media.Rule{Charges: []media.Charge{{Name: "test", Unit: "request", Price: ptr("1"), QuantitySource: "task_count", Trigger: "success", Refundable: true, Method: "linear", Rounding: media.Rounding{Mode: "none", Step: "1", Minimum: "0", Scope: "aggregate"}}}}}
}
func moneyState(t *testing.T, db *sql.DB, user int64) (string, string) {
	t.Helper()
	var balance, frozen string
	require.NoError(t, db.QueryRow(`SELECT balance::text,frozen_balance::text FROM users WHERE id=$1`, user).Scan(&balance, &frozen))
	return balance, frozen
}

func cleanupMediaTasks(t *testing.T, db *sql.DB, userIDs ...int64) {
	t.Helper()
	users := pq.Array(userIDs)
	for _, table := range []string{"media_artifacts", "media_cost_entries", "media_quota_holds", "media_ledger_entries"} {
		_, err := db.Exec(`DELETE FROM `+table+` WHERE task_id IN (SELECT id FROM media_tasks WHERE user_id = ANY($1))`, users)
		require.NoError(t, err)
	}
	_, err := db.Exec(`DELETE FROM media_tasks WHERE user_id = ANY($1)`, users)
	require.NoError(t, err)
}

func parkOtherMediaTasks(t *testing.T, db *sql.DB, userID int64) {
	t.Helper()
	_, err := db.Exec(`UPDATE media_tasks SET next_poll_at='infinity',lease_until=NULL WHERE user_id<>$1 AND settlement_state IN ('held','pending_usage')`, userID)
	require.NoError(t, err)
}

func TestPersistentReserveSettleRefundAndRepeatedOperations(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	snap := fixture(t, db, "10")
	ctx := context.Background()
	task, e := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "unique", "same", media.NewTaskID())
	require.NoError(t, e)
	b, f := moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "9.00000000", b)
	require.Equal(t, "1.00000000", f)
	for i := 0; i < 20; i++ {
		same, e := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "unique", "same", media.NewTaskID())
		require.NoError(t, e)
		require.Equal(t, task.ID, same.ID)
	}
	_, e = s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "unique", "different", media.NewTaskID())
	require.Error(t, e)
	_, e = db.Exec(`UPDATE media_tasks SET state='completed' WHERE id=$1`, task.ID)
	require.NoError(t, e)
	calc := media.Calculation{Currency: "USD", Total: "0.60000000", Uncapped: "0.60000000"}
	for i := 0; i < 20; i++ {
		require.NoError(t, s.Settle(ctx, task.ID, calc, "all", nil))
	}
	b, f = moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "9.40000000", b)
	require.Equal(t, "0.00000000", f)
	task, e = s.GetTask(ctx, task.ID)
	require.NoError(t, e)
	require.Equal(t, "settled", task.Settlement)
	require.NoError(t, s.Refund(ctx, task.ID, "refund-1", "0.20", "test refund", task.Version, snap.Identity.UserID))
	require.NoError(t, s.Refund(ctx, task.ID, "refund-1", "0.20", "test refund", task.Version, snap.Identity.UserID))
	b, f = moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "9.60000000", b)
	require.Equal(t, "0.00000000", f)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM usage_logs WHERE request_id=$1`, task.ID).Scan(&count))
	require.Equal(t, 1, count)
}
func TestConcurrentReserveCannotOversellAndFailureReleases(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	snap := fixture(t, db, "3")
	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	tasks := []media.StoredTask{}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, e := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
			if e == nil {
				mu.Lock()
				tasks = append(tasks, task)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	require.Len(t, tasks, 3)
	b, f := moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "0.00000000", b)
	require.Equal(t, "3.00000000", f)
	for _, task := range tasks {
		_, e := db.Exec(`UPDATE media_tasks SET state='failed' WHERE id=$1`, task.ID)
		require.NoError(t, e)
		require.NoError(t, s.Settle(ctx, task.ID, media.Calculation{Total: "0.00000000"}, "failure", nil))
	}
	b, f = moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "3.00000000", b)
	require.Equal(t, "0.00000000", f)
}

func TestMediaAdmissionUsesAccountConcurrencyAndCountsManualReview(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	store := &media.Store{DB: db}

	// The legacy capability value remains 2 in the snapshot, but the account
	// budget is 100 and therefore all three submissions are admitted.
	wide := fixture(t, db, "10")
	wide.Capability.Limits.MaxConcurrentTasks = ptr(2)
	for i := 0; i < 3; i++ {
		_, err := store.CreateTask(ctx, wide, wide.Capability.AccountIDs[0], "", "", media.NewTaskID())
		require.NoError(t, err)
	}

	narrow := fixture(t, db, "10")
	t.Cleanup(func() { cleanupMediaTasks(t, db, wide.Identity.UserID, narrow.Identity.UserID) })
	narrow.Capability.Limits.MaxConcurrentTasks = ptr(2)
	_, err := db.ExecContext(ctx, `UPDATE accounts SET concurrency=2 WHERE id=$1`, narrow.Capability.AccountIDs[0])
	require.NoError(t, err)
	manual, err := store.CreateTask(ctx, narrow, narrow.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE media_tasks SET state='unknown',settlement_state='manual_review' WHERE id=$1`, manual.ID)
	require.NoError(t, err)
	otherModel := narrow
	otherModel.Capability.Model = "same-account-second-model"
	_, err = store.CreateTask(ctx, otherModel, narrow.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	_, err = store.CreateTask(ctx, narrow, narrow.Capability.AccountIDs[0], "", "", media.NewTaskID())
	var admissionErr *media.Error
	require.ErrorAs(t, err, &admissionErr)
	require.Equal(t, 429, admissionErr.Status)
	require.Equal(t, "account_concurrency_exhausted", admissionErr.Code)
	require.Equal(t, 2, admissionErr.Details["account_concurrency"])
	require.Equal(t, 1, admissionErr.Details["manual_review"])
	_, err = db.ExecContext(ctx, `UPDATE accounts SET concurrency=3 WHERE id=$1`, narrow.Capability.AccountIDs[0])
	require.NoError(t, err)
	_, err = store.CreateTask(ctx, narrow, narrow.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err, "a live account concurrency edit must apply to the next admission")
}

func TestClaimHonorsAccountConcurrencyAcrossMediaModels(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	store := &media.Store{DB: db}
	snap := fixture(t, db, "10")
	_, err := db.ExecContext(ctx, `UPDATE accounts SET concurrency=2 WHERE id=$1`, snap.Capability.AccountIDs[0])
	require.NoError(t, err)
	first, err := store.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	second := snap
	second.Capability.Model = "claim-second-model"
	secondTask, err := store.CreateTask(ctx, second, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)

	parkOtherMediaTasks(t, db, snap.Identity.UserID)
	claimed, err := store.Claim(ctx)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, first.ID, claimed.ID)
	claimed, err = store.Claim(ctx)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, secondTask.ID, claimed.ID)
	claimed, err = store.Claim(ctx)
	require.NoError(t, err)
	require.Nil(t, claimed, "workers must not claim a third task beyond the account budget")
}
func TestInterruptedSubmissionIsNotResubmittedAndFencing(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	snap := fixture(t, db, "5")
	ctx := context.Background()
	task, e := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, e)
	_, e = db.Exec(`UPDATE media_tasks SET state='submitting',lease_until=now()-interval '1 minute',next_poll_at='1970-01-01' WHERE id=$1`, task.ID)
	require.NoError(t, e)
	parkOtherMediaTasks(t, db, snap.Identity.UserID)
	_, e = s.Claim(ctx)
	require.NoError(t, e)
	task, e = s.GetTask(ctx, task.ID)
	require.NoError(t, e)
	require.Equal(t, "manual_review", task.Settlement)
	b, f := moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "4.00000000", b)
	require.Equal(t, "1.00000000", f)
	require.Error(t, s.Progress(ctx, task, media.UpstreamResult{Status: "completed", Quantities: map[string][]string{}}, ""))
}
func TestInvariantFailureRollsBackAndFreeZeroQuotaDisabled(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	snap := fixture(t, db, "5")
	ctx := context.Background()
	task, e := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, e)
	_, e = db.Exec(`UPDATE media_tasks SET state='completed' WHERE id=$1;`, task.ID)
	require.NoError(t, e)
	_, e = db.Exec(`UPDATE users SET frozen_balance=0 WHERE id=$1`, snap.Identity.UserID)
	require.NoError(t, e)
	require.Error(t, s.Settle(ctx, task.ID, media.Calculation{Total: "1.00000000"}, "all", nil))
	task, e = s.GetTask(ctx, task.ID)
	require.NoError(t, e)
	require.Equal(t, "held", task.Settlement)
	snap.Maximum = "0.00000000"
	_, e = db.Exec(`INSERT INTO user_platform_quotas(user_id,platform,daily_limit_usd) VALUES($1,'openai',0)`, snap.Identity.UserID)
	require.NoError(t, e)
	_, e = s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.Error(t, e)
}
func TestConfigurationVersionsDoNotDeadlockWithSingleConnection(t *testing.T) {
	db := testDB(t)
	db.SetMaxOpenConns(1)
	s := &media.Store{DB: db}
	snap := fixture(t, db, "5")
	var card media.PriceCard
	raw := []byte(`{"price_id":"placeholder","version":"1","purpose":"sale","currency":"USD","media_type":"video","operation":"video","model_match":{"kind":"exact","value":"00sd2.5"},"scope":{"kind":"group","reference":"1"},"rules":[]}`)
	require.NoError(t, json.Unmarshal(raw, &card))
	card.ID = media.NewTaskID()
	card.Scope.Reference = fmt.Sprint(snap.Identity.GroupID)
	card.SuccessPolicy = "any_output"
	card.MultiplierPolicy = "effective_media_then_time"
	price := "1"
	card.Rules = []media.Rule{{Priority: 1, When: map[string]any{}, Charges: []media.Charge{{Name: "generation", Unit: "request", Price: &price, QuantitySource: "task_count", Trigger: "success", Method: "linear", Rounding: media.Rounding{Mode: "none", Step: "1", Minimum: "0", Scope: "aggregate"}}}}}
	data, e := json.Marshal(card)
	require.NoError(t, e)
	ctx := context.Background()
	record, e := s.SaveConfig(ctx, "price", card.ID, 0, snap.Identity.UserID, data, true)
	require.NoError(t, e)
	require.EqualValues(t, 1, record.Version)
	_, e = s.SaveConfig(ctx, "price", card.ID, 0, snap.Identity.UserID, data, false)
	require.Error(t, e)
}

func TestFailureWithoutUsageAndStaleSettlement(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	snap := fixture(t, db, "5")
	ctx := context.Background()
	task, err := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	require.NoError(t, s.Progress(ctx, task, media.UpstreamResult{Status: "failed"}, ""))
	task, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"1"}, task.Quantities["task_count"])
	_, err = db.Exec(`UPDATE media_tasks SET lease_epoch=lease_epoch+1 WHERE id=$1`, task.ID)
	require.NoError(t, err)
	require.Error(t, s.SettleClaim(ctx, task, media.Calculation{Total: "0.00000000"}, "failure", nil))
	task, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.NoError(t, s.SettleClaim(ctx, task, media.Calculation{Total: "0.00000000"}, "failure", nil))
	b, f := moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "5.00000000", b)
	require.Equal(t, "0.00000000", f)
}
