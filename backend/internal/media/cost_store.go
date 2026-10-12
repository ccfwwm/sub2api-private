package media

import (
	"context"
	"database/sql"
	"encoding/json"
)

// A verified cost is an administrator's bill reconciliation, never a retail
// fallback or a correction to the original customer settlement.
func (s *Store) VerifyCost(ctx context.Context, id, amount, evidence, idem string, expected, actor int64) error {
	if evidence == "" || len(evidence) > 2000 || idem == "" || len(idem) > 200 {
		return fail(400, "cost_evidence_required", "Cost reconciliation needs evidence and an operation key")
	}
	a, err := money(amount)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	t, err := lockTask(ctx, tx, id)
	if err != nil {
		return err
	}
	op := id + ":cost:" + idem
	var prior string
	err = tx.QueryRowContext(ctx, `SELECT amount::text FROM media_ledger_entries WHERE operation_key=$1`, op).Scan(&prior)
	if err == nil {
		p, _ := number(prior)
		if !p.Equal(a) {
			return fail(409, "idempotency_conflict", "Cost operation key already has a different amount")
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	if t.Version != expected {
		return fail(409, "version_conflict", "Refresh task before cost reconciliation")
	}
	raw, err := json.Marshal(map[string]any{"evidence": evidence, "actor_id": actor})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO media_cost_entries(task_id,status,amount,details) VALUES($1,'verified',$2,$3) ON CONFLICT(task_id) DO UPDATE SET status='verified',amount=excluded.amount,details=excluded.details,updated_at=now()`, id, a.StringFixed(8), raw)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO media_ledger_entries(operation_key,task_id,kind,amount,details,actor_id) VALUES($1,$2,'cost_verification',$3,$4,$5)`, op, id, a.StringFixed(8), raw, actor)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE media_tasks SET version=version+1 WHERE id=$1`, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
