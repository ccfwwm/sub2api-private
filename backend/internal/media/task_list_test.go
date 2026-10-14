package media

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestTaskPageCombinedFiltersAndStableOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	start := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	f := TaskListFilter{UserID: 42, KeyID: 3, GroupID: 66, AccountID: 7, TaskID: "task_a", Model: "Model'", MediaType: "video", Status: "queued", Settlement: "held", Start: &start, End: &end, Page: 3, PageSize: 20, SortOrder: "asc"}
	// The hostile model string remains an argument, not SQL; count and items have identical scope.
	scope := `TRUE AND user_id=$1 AND api_key_id=$2 AND account_id=$3 AND group_id=$4 AND id=$5 AND model=$6 AND media_type=$7 AND settlement_state=$8 AND state IN ('queued','submitting') AND created_at >= $9 AND created_at < $10`
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM media_tasks WHERE "+scope)).WithArgs(int64(42), int64(3), int64(7), int64(66), "task_a", "Model'", "video", "held", start, end).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(45))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT "+taskSelect+" FROM media_tasks WHERE "+scope+" ORDER BY created_at ASC,id ASC LIMIT $11 OFFSET $12")).WithArgs(int64(42), int64(3), int64(7), int64(66), "task_a", "Model'", "video", "held", start, end, 20, 40).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	items, total, err := (&Store{DB: db}).TaskPage(context.Background(), f)
	require.NoError(t, err)
	require.EqualValues(t, 45, total)
	require.Empty(t, items)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRecordsBatchingAndUserPrivacy(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(map[bool]string{false: "user", true: "admin"}[admin], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			tasks := []StoredTask{}
			for _, id := range []string{"a", "b"} {
				tasks = append(tasks, StoredTask{ID: id, State: "completed", Created: time.Unix(123, 0), Accepted: "1", Captured: "2", Refunded: "0.5", Identity: Identity{UserID: 42, KeyID: 3, GroupID: 66}, Snapshot: Snapshot{Capability: Capability{MediaType: "video", Model: "m"}, Request: Request{Operation: "video"}}})
			}
			names := sqlmock.NewRows([]string{"id", "key", "group"}).AddRow("a", "my key", "media").AddRow("b", "my key", "media")
			if admin {
				names = sqlmock.NewRows([]string{"id", "key", "group", "user", "email", "account", "name"}).AddRow("a", "my key", "media", 42, "owner@test", 7, "supplier").AddRow("b", "my key", "media", 42, "owner@test", 7, "supplier")
			}
			mock.ExpectQuery(`SELECT t.id,.*WHERE t.id=ANY`).WillReturnRows(names)
			mock.ExpectQuery(`SELECT task_id,key,media_type.*FROM media_artifacts`).WillReturnRows(sqlmock.NewRows([]string{"task", "key", "type", "mime", "size", "duration", "available"}).AddRow("a", "0", "video", "video/mp4", 123, "8", true))
			mock.ExpectQuery(`SELECT task_id,details FROM media_ledger_entries`).WillReturnRows(sqlmock.NewRows([]string{"task", "details"}).AddRow("a", []byte(`{"total":"2","items":[]}`)))
			if admin {
				mock.ExpectQuery(`SELECT task_id,status,amount::text FROM media_cost_entries`).WillReturnRows(sqlmock.NewRows([]string{"task", "status", "amount"}).AddRow("a", "verified", "1.25"))
			}
			records, err := (&Store{DB: db}).TaskRecords(context.Background(), tasks, admin)
			require.NoError(t, err)
			require.Len(t, records, 2)
			require.Equal(t, "my key", records[0].KeyName)
			require.Equal(t, "/v1/tasks/a/artifacts/0/content", records[0].Task.Artifacts[0].ContentURL)
			raw, err := json.Marshal(records)
			require.NoError(t, err)
			if admin {
				require.Equal(t, "1.25000000", *records[0].MarginAmount)
				require.Contains(t, string(raw), `"account_name":"supplier"`)
			} else {
				for _, field := range []string{"account_id", "account_name", "cost_status", "cost_amount", "margin_amount", "user_email", "user_id", "snapshot", "upstream_id", "request_hash"} {
					require.NotContains(t, string(raw), `"`+field+`"`)
				}
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestEmptyTaskRecordsDoNotQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	records, err := (&Store{DB: db}).TaskRecords(context.Background(), nil, false)
	require.NoError(t, err)
	require.NotNil(t, records)
	require.Empty(t, records)
	require.NoError(t, mock.ExpectationsWereMet())
}
