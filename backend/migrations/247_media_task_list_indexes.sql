-- Support task history without changing task, ledger or worker semantics.
CREATE INDEX IF NOT EXISTS media_tasks_created_id ON media_tasks(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS media_tasks_user_created_id ON media_tasks(user_id, created_at DESC, id DESC);
