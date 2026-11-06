export default {
  "title": "Probe prompt and latency",
  "target": "Configure",
  "defaults": "Monitor defaults",
  "inheritance": "Leave blank to inherit. Per-model values take precedence.",
  "prompt": "Probe prompt",
  "defaultPrompt": "Leave blank to use the current protocol default",
  "externalPrompt": "The complete request body controls the prompt. Edit it in the request body.",
  "normal_latency_ms": "Normal latency limit (seconds)",
  "request_timeout_ms": "HTTP request timeout (seconds)",
  "observation_timeout_ms": "Generation observation deadline (seconds)",
  "latencyHint": "Successful slow probes are degraded and still available. Async generation uses total duration; pending is excluded from failures.",
  "invalid": "Check timeout ranges and ensure normal latency is below the request or async observation deadline. Prompts allow up to 8192 bytes.",
  "noData": "No data",
  "readError": "Data unavailable",
  "stale": "Result is stale",
  "lastResult": "Result recorded",
  "samples": "Valid samples: {count}"
}
