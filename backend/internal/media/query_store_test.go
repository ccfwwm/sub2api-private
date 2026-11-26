package media

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestPublicVideoTaskWaitsForDownloadableArtifact(t *testing.T) {
	for _, tc := range []struct {
		name       string
		settlement string
		hasVideo   bool
		available  bool
		wantStatus string
	}{
		{name: "awaiting download", settlement: "pending_usage", wantStatus: "in_progress"},
		{name: "unavailable video", settlement: "pending_usage", hasVideo: true, wantStatus: "in_progress"},
		{name: "download ready", settlement: "pending_usage", hasVideo: true, available: true, wantStatus: "completed"},
		{name: "expired video", settlement: "settled", hasVideo: true, wantStatus: "completed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			completed := time.Unix(123, 0)
			stored := StoredTask{
				ID:         "mt_video",
				State:      "completed",
				Settlement: tc.settlement,
				Completed:  &completed,
				Snapshot:   Snapshot{Capability: Capability{MediaType: "video"}},
				Quantities: map[string][]string{"upstream_progress": {"100"}},
			}
			rows := sqlmock.NewRows([]string{"key", "media_type", "mime_type", "size_bytes", "duration_seconds", "available", "filename"})
			if tc.hasVideo {
				rows.AddRow("video1", "video", "video/mp4", 1234, "15.04", tc.available, "")
			}
			mock.ExpectQuery(`SELECT key,media_type,.*FROM media_artifacts`).WithArgs(stored.ID).WillReturnRows(rows)
			mock.ExpectQuery(`SELECT details FROM media_ledger_entries`).WithArgs(stored.ID + ":finalize").WillReturnRows(sqlmock.NewRows([]string{"details"}))

			public, err := (&Store{DB: db}).PublicTask(context.Background(), stored)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, public.Status)
			if tc.wantStatus == "in_progress" {
				require.Less(t, public.Progress, 100)
				require.Nil(t, public.CompletedAt)
			} else {
				require.Equal(t, 100, public.Progress)
				require.NotNil(t, public.CompletedAt)
				require.Equal(t, completed.Unix(), *public.CompletedAt)
			}
			if tc.hasVideo {
				require.Len(t, public.Artifacts, 1)
				require.Equal(t, "/v1/tasks/mt_video/artifacts/video1/content", public.Artifacts[0].ContentURL)
				require.Equal(t, tc.available, public.Artifacts[0].Available)
			} else {
				require.Empty(t, public.Artifacts)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
