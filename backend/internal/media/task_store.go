package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type StoredTask struct {
	ID, State, Settlement, UpstreamID, ResultPath, Diagnostic string
	Identity                                                  Identity
	AccountID                                                 int64
	Snapshot                                                  Snapshot
	Quantities                                                map[string][]string
	Reserved, Accepted, Captured, Released, Refunded          string
	Version, Epoch                                            int64
	Created                                                   time.Time
	Completed                                                 *time.Time
}

const taskSelect = `id,state,settlement_state,coalesce(upstream_id,''),coalesce(result_path,''),diagnostic,user_id,api_key_id,group_id,coalesce(subscription_id,0),account_id,snapshot,quantities,reserved::text,accepted::text,captured::text,released::text,refunded::text,version,lease_epoch,created_at,completed_at`

func scanTask(row interface{ Scan(...any) error }) (StoredTask, error) {
	var t StoredTask
	var snap, q []byte
	e := row.Scan(&t.ID, &t.State, &t.Settlement, &t.UpstreamID, &t.ResultPath, &t.Diagnostic, &t.Identity.UserID, &t.Identity.KeyID, &t.Identity.GroupID, &t.Identity.SubscriptionID, &t.AccountID, &snap, &q, &t.Reserved, &t.Accepted, &t.Captured, &t.Released, &t.Refunded, &t.Version, &t.Epoch, &t.Created, &t.Completed)
	if e != nil {
		return t, e
	}
	if e = json.Unmarshal(snap, &t.Snapshot); e != nil {
		return t, e
	}
	t.Identity = t.Snapshot.Identity
	e = json.Unmarshal(q, &t.Quantities)
	return t, e
}
func (s *Store) GetTask(ctx context.Context, id string) (StoredTask, error) {
	t, e := scanTask(s.DB.QueryRowContext(ctx, `SELECT `+taskSelect+` FROM media_tasks WHERE id=$1`, id))
	if e == sql.ErrNoRows {
		return t, fail(404, "task_not_found", "Task does not exist")
	}
	return t, e
}
func (s *Store) Existing(ctx context.Context, keyID int64, op, idem, hash string) (*StoredTask, error) {
	if idem == "" {
		return nil, nil
	}
	t, e := scanTask(s.DB.QueryRowContext(ctx, `SELECT `+taskSelect+` FROM media_tasks WHERE api_key_id=$1 AND operation=$2 AND idempotency_key=$3`, keyID, op, idem))
	if e == sql.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var existing string
	e = s.DB.QueryRowContext(ctx, `SELECT request_hash FROM media_tasks WHERE id=$1`, t.ID).Scan(&existing)
	if e != nil {
		return nil, e
	}
	if existing != hash {
		return nil, fail(409, "idempotency_conflict", "Same key with different parameters")
	}
	return &t, nil
}

func (s *Store) RecoverTask(ctx context.Context, keyID int64, op, idem string) (StoredTask, error) {
	if idem == "" || len(idem) > 200 || !contains([]string{"image", "image_edit", "video", "audio", "chat_music", "gemini_image"}, op) {
		return StoredTask{}, fail(400, "invalid_recovery", "operation and idempotency_key are required")
	}
	t, err := scanTask(s.DB.QueryRowContext(ctx, `SELECT `+taskSelect+` FROM media_tasks WHERE api_key_id=$1 AND operation=$2 AND idempotency_key=$3`, keyID, op, idem))
	if err == sql.ErrNoRows {
		return t, fail(404, "task_not_found", "Original task not found; acceptance is not established")
	}
	return t, err
}

type quotaSpec struct {
	kind, table, used, limit, start string
	source                          int64
	period                          time.Duration
	limitValue                      *string
}

