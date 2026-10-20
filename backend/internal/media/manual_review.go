package media

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// ManualReview is an explicit administrative override, separate from automatic
// pricing. Target is the final net task charge, never an additional charge.
type ManualReview struct {
	Status  string `json:"status"`
	Target  string `json:"target_charge"`
	Reason  string `json:"reason"`
	Version int64  `json:"expected_version"`
}

func (s *Store) ReviewTask(ctx context.Context, id, idem string, input ManualReview, actor int64) error {
	if actor <= 0 || idem == "" || len(idem) > 200 || strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 2000 {
		return fail(400, "review_identity_required", "请填写核对原因和幂等键")
	}
	if input.Status != "failed" && input.Status != "completed" {
		return fail(400, "invalid_decision", "请选择失败或已完成")
	}
	target, err := money(input.Target)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t, err := lockTask(ctx, tx, id)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(input)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(raw))
	op := id + ":manual:" + idem
	var previous string
	err = tx.QueryRowContext(ctx, `SELECT details->>'fingerprint' FROM media_ledger_entries WHERE operation_key=$1`, op).Scan(&previous)
	if err == nil {
		if previous != fingerprint {
			return fail(409, "idempotency_conflict", "相同幂等键已用于其他核对内容")
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	if t.Version != input.Version {
		return fail(409, "version_conflict", "任务已更新，请刷新后重新核对")
	}
	reserved, _ := number(t.Reserved)
	if target.GreaterThan(reserved) {
		return fail(400, "charge_exceeds_maximum", "人工扣费不能超过任务原有最高收费")
	}
	accepted, _ := number(t.Accepted)
	captured, _ := number(t.Captured)
	refunded, _ := number(t.Refunded)
	net := accepted.Add(captured).Sub(refunded)
	delta := target.Sub(net)
	held := decimal.Zero
	if t.Settlement != "settled" && t.Settlement != "released" {
		held = reserved.Sub(accepted)
	}
	if t.Identity.SubscriptionID == 0 {
		// Releasing a hold and charging/refunding happen in one locked transaction.
		r, e := tx.ExecContext(ctx, `UPDATE users SET frozen_balance=frozen_balance-$1,balance=balance+$1-$2,updated_at=now() WHERE id=$3 AND frozen_balance >= $1 AND balance+$1 >= $2`, held.StringFixed(8), delta.StringFixed(8), t.Identity.UserID)
		if requireOne(r, e) != nil {
			return fail(409, "review_balance_conflict", "可用余额不足或预留金额已变更")
		}
	}
	if err = reviewQuota(ctx, tx, id, held, delta); err != nil {
		return err
	}
	// Recharging a previously returned task first reverses its current refund
	// balance, then consumes unused released capacity. Original ledger events
	// remain append-only; task aggregates continue to satisfy the reserve cap.
	restoredRefund := decimal.Min(decimal.Max(delta, decimal.Zero), refunded)
	newCaptured := captured.Add(decimal.Max(delta, decimal.Zero).Sub(restoredRefund))
	newRefunded := refunded.Sub(restoredRefund).Add(decimal.Max(delta.Neg(), decimal.Zero))
	newReleased := reserved.Sub(accepted).Sub(newCaptured)
	settlement := "settled"
	if target.IsZero() {
		settlement = "released"
	}
	details, _ := json.Marshal(map[string]any{"fingerprint": fingerprint, "status": input.Status, "target_charge": target.StringFixed(8), "previous_net": net.StringFixed(8), "reason": input.Reason, "released_hold": held.StringFixed(8), "before": map[string]string{"captured": t.Captured, "refunded": t.Refunded, "released": t.Released}, "after": map[string]string{"captured": newCaptured.StringFixed(8), "refunded": newRefunded.StringFixed(8), "released": newReleased.StringFixed(8)}})
	_, err = tx.ExecContext(ctx, `INSERT INTO media_ledger_entries(operation_key,task_id,kind,amount,details,actor_id) VALUES($1,$2,'manual_review',$3,$4,$5)`, op, id, delta.Abs().StringFixed(8), details, actor)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE media_tasks SET state=$1,settlement_state=$2,captured=$3,refunded=$4,released=$5,diagnostic=$6,completed_at=coalesce(completed_at,now()),lease_until=NULL,lease_epoch=lease_epoch+1,version=version+1,cache_dirty=true WHERE id=$7`, input.Status, settlement, newCaptured.StringFixed(8), newRefunded.StringFixed(8), newReleased.StringFixed(8), "管理员核对："+input.Reason, id)
	if err != nil {
		return err
	}
	// Usage counters reflect the reviewed net amount; original ledger entries,
	// request, capability snapshot and price version remain intact.
	if err = writeUsage(ctx, tx, t, target); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE usage_logs SET total_cost=$1,actual_cost=$1 WHERE request_id=$2 AND api_key_id=$3`, target.StringFixed(8), id, t.Identity.KeyID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE api_keys SET status='active',updated_at=now() WHERE id=$1 AND status='quota_exhausted' AND quota>0 AND quota_used<quota`, t.Identity.KeyID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func reviewQuota(ctx context.Context, tx *sql.Tx, id string, held, delta decimal.Decimal) error {
	rows, err := tx.QueryContext(ctx, `SELECT kind,source_id,window_start,remaining::text FROM media_quota_holds WHERE task_id=$1 ORDER BY kind FOR UPDATE`, id)
	if err != nil {
		return err
	}
	type hold struct {
		kind      string
		source    int64
		window    any
		remaining string
	}
	holds := []hold{}
	for rows.Next() {
		var h hold
		if err = rows.Scan(&h.kind, &h.source, &h.window, &h.remaining); err != nil {
			rows.Close()
			return err
		}
		holds = append(holds, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, h := range holds {
		remaining, _ := number(h.remaining)
		if !remaining.Equal(held) {
			return fail(409, "review_hold_conflict", "任务额度预留不一致，请核查账务")
		}
		r, e := tx.ExecContext(ctx, `UPDATE media_quota_windows SET held=held-$1,captured=captured+$2 WHERE kind=$3 AND source_id=$4 AND window_start=$5 AND held >= $1 AND captured+$2 >= 0`, held.StringFixed(8), delta.StringFixed(8), h.kind, h.source, h.window)
		if err = requireOne(r, e); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE media_quota_holds SET remaining=0 WHERE task_id=$1 AND kind=$2`, id, h.kind)
		if err != nil {
			return err
		}
		table, used, start := quotaColumns(h.kind)
		if table == "" {
			return fmt.Errorf("unknown quota hold")
		}
		query := `UPDATE ` + table + ` SET ` + used + `=greatest(0,` + used + `+$1) WHERE id=$2`
		args := []any{delta.Sub(held).StringFixed(8), h.source}
		if start != "" {
			query += ` AND ` + start + `=$3`
			args = append(args, h.window)
		}
		if _, err = tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}
