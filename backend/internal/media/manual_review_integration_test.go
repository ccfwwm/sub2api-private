//go:build integration

package media_test

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestManualReviewReleasesUnknownAndCanChargeAndRefundAnyTask(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	ctx := context.Background()
	snap := fixture(t, db, "10")
	task, err := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE media_tasks SET state='unknown',settlement_state='manual_review' WHERE id=$1`, task.ID)
	require.NoError(t, err)
	input := media.ManualReview{Status: "failed", Target: "0", Reason: "confirmed upstream 503", Version: task.Version}
	require.NoError(t, s.ReviewTask(ctx, task.ID, "return-all", input, snap.Identity.UserID))
	require.NoError(t, s.ReviewTask(ctx, task.ID, "return-all", input, snap.Identity.UserID))
	balance, frozen := moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "10.00000000", balance)
	require.Equal(t, "0.00000000", frozen)
	current, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", current.State)
	require.Equal(t, "released", current.Settlement)
	input.Target = "0.6"
	input.Status = "completed"
	input.Version = current.Version
	require.ErrorContains(t, s.ReviewTask(ctx, task.ID, "return-all", input, snap.Identity.UserID), "idempotency_conflict")
	require.NoError(t, s.ReviewTask(ctx, task.ID, "charge", input, snap.Identity.UserID))
	balance, frozen = moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "9.40000000", balance)
	require.Equal(t, "0.00000000", frozen)
	current, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	input.Target = "0.2"
	input.Version = current.Version
	require.NoError(t, s.ReviewTask(ctx, task.ID, "partial-return", input, snap.Identity.UserID))
	balance, _ = moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "9.80000000", balance)
	require.ErrorContains(t, s.ReviewTask(ctx, task.ID, "stale", input, snap.Identity.UserID), "version_conflict")
	var quota string
	require.NoError(t, db.QueryRow(`SELECT quota_used::text FROM api_keys WHERE id=$1`, snap.Identity.KeyID).Scan(&quota))
	require.Equal(t, "0.20000000", quota)
}
