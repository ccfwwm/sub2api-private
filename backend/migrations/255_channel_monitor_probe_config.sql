-- Probe configuration is snapshotted alongside existing template request settings.
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS probe_type VARCHAR(16) NOT NULL DEFAULT 'text',
    ADD COLUMN IF NOT EXISTS image_probe_schedule_enabled BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS custom_method VARCHAR(10) NOT NULL DEFAULT 'POST',
    ADD COLUMN IF NOT EXISTS custom_path VARCHAR(500) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS custom_body JSONB NULL,
    ADD COLUMN IF NOT EXISTS custom_response_json_path VARCHAR(300) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS custom_response_require_non_empty BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE channel_monitor_request_templates
    ADD COLUMN IF NOT EXISTS probe_type VARCHAR(16) NOT NULL DEFAULT 'text',
    ADD COLUMN IF NOT EXISTS image_probe_schedule_enabled BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS custom_method VARCHAR(10) NOT NULL DEFAULT 'POST',
    ADD COLUMN IF NOT EXISTS custom_path VARCHAR(500) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS custom_body JSONB NULL,
    ADD COLUMN IF NOT EXISTS custom_response_json_path VARCHAR(300) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS custom_response_require_non_empty BOOLEAN NOT NULL DEFAULT true;
