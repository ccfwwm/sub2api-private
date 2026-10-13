-- Media monitoring reuses body_override JSON snapshots and existing history.
-- Additive enum expansion; no existing configuration or pricing is changed.
ALTER TABLE channel_monitors DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;
ALTER TABLE channel_monitors ADD CONSTRAINT channel_monitors_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity',
                        'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'imagevideo'));

ALTER TABLE channel_monitor_request_templates DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;
ALTER TABLE channel_monitor_request_templates ADD CONSTRAINT channel_monitor_request_templates_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity',
                        'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'imagevideo'));

ALTER TABLE channel_monitor_histories DROP CONSTRAINT IF EXISTS channel_monitor_histories_status_check;
ALTER TABLE channel_monitor_histories ADD CONSTRAINT channel_monitor_histories_status_check
    CHECK (status IN ('operational', 'degraded', 'failed', 'error', 'pending'));
