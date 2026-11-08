//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorAtomicReceiptAndPolicy(t *testing.T) {
	ctx := context.Background()
	repo := NewChannelMonitorRepository(integrationEntClient, integrationDB).(*channelMonitorRepository)
	prompt := "中文测试"
	m := &service.ChannelMonitor{Name: "atomic-monitor", Provider: service.MonitorProviderOpenAI, Endpoint: "https://example.com", APIKey: "encrypted", PrimaryModel: "model", IntervalSeconds: 60, ProbeConfig: domain.MonitorProbeConfig{Models: map[string]domain.MonitorProbePolicy{"model": {Prompt: &prompt}}}}
	require.NoError(t, repo.Create(ctx, m))
	t.Cleanup(func() { _ = repo.Delete(ctx, m.ID) })
	loaded, err := repo.GetByID(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, m.ProbeConfig, loaded.ProbeConfig)
	now := time.Now().UTC()
	row := &service.ChannelMonitorHistoryRow{MonitorID: m.ID, Model: "model", Status: service.MonitorStatusDegraded, Message: strings.Repeat("中文😀", 200), SampleID: "stable-media-task", CheckedAt: now}
	require.NoError(t, repo.SaveCheckResults(ctx, m.ID, []*service.ChannelMonitorHistoryRow{row}, now))
	later := *row
	later.CheckedAt = now.Add(15 * time.Second)
	require.NoError(t, repo.SaveCheckResults(ctx, m.ID, []*service.ChannelMonitorHistoryRow{&later}, later.CheckedAt))
	history, err := repo.ListHistory(ctx, m.ID, "model", 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.WithinDuration(t, later.CheckedAt, history[0].CheckedAt, time.Millisecond)
	a, err := repo.ComputeAvailability(ctx, m.ID, 7)
	require.NoError(t, err)
	require.Len(t, a, 1)
	require.Equal(t, 1, a[0].TotalChecks)
	require.Equal(t, float64(100), a[0].AvailabilityPct)
	loaded, err = repo.GetByID(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded.LastCheckedAt)
	// A bad second row must roll back both the good row and freshness.
	bad := *row
	bad.SampleID = "bad"
	bad.Status = "invalid-state"
	good := *row
	good.SampleID = "good"
	err = repo.SaveCheckResults(ctx, m.ID, []*service.ChannelMonitorHistoryRow{&good, &bad}, now.Add(time.Hour))
	require.Error(t, err)
	a, err = repo.ComputeAvailability(ctx, m.ID, 7)
	require.NoError(t, err)
	require.Equal(t, 1, a[0].TotalChecks)
	after, err := repo.GetByID(ctx, m.ID)
	require.NoError(t, err)
	require.WithinDuration(t, *loaded.LastCheckedAt, *after.LastCheckedAt, time.Millisecond)
}

func TestChannelMonitorDurableRunLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := NewChannelMonitorRepository(integrationEntClient, integrationDB).(*channelMonitorRepository)
	m := &service.ChannelMonitor{Name: "run-monitor", Provider: service.MonitorProviderOpenAI, Endpoint: "https://example.com", APIKey: "encrypted", PrimaryModel: "model", IntervalSeconds: 60}
	require.NoError(t, repo.Create(ctx, m))
	t.Cleanup(func() { _ = repo.Delete(ctx, m.ID) })
	now := time.Now().UTC()
	run := &service.MonitorRun{RunID: "fd58938c-fd0a-4f51-a13f-e50121a79271", MonitorID: m.ID, State: "running", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
	require.NoError(t, repo.CreateMonitorRun(ctx, run))
	duplicate := *run
	duplicate.RunID = "fd58938c-fd0a-4f51-a13f-e50121a79272"
	require.Error(t, repo.CreateMonitorRun(ctx, &duplicate))
	require.NoError(t, repo.InterruptMonitorRuns(ctx, now.Add(time.Second)))
	loaded, err := repo.GetMonitorRun(ctx, m.ID, run.RunID)
	require.NoError(t, err)
	require.Equal(t, "interrupted", loaded.State)
	loaded.State = "completed"
	loaded.Results = []*service.CheckResult{{Model: "model", Status: service.MonitorStatusOperational}}
	require.NoError(t, repo.SaveMonitorRun(ctx, loaded))
	loaded, err = repo.GetMonitorRun(ctx, m.ID, run.RunID)
	require.NoError(t, err)
	require.Len(t, loaded.Results, 1)
	_, err = integrationDB.ExecContext(ctx, `UPDATE channel_monitor_runs SET expires_at=now()-interval '1 second' WHERE run_id=$1`, run.RunID)
	require.NoError(t, err)
	_, err = repo.GetMonitorRun(ctx, m.ID, run.RunID)
	require.Error(t, err)
}
