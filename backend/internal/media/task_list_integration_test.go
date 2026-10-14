//go:build integration

package media_test

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/stretchr/testify/require"
)

func TestTaskPaginationRealDatabaseAndUserIsolation(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	ctx := context.Background()
	snap := fixture(t, db, "100")
	snap.Capability.Limits.MaxConcurrentTasks = ptr(100)
	ids := []string{}
	for i := 0; i < 45; i++ {
		id := media.NewTaskID()
		ids = append(ids, id)
		_, err := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", id)
		require.NoError(t, err)
	}
	timestamp := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	_, err := db.Exec(`UPDATE media_tasks SET created_at=$1,state='submitting' WHERE user_id=$2`, timestamp, snap.Identity.UserID)
	require.NoError(t, err)
	other := fixture(t, db, "10")
	outsider, err := s.CreateTask(ctx, other, other.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	filter := media.TaskListFilter{UserID: snap.Identity.UserID, PageSize: 20, SortOrder: "desc"}
	seen := []string{}
	for p := 1; p <= 3; p++ {
		filter.Page = p
		rows, total, e := s.TaskPage(ctx, filter)
		require.NoError(t, e)
		require.EqualValues(t, 45, total)
		for _, r := range rows {
			seen = append(seen, r.ID)
			require.NotEqual(t, outsider.ID, r.ID)
		}
		records, e := s.TaskRecords(ctx, rows, false)
		require.NoError(t, e)
		encoded, e := json.Marshal(records)
		require.NoError(t, e)
		require.NotContains(t, string(encoded), `"account_id"`)
		require.NotContains(t, string(encoded), `"cost_amount"`)
	}
	require.Equal(t, ids, seen)
	filter.Page = 4
	rows, total, err := s.TaskPage(ctx, filter)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.EqualValues(t, 45, total)
	filter.Page = 1
	filter.KeyID = snap.Identity.KeyID
	filter.GroupID = snap.Identity.GroupID
	filter.AccountID = snap.Capability.AccountIDs[0]
	filter.Model = snap.Capability.Model
	filter.MediaType = "video"
	filter.Settlement = "held"
	filter.Status = "queued"
	start, end := timestamp.Add(-time.Second), timestamp.Add(time.Second)
	filter.Start, filter.End = &start, &end
	rows, total, err = s.TaskPage(ctx, filter)
	require.NoError(t, err)
	require.Len(t, rows, 20)
	require.EqualValues(t, 45, total)
	records, err := s.TaskRecords(ctx, rows, true)
	require.NoError(t, err)
	require.NotEmpty(t, records[0].UserEmail)
	require.NotEmpty(t, records[0].AccountName)
	filter.TaskID = outsider.ID
	rows, total, err = s.TaskPage(ctx, filter)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Zero(t, total)
	old, err := s.TaskList(ctx, snap.Identity.UserID, 20, ids[19])
	require.NoError(t, err)
	require.Equal(t, ids[20], old[0].ID)
	old, err = s.TaskList(ctx, snap.Identity.UserID, 20, outsider.ID)
	require.NoError(t, err)
	require.Empty(t, old)
	var indexes int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM pg_indexes WHERE indexname IN ('media_tasks_created_id','media_tasks_user_created_id')`).Scan(&indexes))
	require.Equal(t, 2, indexes)
}

func TestMediaAccountingActionsKeepVersionProtection(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	ctx := context.Background()
	snap := fixture(t, db, "10")
	task, err := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE media_tasks SET state='completed' WHERE id=$1`, task.ID)
	require.NoError(t, err)
	require.NoError(t, s.Settle(ctx, task.ID, media.Calculation{Currency: "USD", Total: "0.60000000", Uncapped: "0.60000000"}, "all", nil))
	task, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.ErrorContains(t, s.Refund(ctx, task.ID, "stale-refund", "0.1", "test", task.Version-1, snap.Identity.UserID), "version_conflict")
	require.ErrorContains(t, s.VerifyCost(ctx, task.ID, "0.2", "test", "stale-cost", task.Version-1, snap.Identity.UserID), "version_conflict")
	require.NoError(t, s.VerifyCost(ctx, task.ID, "0.2", "test", "cost", task.Version, snap.Identity.UserID))
	require.NoError(t, s.VerifyCost(ctx, task.ID, "0.2", "test", "cost", task.Version, snap.Identity.UserID))
	require.ErrorContains(t, s.Refund(ctx, task.ID, "refund", "0.1", "test", task.Version, snap.Identity.UserID), "version_conflict")
	task, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.NoError(t, s.Refund(ctx, task.ID, "refund", "0.1", "test", task.Version, snap.Identity.UserID))
	records, err := s.TaskRecords(ctx, []media.StoredTask{task}, true)
	require.NoError(t, err)
	require.Equal(t, "verified", records[0].CostStatus)
	pending, err := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE media_tasks SET state='unknown',settlement_state='manual_review' WHERE id=$1`, pending.ID)
	require.NoError(t, err)
	require.ErrorContains(t, s.Reconcile(ctx, pending.ID, "provider-task", "query", "test", pending.Version-1, snap.Identity.UserID), "reconcile_conflict")
	require.NoError(t, s.Reconcile(ctx, pending.ID, "provider-task", "query", "test", pending.Version, snap.Identity.UserID))
	require.ErrorContains(t, s.Reconcile(ctx, pending.ID, "provider-task", "query", "test", pending.Version, snap.Identity.UserID), "reconcile_conflict")
}