func (s *Store) CreateTask(ctx context.Context, snapshot Snapshot, accountID int64, idem, hash, id string) (StoredTask, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return StoredTask{}, e
	}
	defer func() { _ = tx.Rollback() }()
	identity := snapshot.Identity
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("billing-user:%d", identity.UserID)); e != nil {
		return StoredTask{}, e
	}
	if _, e = money(snapshot.Maximum); e != nil {
		return StoredTask{}, e
	}
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "media-profile:"+snapshot.Capability.ID); e != nil {
		return StoredTask{}, e
	}
	// Per-user lock serializes all new media holds. Existing balances use row-atomic updates.
	var status string
	var balance string
	e = tx.QueryRowContext(ctx, `SELECT status,balance::text FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, identity.UserID).Scan(&status, &balance)
	if e != nil {
		return StoredTask{}, e
	}
	if status != "active" {
		return StoredTask{}, fail(403, "user_disabled", "User disabled")
	}
	if idem != "" {
		existing, e := scanTask(tx.QueryRowContext(ctx, `SELECT `+taskSelect+` FROM media_tasks WHERE api_key_id=$1 AND operation=$2 AND idempotency_key=$3`, identity.KeyID, snapshot.Request.Operation, idem))
		if e == nil {
			var h string
			if e = tx.QueryRowContext(ctx, `SELECT request_hash FROM media_tasks WHERE id=$1`, existing.ID).Scan(&h); e != nil {
				return StoredTask{}, e
			}
			if h != hash {
				return StoredTask{}, fail(409, "idempotency_conflict", "Same key with different request")
			}
			return existing, nil
		}
		if e != sql.ErrNoRows {
			return StoredTask{}, e
		}
	}
	var subscriptionStatus string
	var expires time.Time
	if identity.SubscriptionID > 0 {
		e = tx.QueryRowContext(ctx, `SELECT status,expires_at FROM user_subscriptions WHERE id=$1 AND user_id=$2 AND group_id=$3 AND deleted_at IS NULL FOR UPDATE`, identity.SubscriptionID, identity.UserID, identity.GroupID).Scan(&subscriptionStatus, &expires)
		if e != nil {
			return StoredTask{}, e
		}
		if subscriptionStatus != "active" || !expires.After(time.Now()) {
			return StoredTask{}, fail(402, "subscription_expired", "Subscription not active")
		}
	}
	var keyStatus string
	var keyUser int64
	var keyGroup sql.NullInt64
	var keyExpires sql.NullTime
	e = tx.QueryRowContext(ctx, `SELECT status,user_id,group_id,expires_at FROM api_keys WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, identity.KeyID).Scan(&keyStatus, &keyUser, &keyGroup, &keyExpires)
	if e != nil {
		return StoredTask{}, e
	}
	if keyStatus != "active" || keyUser != identity.UserID || !keyGroup.Valid || keyGroup.Int64 != identity.GroupID || keyExpires.Valid && keyExpires.Time.Before(time.Now()) {
		return StoredTask{}, fail(403, "key_unavailable", "API Key changed or inactive")
	}
	var count int
	e = tx.QueryRowContext(ctx, `SELECT count(*) FROM media_tasks WHERE snapshot->'capability'->>'profile_id'=$1 AND settlement_state IN ('held','pending_usage','manual_review')`, snapshot.Capability.ID).Scan(&count)
	if e != nil {
		return StoredTask{}, e
	}
	if snapshot.Capability.Limits.MaxConcurrentTasks == nil || count >= *snapshot.Capability.Limits.MaxConcurrentTasks {
		return StoredTask{}, fail(429, "task_concurrency_limit", "Model concurrency exhausted")
	}
	rawSnap, e := json.Marshal(snapshot)
	if e != nil {
		return StoredTask{}, e
	}
	q, e := json.Marshal(snapshot.Request.Quantities)
	if e != nil {
		return StoredTask{}, e
	}
	var sub any
	if identity.SubscriptionID > 0 {
		sub = identity.SubscriptionID
	}
	var idemValue any
	if idem != "" {
		idemValue = idem
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO media_tasks(id,user_id,api_key_id,group_id,subscription_id,account_id,operation,media_type,model,idempotency_key,request_hash,snapshot,quantities,reserved) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, identity.UserID, identity.KeyID, identity.GroupID, sub, accountID, snapshot.Request.Operation, snapshot.Capability.MediaType, snapshot.Capability.Model, idemValue, hash, rawSnap, q, snapshot.Maximum)
	if e != nil {
		return StoredTask{}, e
	}
	if identity.SubscriptionID == 0 {
		if s.LegacyBalanceHeld != nil {
			held, err := s.LegacyBalanceHeld(ctx, identity.UserID)
			if err != nil {
				return StoredTask{}, fail(503, "legacy_reservation_unavailable", "Cannot reconcile HTTP reservations")
			}
			legacy, err := number(held)
			if err != nil {
				return StoredTask{}, err
			}
			available, _ := number(balance)
			maximum, _ := number(snapshot.Maximum)
			if available.Sub(legacy).LessThan(maximum) {
				return StoredTask{}, fail(402, "insufficient_funds", "Available balance excludes HTTP reservations")
			}
		}
		result, e := tx.ExecContext(ctx, `UPDATE users SET balance=balance-$1,frozen_balance=frozen_balance+$1,updated_at=now() WHERE id=$2 AND balance >= $1`, snapshot.Maximum, identity.UserID)
		if e != nil {
			return StoredTask{}, e
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return StoredTask{}, fail(402, "insufficient_funds", "Available balance is insufficient")
		}
	}
	specs := []quotaSpec{{"key_total", "api_keys", "quota_used", "quota", "", identity.KeyID, 0, nil}, {"key_5h", "api_keys", "usage_5h", "rate_limit_5h", "window_5h_start", identity.KeyID, 5 * time.Hour, nil}, {"key_1d", "api_keys", "usage_1d", "rate_limit_1d", "window_1d_start", identity.KeyID, 24 * time.Hour, nil}, {"key_7d", "api_keys", "usage_7d", "rate_limit_7d", "window_7d_start", identity.KeyID, 7 * 24 * time.Hour, nil}}
	if identity.SubscriptionID > 0 {
		var daily, weekly, monthly *string
		e = tx.QueryRowContext(ctx, `SELECT daily_limit_usd::text,weekly_limit_usd::text,monthly_limit_usd::text FROM groups WHERE id=$1`, identity.GroupID).Scan(&daily, &weekly, &monthly)
		if e != nil {
			return StoredTask{}, e
		}
		for i, part := range []string{"daily", "weekly", "monthly"} {
			limits := []*string{daily, weekly, monthly}
			periods := []time.Duration{24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour}
			specs = append(specs, quotaSpec{"subscription_" + part, "user_subscriptions", part + "_usage_usd", "", part + "_window_start", identity.SubscriptionID, periods[i], limits[i]})
		}
	}
	var platformID int64
	e = tx.QueryRowContext(ctx, `SELECT id FROM user_platform_quotas WHERE user_id=$1 AND platform=$2 AND deleted_at IS NULL FOR UPDATE`, identity.UserID, identity.Platform).Scan(&platformID)
	if e != nil && e != sql.ErrNoRows {
		return StoredTask{}, e
	}
	if e == nil {
		for i, part := range []string{"daily", "weekly", "monthly"} {
			periods := []time.Duration{24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour}
			specs = append(specs, quotaSpec{"platform_" + part, "user_platform_quotas", "media_" + part + "_usage_usd", part + "_limit_usd", "media_" + part + "_window_start", platformID, periods[i], nil})
		}
	}
	for _, spec := range specs {
		if e = reserveQuota(ctx, tx, id, spec, snapshot.Maximum, identity.PlatformBaseline); e != nil {
			return StoredTask{}, e
		}
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO media_ledger_entries(operation_key,task_id,kind,amount) VALUES($1,$2,'reserve',$3)`, id+":reserve", id, snapshot.Maximum)
	if e != nil {
		return StoredTask{}, e
	}
	if e = tx.Commit(); e != nil {
		return StoredTask{}, e
	}
	return s.GetTask(ctx, id)
}
func reserveQuota(ctx context.Context, tx *sql.Tx, taskID string, q quotaSpec, amount string, platformBaseline map[string]string) error {
	var used string
	var limit *string
	var start sql.NullTime
	startCol := "NULL::timestamptz"
	if q.start != "" {
		startCol = q.start
	}
	limitCol := "NULL::text"
	if q.limit != "" {
		limitCol = q.limit + "::text"
	}
	e := tx.QueryRowContext(ctx, `SELECT `+q.used+`::text,`+limitCol+`,`+startCol+` FROM `+q.table+` WHERE id=$1 FOR UPDATE`, q.source).Scan(&used, &limit, &start)
	if e != nil {
		return e
	}
	if q.limitValue != nil {
		limit = q.limitValue
	}
	now := time.Now().UTC()
	window := time.Unix(0, 0).UTC()
	reset := false
	if q.period > 0 {
		if start.Valid {
			window = start.Time
		}
		if !start.Valid || !window.Add(q.period).After(now) {
			window = now
			used = "0"
			reset = true
		}
		if strings.HasSuffix(q.kind, "_daily") {
			today := timezone.StartOfDay(now)
			reset = !start.Valid || today.After(timezone.StartOfDay(start.Time))
			if q.table == "user_subscriptions" && start.Valid {
				var begins, expires time.Time
				if e := tx.QueryRowContext(ctx, `SELECT starts_at,expires_at FROM user_subscriptions WHERE id=$1`, q.source).Scan(&begins, &expires); e != nil {
					return e
				}
				if !expires.After(begins.AddDate(0, 0, 1)) {
					reset = false
				}
			}
			if reset {
				window = today
				used = "0"
			}
		}
		if q.kind == "platform_weekly" {
			week := timezone.StartOfWeek(now)
			reset = !start.Valid || start.Time.Before(week)
			if reset {
				window = week
				used = "0"
			}
		}
		if q.table == "user_subscriptions" && (q.kind == "subscription_weekly" || q.kind == "subscription_monthly") {
			var begins, expires time.Time
			if e := tx.QueryRowContext(ctx, `SELECT starts_at,expires_at FROM user_subscriptions WHERE id=$1`, q.source).Scan(&begins, &expires); e != nil {
				return e
			}
			reset = false
			if !start.Valid {
				window = begins
				used = "0"
				reset = true
			} else {
				anchor := start.Time
				if anchor.Equal(timezone.StartOfDay(begins)) && anchor.Before(begins) {
					anchor = begins
				}
				next := anchor.Add(q.period)
				if !now.Before(next) && next.Before(expires) {
					periods := now.Sub(anchor) / q.period
					last := (expires.Sub(anchor) - 1) / q.period
					if periods > last {
						periods = last
					}
					window = anchor.Add(periods * q.period)
					used = "0"
					reset = true
				} else {
					window = start.Time
				}
			}
		}
	}
	u, e := number(used)
	if e != nil {
		return e
	}
	a, e := number(amount)
	if e != nil {
		return e
	}
	if limit != nil {
		l, e := number(*limit)
		if e != nil {
			return e
		}
		isUnlimited := q.table == "api_keys" && l.IsZero()
		baseline := decimal.Zero
		if q.table == "user_platform_quotas" {
			var value string
			var oldStart sql.NullTime
			part := strings.TrimPrefix(q.kind, "platform_")
			if v, ok := platformBaseline[part]; ok {
				baseline, e = number(v)
				if e != nil {
					return e
				}
			} else {
				if e = tx.QueryRowContext(ctx, `SELECT `+part+`_usage_usd::text,`+part+`_window_start FROM user_platform_quotas WHERE id=$1`, q.source).Scan(&value, &oldStart); e != nil {
					return e
				}
				valid := oldStart.Valid
				if part == "daily" {
					valid = valid && !oldStart.Time.Before(timezone.StartOfDay(now))
				}
				if part == "weekly" {
					valid = valid && !oldStart.Time.Before(timezone.StartOfWeek(now))
				}
				if part == "monthly" {
					valid = valid && now.Sub(oldStart.Time) < 30*24*time.Hour
				}
				if valid {
					baseline, e = number(value)
					if e != nil {
						return e
					}
				}
			}
		}
		if !isUnlimited && (l.IsZero() || u.Add(baseline).Add(a).GreaterThan(l)) {
			return fail(402, "quota_exceeded", "In-flight usage would exceed "+q.kind)
		}
	}
	query := `UPDATE ` + q.table + ` SET ` + q.used + `=` + q.used + `+$1`
	if reset {
		query = `UPDATE ` + q.table + ` SET ` + q.used + `=$1,` + q.start + `=$3`
	}
	query += ` WHERE id=$2`
	if reset {
		_, e = tx.ExecContext(ctx, query, amount, q.source, window)
	} else {
		_, e = tx.ExecContext(ctx, query, amount, q.source)
	}
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO media_quota_windows(kind,source_id,window_start,held) VALUES($1,$2,$3,$4) ON CONFLICT(kind,source_id,window_start) DO UPDATE SET held=media_quota_windows.held+excluded.held`, q.kind, q.source, window, amount)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO media_quota_holds(task_id,kind,source_id,window_start,amount,remaining) VALUES($1,$2,$3,$4,$5,$5)`, taskID, q.kind, q.source, window, amount)
	return e
}

// Claim uses database leases; a stale submitting lease is never resubmitted.
func (s *Store) Claim(ctx context.Context) (*StoredTask, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer func() { _ = tx.Rollback() }()
	t, e := scanTask(tx.QueryRowContext(ctx, `SELECT `+taskSelect+` FROM media_tasks WHERE settlement_state IN ('held','pending_usage') AND next_poll_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY next_poll_at LIMIT 1 FOR UPDATE SKIP LOCKED`))
	if e == sql.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if t.State == "submitting" {
		_, e = tx.ExecContext(ctx, `UPDATE media_tasks SET state='unknown',settlement_state='manual_review',diagnostic='Submission interrupted; acceptance requires reconciliation',version=version+1 WHERE id=$1`, t.ID)
		if e != nil {
			return nil, e
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
		return nil, nil
	}
	_, e = tx.ExecContext(ctx, `UPDATE media_tasks SET state=CASE WHEN state='queued' THEN 'submitting' ELSE state END,lease_until=now()+interval '4 minutes',lease_epoch=lease_epoch+1,version=version+1 WHERE id=$1`, t.ID)
	if e != nil {
		return nil, e
	}
	t.Epoch++
	if t.State == "queued" {
		t.State = "submitting"
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return &t, nil
}
func (s *Store) Progress(ctx context.Context, t StoredTask, result UpstreamResult, path string) error {
	if result.Quantities == nil {
		result.Quantities = map[string][]string{}
	}
	q, _ := json.Marshal(result.Quantities)
	status := result.Status
	settlement := "held"
	if status == "unknown" {
		settlement = "manual_review"
	}
	if status == "completed" {
		settlement = "pending_usage"
	}
	r, e := s.DB.ExecContext(ctx, `UPDATE media_tasks SET state=$1,settlement_state=$2,upstream_id=coalesce(nullif($3,''),upstream_id),result_path=coalesce(nullif($4,''),result_path),quantities=quantities||$5::jsonb,diagnostic=$6,lease_until=CASE WHEN $1='completed' THEN lease_until ELSE NULL END,next_poll_at=now()+make_interval(secs=>$7),completed_at=CASE WHEN $1 IN ('completed','failed') THEN coalesce(completed_at,now()) ELSE completed_at END,version=version+1 WHERE id=$8 AND lease_epoch=$9 AND settlement_state IN ('held','pending_usage')`, status, settlement, result.ID, path, q, result.Diagnostic, t.Snapshot.Capability.Delivery.PollSeconds, t.ID, t.Epoch)
	return requireOne(r, e)
}

func requireOne(r sql.Result, e error) error {
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return fail(409, "ledger_invariant", "Concurrent change or missing financial hold")
	}
	return nil
}
func lockTask(ctx context.Context, tx *sql.Tx, id string) (StoredTask, error) {
	var userID int64
	if e := tx.QueryRowContext(ctx, `SELECT user_id FROM media_tasks WHERE id=$1`, id).Scan(&userID); e != nil {
		return StoredTask{}, e
	}
	var ignored int64
	if e := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&ignored); e != nil {
		return StoredTask{}, e
	}
	return scanTask(tx.QueryRowContext(ctx, `SELECT `+taskSelect+` FROM media_tasks WHERE id=$1 FOR UPDATE`, id))
}
func (s *Store) Settle(ctx context.Context, id string, calc Calculation, event string, cost *Calculation) error {
	return s.settle(ctx, id, calc, event, cost, nil)
}

// Workers must present the lease that produced their calculation. Administrative
// reconciliation uses Settle after its own version check.
func (s *Store) SettleClaim(ctx context.Context, t StoredTask, calc Calculation, event string, cost *Calculation) error {
	return s.settle(ctx, t.ID, calc, event, cost, &t.Epoch)
}
func (s *Store) settle(ctx context.Context, id string, calc Calculation, event string, cost *Calculation, epoch *int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	t, e := lockTask(ctx, tx, id)
	if e != nil {
		return e
	}
	if t.Settlement == "settled" || t.Settlement == "released" {
		return nil
	}
	if epoch != nil && t.Epoch != *epoch {
		return fail(409, "stale_lease", "Worker lease has changed")
	}
	if len(calc.Missing) > 0 {
		_, e = tx.ExecContext(ctx, `UPDATE media_tasks SET settlement_state='pending_usage',next_poll_at=now()+interval '1 minute',lease_until=NULL WHERE id=$1`, id)
		if e != nil {
			return e
		}
		return tx.Commit()
	}
	total, e := money(calc.Total)
	if e != nil {
		return e
	}
	reserved, _ := number(t.Reserved)
	accepted, _ := number(t.Accepted)
	total = decimal.Max(total, accepted)
	if total.GreaterThan(reserved) {
		total = reserved
	}
	acceptedEvent := event == "accepted"
	capture := total.Sub(accepted)
	if acceptedEvent {
		capture = total.Sub(accepted)
	}
	capture = decimal.Max(decimal.Zero, capture)
	if acceptedEvent && accepted.GreaterThanOrEqual(total) {
		return nil
	}
	if !acceptedEvent && t.State != "completed" && t.State != "failed" {
		return fail(409, "not_terminal", "Task is not terminal")
	}
	remaining := reserved.Sub(accepted)
	refundHold := decimal.Zero
	if !acceptedEvent {
		refundHold = remaining.Sub(capture)
	}
	op := id + ":finalize"
	if acceptedEvent {
		op = id + ":accepted"
	}
	details, _ := json.Marshal(calc)
	r, e := tx.ExecContext(ctx, `INSERT INTO media_ledger_entries(operation_key,task_id,kind,amount,details) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, op, id, event, capture.StringFixed(8), details)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return nil
	}
	if t.Identity.SubscriptionID == 0 {
		r, err := tx.ExecContext(ctx, `UPDATE users SET frozen_balance=frozen_balance-$1,balance=balance+$2,updated_at=now() WHERE id=$3 AND frozen_balance >= $1`, capture.Add(refundHold).StringFixed(8), refundHold.StringFixed(8), t.Identity.UserID)
		e = requireOne(r, err)
		if e != nil {
			return e
		}
	}
	if e = adjustQuotaHolds(ctx, tx, id, capture, refundHold, false); e != nil {
		return e
	}
	if acceptedEvent {
		_, e = tx.ExecContext(ctx, `UPDATE media_tasks SET accepted=accepted+$1,cache_dirty=true,version=version+1 WHERE id=$2`, capture.StringFixed(8), id)
	} else {
		state := "settled"
		if total.IsZero() {
			state = "released"
		}
		_, e = tx.ExecContext(ctx, `UPDATE media_tasks SET captured=$1,released=$2,settlement_state=$3,cache_dirty=true,lease_until=NULL,version=version+1 WHERE id=$4`, capture.StringFixed(8), refundHold.StringFixed(8), state, id)
		if e == nil {
			_, e = tx.ExecContext(ctx, `INSERT INTO media_ledger_entries(operation_key,task_id,kind,amount) VALUES($1,$2,'release',$3)`, id+":release", id, refundHold.StringFixed(8))
		}
		if e == nil {
			e = writeUsage(ctx, tx, t, total)
		}
	}
	if e != nil {
		return e
	}
	status := "unknown"
	if _, e = tx.ExecContext(ctx, `UPDATE api_keys SET status='active',updated_at=now() WHERE id=$1 AND status='quota_exhausted' AND quota>0 AND quota_used<quota`, t.Identity.KeyID); e != nil {
		return e
	}
	var amount any
	costDetails := []byte(`{}`)
	if cost != nil && len(cost.Missing) == 0 {
		status = "estimated"
		amount = cost.Total
		costDetails, _ = json.Marshal(cost)
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO media_cost_entries(task_id,status,amount,details) VALUES($1,$2,$3,$4) ON CONFLICT(task_id) DO UPDATE SET status=CASE WHEN media_cost_entries.status='verified' THEN 'verified' WHEN excluded.status='unknown' THEN media_cost_entries.status ELSE excluded.status END,amount=CASE WHEN media_cost_entries.status='verified' THEN media_cost_entries.amount ELSE greatest(media_cost_entries.amount,excluded.amount) END,details=CASE WHEN excluded.status='unknown' OR media_cost_entries.status='verified' THEN media_cost_entries.details ELSE excluded.details END,updated_at=now()`, id, status, amount, costDetails)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func quotaColumns(kind string) (table, used, start string) {
	switch kind {
	case "key_total":
		return "api_keys", "quota_used", ""
	case "key_5h":
		return "api_keys", "usage_5h", "window_5h_start"
	case "key_1d":
		return "api_keys", "usage_1d", "window_1d_start"
	case "key_7d":
		return "api_keys", "usage_7d", "window_7d_start"
	}
	for _, part := range []string{"daily", "weekly", "monthly"} {
		if kind == "subscription_"+part {
			return "user_subscriptions", part + "_usage_usd", part + "_window_start"
		}
		if kind == "platform_"+part {
			return "user_platform_quotas", "media_" + part + "_usage_usd", "media_" + part + "_window_start"
		}
	}
	return "", "", ""
}
func adjustQuotaHolds(ctx context.Context, tx *sql.Tx, id string, capture, release decimal.Decimal, refund bool) error {
	rows, e := tx.QueryContext(ctx, `SELECT kind,source_id,window_start FROM media_quota_holds WHERE task_id=$1 ORDER BY kind FOR UPDATE`, id)
	if e != nil {
		return e
	}
	type hold struct {
		kind   string
		source int64
		window time.Time
	}
	holds := []hold{}
	for rows.Next() {
		var h hold
		if e = rows.Scan(&h.kind, &h.source, &h.window); e != nil {
			_ = rows.Close()
			return e
		}
		holds = append(holds, h)
	}
	e = rows.Err()
	_ = rows.Close()
	if e != nil {
		return e
	}
	for _, h := range holds {
		var result sql.Result
		if refund {
			result, e = tx.ExecContext(ctx, `UPDATE media_quota_windows SET captured=captured-$1 WHERE kind=$2 AND source_id=$3 AND window_start=$4 AND captured >= $1`, release.StringFixed(8), h.kind, h.source, h.window)
		} else {
			result, e = tx.ExecContext(ctx, `UPDATE media_quota_windows SET held=held-$1,captured=captured+$2 WHERE kind=$3 AND source_id=$4 AND window_start=$5 AND held >= $1`, capture.Add(release).StringFixed(8), capture.StringFixed(8), h.kind, h.source, h.window)
			if e = requireOne(result, e); e != nil {
				return e
			}
			if e == nil {
				result, e = tx.ExecContext(ctx, `UPDATE media_quota_holds SET remaining=remaining-$1 WHERE task_id=$2 AND kind=$3 AND remaining >= $1`, capture.Add(release).StringFixed(8), id, h.kind)
			}
		}
		if e = requireOne(result, e); e != nil {
			return e
		}
		table, used, start := quotaColumns(h.kind)
		if table == "" {
			return fmt.Errorf("unknown quota hold")
		}
		query := `UPDATE ` + table + ` SET ` + used + `=greatest(0,` + used + `-$1) WHERE id=$2`
		if start != "" {
			query += ` AND ` + start + `=$3`
			_, e = tx.ExecContext(ctx, query, release.StringFixed(8), h.source, h.window)
		} else {
			_, e = tx.ExecContext(ctx, query, release.StringFixed(8), h.source)
		}
		if e != nil {
			return e
		}
	}
	return nil
}
func writeUsage(ctx context.Context, tx *sql.Tx, t StoredTask, total decimal.Decimal) error {
	var subscription any
	billingType := 0
	if t.Identity.SubscriptionID > 0 {
		subscription = t.Identity.SubscriptionID
		billingType = 1
	}
	_, e := tx.ExecContext(ctx, `INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,requested_model,upstream_model,group_id,subscription_id,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,input_cost,output_cost,cache_creation_cost,cache_read_cost,total_cost,actual_cost,rate_multiplier,billing_type,stream,billing_mode,created_at) VALUES($1,$2,$3,$4,$5,$5,$6,$7,$8,0,0,0,0,0,0,0,0,$9,$9,$10,$11,false,'per_request',now()) ON CONFLICT(request_id,api_key_id) DO NOTHING`, t.Identity.UserID, t.Identity.KeyID, t.AccountID, t.ID, t.Snapshot.Capability.Model, t.Snapshot.Capability.UpstreamModel, t.Identity.GroupID, subscription, total.StringFixed(8), t.Identity.Multiplier, billingType)
	return e
}
func (s *Store) Refund(ctx context.Context, id, idem, amount, reason string, expected, actor int64) error {
	if idem == "" || len(idem) > 200 || reason == "" {
		return fail(400, "refund_identity_required", "Refund idempotency key and reason required")
	}
	a, e := number(amount)
	if e != nil {
		return e
	}
	if !a.IsPositive() || a.Exponent() < -8 {
		return fail(400, "invalid_refund", "Positive amount with up to 8 decimals required")
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	t, e := lockTask(ctx, tx, id)
	if e != nil {
		return e
	}
	op := id + ":refund:" + idem
	var previous string
	e = tx.QueryRowContext(ctx, `SELECT amount::text FROM media_ledger_entries WHERE operation_key=$1`, op).Scan(&previous)
	if e == nil {
		p, _ := number(previous)
		if !p.Equal(a) {
			return fail(409, "idempotency_conflict", "Refund key already used for another amount")
		}
		return nil
	}
	if e != sql.ErrNoRows {
		return e
	}
	if t.Version != expected {
		return fail(409, "version_conflict", "Refresh task before refund")
	}
	if t.Settlement != "settled" {
		return fail(409, "not_settled", "Only settled tasks may be refunded")
	}
	captured, _ := number(t.Captured)
	refunded, _ := number(t.Refunded)
	ceiling, e := refundableAmount(t, captured)
	if e != nil {
		return e
	}
	if a.GreaterThan(ceiling.Sub(refunded)) {
		return fail(400, "refund_exceeds_charge", "Refund exceeds refundable success charges")
	}
	details, _ := json.Marshal(map[string]string{"reason": reason})
	_, e = tx.ExecContext(ctx, `INSERT INTO media_ledger_entries(operation_key,task_id,kind,amount,details,actor_id) VALUES($1,$2,'refund',$3,$4,$5)`, op, id, a.StringFixed(8), details, actor)
	if e != nil {
		return e
	}
	if t.Identity.SubscriptionID == 0 {
		_, e = tx.ExecContext(ctx, `UPDATE users SET balance=balance+$1,updated_at=now() WHERE id=$2`, a.StringFixed(8), t.Identity.UserID)
		if e != nil {
			return e
		}
	}
	if e = adjustQuotaHolds(ctx, tx, id, decimal.Zero, a, true); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE api_keys SET status='active',updated_at=now() WHERE id=$1 AND status='quota_exhausted' AND quota>0 AND quota_used<quota`, t.Identity.KeyID); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE media_tasks SET refunded=refunded+$1,cache_dirty=true,version=version+1 WHERE id=$2`, a.StringFixed(8), id)
	if e != nil {
		return e
	}
	return tx.Commit()
}

// When the task maximum caps success charges, allocate the captured amount
// proportionally. Accepted charges are already excluded from Captured.
func refundableAmount(t StoredTask, captured decimal.Decimal) (decimal.Decimal, error) {
	rule := t.Snapshot.SaleRule
	eligible := Rule{}
	for _, charge := range rule.Charges {
		if charge.Trigger == "success" && charge.Refundable {
			eligible.Charges = append(eligible.Charges, charge)
		}
	}
	all, err := Calculate(rule, t.Quantities, t.Identity.Multiplier, t.Identity.TimeMultiplier, "success_only", nil)
	if err != nil || len(all.Missing) > 0 {
		return decimal.Zero, fail(409, "refund_usage_missing", "Original success quantities are unavailable")
	}
	part, err := Calculate(eligible, t.Quantities, t.Identity.Multiplier, t.Identity.TimeMultiplier, "success_only", nil)
	if err != nil || len(part.Missing) > 0 {
		return decimal.Zero, fail(409, "refund_usage_missing", "Original refundable quantities are unavailable")
	}
	total, _ := number(all.Total)
	refundable, _ := number(part.Total)
	if total.IsZero() {
		return decimal.Zero, nil
	}
	return decimal.Min(captured, captured.Mul(refundable).DivRound(total, 64).Truncate(8)), nil
}
func NewTaskID() string { return "mt_" + uuid.NewString() }
