package media

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestSpeechRecoveryReturnsOriginalDurableTaskWithoutMutation(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	store := &Store{DB: db}
	ctx := context.Background()
	created := time.Date(2026, 10, 21, 1, 2, 3, 0, time.UTC)
	snapshot := Snapshot{
		Capability: Capability{Model: "public-speech", UpstreamModel: "original-speech", TemplateRef: &TemplateRef{ID: "speech-binary", Version: 1}},
		Identity:   Identity{UserID: 42, KeyID: 3, GroupID: 66, Multiplier: "2", TimeMultiplier: "1"},
		Request:    Request{Operation: "speech", Parameters: map[string]any{"request_blob": "original-encrypted-request"}},
		Maximum:    "0.00400000",
	}
	raw, err := json.Marshal(snapshot)
	require.NoError(t, err)
	quantities := []byte(`{"task_count":["1"],"input_characters":["4"]}`)
	columns := []string{"id", "state", "settlement_state", "upstream_id", "result_path", "diagnostic", "user_id", "api_key_id", "group_id", "subscription_id", "account_id", "snapshot", "quantities", "reserved", "accepted", "captured", "released", "refunded", "version", "lease_epoch", "created_at", "completed_at"}
	query := "SELECT " + taskSelect + " FROM media_tasks WHERE api_key_id=$1 AND operation=$2 AND idempotency_key=$3"
	// Read-only recovery must return the same task, account and frozen billing
	// data on every retry, even when acceptance is still uncertain. No writes
	// or current capability/template lookups are allowed by these expectations.
	for range 2 {
		mock.ExpectQuery(query).WithArgs(int64(3), "speech", "original-idempotency").WillReturnRows(
			sqlmock.NewRows(columns).AddRow("original-task", "unknown", "manual_review", "", "", "acceptance uncertain", 42, 3, 66, 0, 7, raw, quantities, "0.00400000", "0", "0", "0", "0", 5, 2, created, nil),
		)
		task, err := store.RecoverTask(ctx, 3, "speech", "original-idempotency")
		require.NoError(t, err)
		require.Equal(t, "original-task", task.ID)
		require.Equal(t, int64(7), task.AccountID)
		require.Equal(t, snapshot, task.Snapshot)
		require.Equal(t, "unknown", task.State)
		require.Equal(t, "manual_review", task.Settlement)
		require.Equal(t, "0.00400000", task.Reserved)
		require.Equal(t, []string{"4"}, task.Quantities["input_characters"])
	}
	for _, scope := range []struct {
		key int64
		op  string
	}{{4, "speech"}, {3, "audio"}} {
		mock.ExpectQuery(query).WithArgs(scope.key, scope.op, "original-idempotency").WillReturnRows(sqlmock.NewRows(columns))
		_, err := store.RecoverTask(ctx, scope.key, scope.op, "original-idempotency")
		require.ErrorContains(t, err, "task_not_found")
	}
	for _, invalid := range []struct{ op, key string }{{"unsupported", "original-idempotency"}, {"speech", ""}, {"speech", strings.Repeat("x", 201)}} {
		_, err := store.RecoverTask(ctx, 3, invalid.op, invalid.key)
		require.ErrorContains(t, err, "invalid_recovery")
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
