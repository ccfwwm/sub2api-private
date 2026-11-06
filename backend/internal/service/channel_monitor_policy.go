package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/media"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type resolvedMonitorPolicy struct {
	Prompt             string
	NormalLatency      time.Duration
	RequestTimeout     time.Duration
	ObservationTimeout time.Duration
}

func resolveMonitorPolicy(c domain.MonitorProbeConfig, model, kind string) resolvedMonitorPolicy {
	p := resolvedMonitorPolicy{NormalLatency: 6 * time.Second, RequestTimeout: 45 * time.Second, ObservationTimeout: time.Hour}
	switch kind {
	case "image":
		p.NormalLatency = 90 * time.Second
		p.RequestTimeout = 180 * time.Second
	case "video":
		p.NormalLatency = 300 * time.Second
		p.RequestTimeout = 180 * time.Second
	case "audio":
		p.NormalLatency = 60 * time.Second
		p.RequestTimeout = 180 * time.Second
	}
	apply := func(v domain.MonitorProbePolicy) {
		if v.Prompt != nil {
			p.Prompt = *v.Prompt
		}
		if v.NormalLatencyMs != nil {
			p.NormalLatency = time.Duration(*v.NormalLatencyMs) * time.Millisecond
		}
		if v.RequestTimeoutMs != nil {
			p.RequestTimeout = time.Duration(*v.RequestTimeoutMs) * time.Millisecond
		}
		if v.ObservationTimeoutMs != nil {
			p.ObservationTimeout = time.Duration(*v.ObservationTimeoutMs) * time.Millisecond
		}
	}
	apply(c.Defaults)
	apply(c.Models[model])
	return p
}

func validateMonitorPolicy(c domain.MonitorProbeConfig, provider, probeType string, models []string, body map[string]any) error {
	invalid := func(s string) error { return infraerrors.BadRequest("CHANNEL_MONITOR_INVALID_PROBE_CONFIG", s) }
	raw, err := json.Marshal(c)
	if err != nil || len(raw) > 64<<10 || len(c.Models) > 20 {
		return invalid("probe_config exceeds size or model limit")
	}
	policies := []domain.MonitorProbePolicy{c.Defaults}
	for name, v := range c.Models {
		if strings.TrimSpace(name) == "" || len(name) > 200 {
			return invalid("invalid probe model name")
		}
		policies = append(policies, v)
	}
	for _, v := range policies {
		if v.Prompt != nil && (!utf8.ValidString(*v.Prompt) || len(*v.Prompt) > 8192) {
			return invalid("prompt must be valid UTF-8 and at most 8192 bytes")
		}
		if v.RequestTimeoutMs != nil && (*v.RequestTimeoutMs < 1000 || *v.RequestTimeoutMs > 600000) {
			return invalid("request_timeout_ms must be 1000-600000")
		}
		if v.NormalLatencyMs != nil && (*v.NormalLatencyMs < 1 || *v.NormalLatencyMs > 86400000) {
			return invalid("normal_latency_ms must be 1-86400000")
		}
		if v.ObservationTimeoutMs != nil && (*v.ObservationTimeoutMs < 15000 || *v.ObservationTimeoutMs > 86400000) {
			return invalid("observation_timeout_ms must be 15000-86400000")
		}
	}
	var mediaConfig MonitorMediaConfig
	if provider == MonitorProviderImageVideo {
		mediaConfig, _ = monitorMediaConfig(body)
		for model := range mediaConfig.Models {
			models = append(models, model)
		}
	}
	// Request templates do not know their eventual model list. Validate every
	// configured override as well as the models selected by a monitor.
	for model := range c.Models {
		models = append(models, model)
	}
	for _, model := range models {
		kind := defaultMonitorProbeType(probeType)
		async := false
		if provider == MonitorProviderImageVideo {
			if cap := mediaConfig.Models[model].Capability; cap != nil {
				kind = cap.MediaType
				// A saved template reference is resolved again before submission.
				// Its delivery mode cannot be inferred from the media type.
				async = cap.Template == nil && cap.TemplateRef != nil || cap.Template != nil && cap.Template.Mode == "async"
			}
		}
		p := resolveMonitorPolicy(c, model, kind)
		if !async && p.NormalLatency >= p.RequestTimeout {
			return invalid(fmt.Sprintf("%s: normal latency must be below request timeout", model))
		}
		if async && p.NormalLatency >= p.ObservationTimeout {
			return invalid(fmt.Sprintf("%s: normal latency must be below observation timeout", model))
		}
	}
	return nil
}

func (s *ChannelMonitorService) validateResolvedMonitorPolicy(ctx context.Context, c domain.MonitorProbeConfig, provider, probeType string, models []string, body map[string]any) error {
	if provider != MonitorProviderImageVideo || s.mediaTemplateStore == nil {
		return validateMonitorPolicy(c, provider, probeType, models, body)
	}
	config, err := monitorMediaConfig(body)
	if err != nil || config.Mode != "generation" {
		return err
	}
	for model, entry := range config.Models {
		if entry.Capability == nil {
			continue
		}
		resolved, err := s.mediaTemplateStore.ResolveTemplates(ctx, []media.Capability{*entry.Capability})
		if err != nil {
			return err
		}
		if len(resolved) == 1 {
			entry.Capability = &resolved[0]
			config.Models[model] = entry
		}
	}
	return validateMonitorPolicy(c, provider, probeType, models, map[string]any{"media_monitor": config})
}

func cloneMonitorPolicy(c domain.MonitorProbeConfig) domain.MonitorProbeConfig {
	b, _ := json.Marshal(c)
	var out domain.MonitorProbeConfig
	_ = json.Unmarshal(b, &out)
	return out
}

func monitorCheckLimit(m *ChannelMonitor) time.Duration {
	limit := time.Duration(0)
	for _, model := range normalizeModels(append([]string{m.PrimaryModel}, m.ExtraModels...)) {
		kind := defaultMonitorProbeType(m.ProbeType)
		if m.Provider == MonitorProviderImageVideo {
			if c, err := monitorMediaConfig(m.BodyOverride); err == nil {
				if cap := c.Models[model].Capability; cap != nil {
					kind = cap.MediaType
				}
			}
		}
		t := resolveMonitorPolicy(m.ProbeConfig, model, kind).RequestTimeout
		if m.Provider == MonitorProviderImageVideo {
			limit += resolveMonitorPolicy(m.ProbeConfig, model, kind).ObservationTimeout + 2*t + 10*time.Second
		} else if t > limit {
			limit = t
		}
	}
	return limit + monitorPingTimeout + 15*time.Second
}
