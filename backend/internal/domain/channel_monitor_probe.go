package domain

// MonitorProbePolicy uses milliseconds on the wire. Nil values inherit defaults.
type MonitorProbePolicy struct {
	Prompt               *string `json:"prompt,omitempty"`
	NormalLatencyMs      *int    `json:"normal_latency_ms,omitempty"`
	RequestTimeoutMs     *int    `json:"request_timeout_ms,omitempty"`
	ObservationTimeoutMs *int    `json:"observation_timeout_ms,omitempty"`
}

type MonitorProbeConfig struct {
	Defaults MonitorProbePolicy            `json:"defaults"`
	Models   map[string]MonitorProbePolicy `json:"models,omitempty"`
}
