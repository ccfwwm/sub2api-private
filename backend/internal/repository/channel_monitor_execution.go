package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// SaveCheckResults commits the receipt and freshness together. Replaying a saved
// media result updates its original sample instead of counting another attempt.
func (r *channelMonitorRepository) SaveCheckResults(ctx context.Context, id int64, rows []*service.ChannelMonitorHistoryRow, checked time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, v := range rows {
		var sample any
		if v.SampleID != "" {
			sample = v.SampleID
		}
		var quota any
		if v.Quota != nil {
			b, e := json.Marshal(v.Quota)
			if e != nil {
				return e
			}
			quota = string(b)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO channel_monitor_histories(monitor_id,model,status,latency_ms,ping_latency_ms,message,checked_at,quota,sample_id,request_latency_ms)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10)
		ON CONFLICT(monitor_id,model,sample_id) DO UPDATE SET status=excluded.status,latency_ms=excluded.latency_ms,
		ping_latency_ms=excluded.ping_latency_ms,message=excluded.message,quota=excluded.quota,request_latency_ms=excluded.request_latency_ms,checked_at=excluded.checked_at`, id, v.Model, v.Status, v.LatencyMs, v.PingLatencyMs, service.NormalizeMonitorMessage(v.Message), v.CheckedAt, quota, sample, v.RequestLatencyMs)
		if err != nil {
			return fmt.Errorf("save monitor sample: %w", err)
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE channel_monitors SET last_checked_at=$2 WHERE id=$1`, id, checked); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *channelMonitorRepository) CreateMonitorRun(ctx context.Context, v *service.MonitorRun) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM channel_monitor_runs WHERE expires_at<now()`)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO channel_monitor_runs(run_id,monitor_id,state,created_at,expires_at) VALUES($1,$2,'running',$3,$4)`, v.RunID, v.MonitorID, v.CreatedAt, v.ExpiresAt)
	return err
}
func (r *channelMonitorRepository) SaveMonitorRun(ctx context.Context, v *service.MonitorRun) error {
	b, err := json.Marshal(v.Results)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE channel_monitor_runs SET state=$2,results=$3::jsonb,error=$4 WHERE run_id=$1`, v.RunID, v.State, string(b), service.NormalizeMonitorMessage(v.Error))
	return err
}
func (r *channelMonitorRepository) GetMonitorRun(ctx context.Context, id int64, run string) (*service.MonitorRun, error) {
	v := &service.MonitorRun{RunID: run, MonitorID: id}
	var b []byte
	err := r.db.QueryRowContext(ctx, `SELECT state,results,error,created_at,expires_at FROM channel_monitor_runs WHERE run_id=$1 AND monitor_id=$2 AND expires_at>now()`, run, id).Scan(&v.State, &b, &v.Error, &v.CreatedAt, &v.ExpiresAt)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(b, &v.Results)
	return v, err
}
func (r *channelMonitorRepository) InterruptMonitorRuns(ctx context.Context, before time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE channel_monitor_runs SET state='interrupted',error='Service restarted; inspect saved results or resume the original task' WHERE state='running' AND created_at<$1`, before)
	return err
}
