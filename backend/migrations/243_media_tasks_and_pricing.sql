-- Additive only. Existing prices, balances, logs and migrations are unchanged.
-- Independent counters cannot be overwritten by legacy Redis quota snapshots.
ALTER TABLE user_platform_quotas
 ADD COLUMN media_daily_usage_usd numeric(20,8) NOT NULL DEFAULT 0,
 ADD COLUMN media_weekly_usage_usd numeric(20,8) NOT NULL DEFAULT 0,
 ADD COLUMN media_monthly_usage_usd numeric(20,8) NOT NULL DEFAULT 0,
 ADD COLUMN media_daily_window_start timestamptz,
 ADD COLUMN media_weekly_window_start timestamptz,
 ADD COLUMN media_monthly_window_start timestamptz;

CREATE TABLE media_config_versions (
 kind text NOT NULL CHECK(kind IN ('capability','price')),
 id text NOT NULL,
 version bigint NOT NULL,
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','enabled','disabled')),
 config jsonb NOT NULL,
 actor_id bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(kind,id,version)
);
CREATE TABLE media_config_heads (
 kind text NOT NULL,
 id text NOT NULL,
 version bigint NOT NULL,
 published_version bigint,
 PRIMARY KEY(kind,id),
 FOREIGN KEY(kind,id,version) REFERENCES media_config_versions(kind,id,version)
);
CREATE TABLE media_tasks (
 id text PRIMARY KEY,
 user_id bigint NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 api_key_id bigint NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
 group_id bigint NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
 subscription_id bigint REFERENCES user_subscriptions(id) ON DELETE RESTRICT,
 account_id bigint NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
 operation text NOT NULL,
 media_type text NOT NULL,
 model text NOT NULL,
 idempotency_key text,
 request_hash text NOT NULL,
 snapshot jsonb NOT NULL,
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','submitting','in_progress','unknown','completed','failed')),
 settlement_state text NOT NULL DEFAULT 'held' CHECK(settlement_state IN ('held','pending_usage','settled','released','manual_review')),
 upstream_id text,
 reserved numeric(20,8) NOT NULL CHECK(reserved>=0),
 accepted numeric(20,8) NOT NULL DEFAULT 0 CHECK(accepted>=0),
 captured numeric(20,8) NOT NULL DEFAULT 0 CHECK(captured>=0),
 released numeric(20,8) NOT NULL DEFAULT 0 CHECK(released>=0),
 refunded numeric(20,8) NOT NULL DEFAULT 0 CHECK(refunded>=0),
 quantities jsonb NOT NULL DEFAULT '{}',
 result_path text,
 diagnostic text NOT NULL DEFAULT '',
 lease_until timestamptz,
 lease_epoch bigint NOT NULL DEFAULT 0,
 next_poll_at timestamptz NOT NULL DEFAULT now(),
 cache_dirty boolean NOT NULL DEFAULT true,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz,
 CHECK(accepted+captured+released<=reserved),
 CHECK(refunded<=accepted+captured),
 UNIQUE(api_key_id,operation,idempotency_key)
);
CREATE INDEX media_tasks_worker ON media_tasks(next_poll_at,lease_until) WHERE settlement_state IN ('held','pending_usage');
CREATE UNIQUE INDEX media_tasks_upstream ON media_tasks(account_id,upstream_id) WHERE upstream_id IS NOT NULL;
CREATE TABLE media_quota_windows (
 kind text NOT NULL,
 source_id bigint NOT NULL,
 window_start timestamptz NOT NULL,
 captured numeric(20,8) NOT NULL DEFAULT 0 CHECK(captured>=0),
 held numeric(20,8) NOT NULL DEFAULT 0 CHECK(held>=0),
 PRIMARY KEY(kind,source_id,window_start)
);
CREATE TABLE media_quota_holds (
 task_id text NOT NULL REFERENCES media_tasks(id) ON DELETE RESTRICT,
 kind text NOT NULL,
 source_id bigint NOT NULL,
 window_start timestamptz NOT NULL,
 amount numeric(20,8) NOT NULL CHECK(amount>=0),
 remaining numeric(20,8) NOT NULL CHECK(remaining>=0),
 PRIMARY KEY(task_id,kind),
 FOREIGN KEY(kind,source_id,window_start) REFERENCES media_quota_windows(kind,source_id,window_start)
);
CREATE TABLE media_ledger_entries (
 operation_key text PRIMARY KEY,
 task_id text NOT NULL REFERENCES media_tasks(id) ON DELETE RESTRICT,
 kind text NOT NULL,
 amount numeric(20,8) NOT NULL CHECK(amount>=0),
 details jsonb NOT NULL DEFAULT '{}',
 actor_id bigint,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE media_artifacts (
 task_id text NOT NULL REFERENCES media_tasks(id) ON DELETE RESTRICT,
 key text NOT NULL,
 media_type text NOT NULL,
 mime_type text NOT NULL,
 size_bytes bigint,
 duration_seconds numeric,
 content_hash text,
 source_encrypted bytea,
 file_path text,
 available boolean NOT NULL DEFAULT false,
 PRIMARY KEY(task_id,key),
 UNIQUE(task_id,content_hash)
);
CREATE TABLE media_cost_entries (
 task_id text PRIMARY KEY REFERENCES media_tasks(id) ON DELETE RESTRICT,
 status text NOT NULL CHECK(status IN ('unknown','estimated','verified')),
 amount numeric(20,8),
 details jsonb NOT NULL DEFAULT '{}',
 updated_at timestamptz NOT NULL DEFAULT now()
);
