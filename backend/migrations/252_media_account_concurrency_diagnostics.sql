-- Media admission is owned by accounts.concurrency. These optional snapshots
-- make local capacity rejections searchable in ops_error_logs without changing
-- the existing error log semantics for non-media requests.
ALTER TABLE ops_error_logs
  ADD COLUMN IF NOT EXISTS account_concurrency INT,
  ADD COLUMN IF NOT EXISTS account_in_flight INT,
  ADD COLUMN IF NOT EXISTS account_queued INT,
  ADD COLUMN IF NOT EXISTS account_manual_review INT;

COMMENT ON COLUMN ops_error_logs.account_concurrency IS 'Live media account concurrency at request admission';
COMMENT ON COLUMN ops_error_logs.account_in_flight IS 'Media tasks held by the account at request admission';
COMMENT ON COLUMN ops_error_logs.account_queued IS 'Queued media tasks held by the account at request admission';
COMMENT ON COLUMN ops_error_logs.account_manual_review IS 'Manual-review media tasks held by the account at request admission';
