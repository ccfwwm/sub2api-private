//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type probeTemplateRepo struct {
	ChannelMonitorRequestTemplateRepository
	template *ChannelMonitorRequestTemplate
	monitors *quotaModeRepoStub
}

func (r *probeTemplateRepo) Create(_ context.Context, template *ChannelMonitorRequestTemplate) error {
	template.ID = 9
	r.template = template
	return nil
}

func (r *probeTemplateRepo) GetByID(_ context.Context, _ int64) (*ChannelMonitorRequestTemplate, error) {
	return r.template, nil
}

func (r *probeTemplateRepo) ApplyToMonitors(_ context.Context, _ int64, _ []int64) (int64, error) {
	r.monitors.monitor.ProbeType = r.template.ProbeType
	r.monitors.monitor.ImageProbeScheduleEnabled = r.template.ImageProbeScheduleEnabled
	return 1, nil
}

func TestMonitorTemplateProbeDefaultsAndExplicitEmptyResponse(t *testing.T) {
	repo := &probeTemplateRepo{}
	svc := NewChannelMonitorRequestTemplateService(repo)
	template, err := svc.Create(context.Background(), ChannelMonitorRequestTemplateCreateParams{Name: "legacy", Provider: MonitorProviderOpenAI})
	require.NoError(t, err)
	require.Equal(t, MonitorProbeTypeText, template.ProbeType)
	require.False(t, template.ImageProbeScheduleEnabled)
	require.True(t, template.CustomResponseRequireNonEmpty)
	allowEmpty := false
	template, err = svc.Create(context.Background(), ChannelMonitorRequestTemplateCreateParams{
		Name: "custom", Provider: MonitorProviderOpenAI, ProbeType: MonitorProbeTypeCustom, CustomMethod: "get", CustomPath: "/health",
		CustomResponseRequireNonEmpty: &allowEmpty,
	})
	require.NoError(t, err)
	require.Equal(t, "GET", template.CustomMethod)
	require.False(t, template.CustomResponseRequireNonEmpty)
}

func TestMonitorTemplateApplyReschedulesImageProbeConsent(t *testing.T) {
	monitorRepo := &quotaModeRepoStub{monitor: &ChannelMonitor{
		ID: 77, Enabled: true, IntervalSeconds: 60, APIKey: "OLD:key", Provider: MonitorProviderOpenAI,
		TemplateID: int64Ptr(9),
	}}
	monitorSvc := newQuotaModeService(monitorRepo)
	runner := newRunnerForTest(&stubMonitorSvc{})
	runner.Start()
	defer runner.Stop()
	monitorSvc.SetScheduler(runner)
	runner.Schedule(monitorRepo.monitor)
	require.Equal(t, 1, runnerTaskCount(runner))
	repo := &probeTemplateRepo{
		template: &ChannelMonitorRequestTemplate{ID: 9, Provider: MonitorProviderOpenAI, ProbeType: MonitorProbeTypeImage}, monitors: monitorRepo,
	}
	svc := NewChannelMonitorRequestTemplateService(repo)
	svc.SetMonitorService(monitorSvc)
	affected, err := svc.ApplyToMonitors(context.Background(), 9, []int64{77})
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)
	require.Zero(t, runnerTaskCount(runner), "manual image snapshot must cancel the previous text timer")
	repo.template.ImageProbeScheduleEnabled = true
	_, err = svc.ApplyToMonitors(context.Background(), 9, []int64{77})
	require.NoError(t, err)
	require.Equal(t, 1, runnerTaskCount(runner))
}

func TestMonitorProbeTypeChangeRequiresNewScheduleConsent(t *testing.T) {
	image := MonitorProbeTypeImage
	for _, explicit := range []*bool{nil, boolPtr(true)} {
		monitor := &ChannelMonitor{Provider: MonitorProviderOpenAI, ImageProbeScheduleEnabled: true}
		err := applyMonitorAdvancedUpdate(monitor, ChannelMonitorUpdateParams{ProbeType: &image, ImageProbeScheduleEnabled: explicit}, false)
		require.NoError(t, err)
		require.Equal(t, explicit != nil, monitor.ImageProbeScheduleEnabled)
		template := &ChannelMonitorRequestTemplate{Provider: MonitorProviderOpenAI, ImageProbeScheduleEnabled: true}
		err = applyTemplateUpdate(template, ChannelMonitorRequestTemplateUpdateParams{ProbeType: &image, ImageProbeScheduleEnabled: explicit})
		require.NoError(t, err)
		require.Equal(t, explicit != nil, template.ImageProbeScheduleEnabled)
	}
}
