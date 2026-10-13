//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorMediaSnapshotAndPendingAvailability(t *testing.T) {
	ctx := context.Background()
	repo := NewChannelMonitorRepository(integrationEntClient, integrationDB)
	m := &service.ChannelMonitor{Name: "media-roundtrip", Provider: service.MonitorProviderImageVideo, Endpoint: "https://media.example", APIKey: "encrypted", PrimaryModel: "omni-flash", IntervalSeconds: 60, BodyOverrideMode: "merge", BodyOverride: map[string]any{"media_monitor": map[string]any{"mode": "connection", "models": map[string]any{}}}}
	require.NoError(t, repo.Create(ctx, m))
	t.Cleanup(func() { _ = repo.Delete(ctx, m.ID) })
	read, err := repo.GetByID(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, service.MonitorProviderImageVideo, read.Provider)
	require.Equal(t, m.BodyOverride, read.BodyOverride)
	read.BodyOverride["media_monitor"] = map[string]any{"mode": "generation", "revision": "explicit-reset"}
	require.NoError(t, repo.Update(ctx, read))
	read, err = repo.GetByID(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, "explicit-reset", read.BodyOverride["media_monitor"].(map[string]any)["revision"])
	now := time.Now().UTC()
	require.NoError(t, repo.InsertHistoryBatch(ctx, []*service.ChannelMonitorHistoryRow{
		{MonitorID: m.ID, Model: m.PrimaryModel, Status: service.MonitorStatusPending, CheckedAt: now},
		{MonitorID: m.ID, Model: m.PrimaryModel, Status: service.MonitorStatusOperational, CheckedAt: now.Add(-time.Minute)},
	}))
	history, err := repo.ListHistory(ctx, m.ID, "", 10)
	require.NoError(t, err)
	require.Equal(t, service.MonitorStatusPending, history[0].Status)
	availability, err := repo.ComputeAvailability(ctx, m.ID, 7)
	require.NoError(t, err)
	require.Len(t, availability, 1)
	require.Equal(t, 1, availability[0].TotalChecks)
	require.Equal(t, float64(100), availability[0].AvailabilityPct)
	batch, err := repo.ComputeAvailabilityForMonitors(ctx, []int64{m.ID}, 7)
	require.NoError(t, err)
	require.Equal(t, float64(100), batch[m.ID][0].AvailabilityPct)
}
