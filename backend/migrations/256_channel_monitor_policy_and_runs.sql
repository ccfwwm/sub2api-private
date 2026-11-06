-- Additive monitor configuration and durable execution receipts.
ALTER TABLE channel_monitors ADD COLUMN probe_config jsonb;
ALTER TABLE channel_monitor_request_templates ADD COLUMN probe_config jsonb;
ALTER TABLE channel_monitor_histories ADD COLUMN sample_id varchar(100);
ALTER TABLE channel_monitor_histories ADD COLUMN request_latency_ms integer;
CREATE UNIQUE INDEX channel_monitor_history_sample ON channel_monitor_histories(monitor_id,model,sample_id);
CREATE TABLE channel_monitor_runs (
 run_id uuid PRIMARY KEY,
 monitor_id bigint NOT NULL REFERENCES channel_monitors(id) ON DELETE CASCADE,
 state varchar(20) NOT NULL CHECK(state IN ('running','completed','failed','interrupted')),
 results jsonb NOT NULL DEFAULT '[]',
 error varchar(500) NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '24 hours'
);
CREATE INDEX channel_monitor_runs_expiry ON channel_monitor_runs(expires_at);
CREATE UNIQUE INDEX channel_monitor_runs_active ON channel_monitor_runs(monitor_id) WHERE state='running';
