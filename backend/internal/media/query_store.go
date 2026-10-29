package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

func (s *Store) TaskList(ctx context.Context, userID int64, limit int, cursor ...string) ([]StoredTask, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	before := ""
	if len(cursor) > 0 {
		before = cursor[0]
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT `+taskSelect+` FROM media_tasks WHERE ($1=0 OR user_id=$1) AND ($3='' OR (created_at,id)<(SELECT created_at,id FROM media_tasks WHERE id=$3 AND ($1=0 OR user_id=$1))) ORDER BY created_at DESC,id DESC LIMIT $2`, userID, limit, before)
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	list := []StoredTask{}
	for rows.Next() {
		task, e := scanTask(rows)
		if e != nil {
			return nil, e
		}
		list = append(list, task)
	}
	return list, rows.Err()
}
func (s *Store) Artifacts(ctx context.Context, id string) ([]Artifact, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT key,media_type,mime_type,size_bytes,duration_seconds::text,available,filename FROM media_artifacts WHERE task_id=$1 ORDER BY key`, id)
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	list := []Artifact{}
	for rows.Next() {
		var a Artifact
		if e = rows.Scan(&a.Key, &a.Type, &a.MIME, &a.Size, &a.Duration, &a.Available, &a.Filename); e != nil {
			return nil, e
		}
		a.ContentURL = fmt.Sprintf("/v1/tasks/%s/artifacts/%s/content", id, a.Key)
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *Store) ArtifactFilename(ctx context.Context, id, key string) string {
	var name string
	_ = s.DB.QueryRowContext(ctx, `SELECT filename FROM media_artifacts WHERE task_id=$1 AND ($2='' OR key=$2) ORDER BY key LIMIT 1`, id, key).Scan(&name)
	return safeArtifactFilename(name)
}
func (s *Store) ArtifactFile(ctx context.Context, id, key string) (*os.File, string, string, error) {
	var path, mime, hash string
	var available bool
	query := `SELECT file_path,mime_type,content_hash,available FROM media_artifacts WHERE task_id=$1`
	args := []any{id}
	if key != "" {
		query += ` AND key=$2`
		args = append(args, key)
	}
	query += ` ORDER BY key LIMIT 1`
	if e := s.DB.QueryRowContext(ctx, query, args...).Scan(&path, &mime, &hash, &available); e != nil {
		return nil, "", "", fail(404, "artifact_not_found", "Artifact does not exist")
	}
	if !available {
		return nil, "", "", fail(410, "artifact_gone", "Artifact is no longer available")
	}
	file, e := os.Open(path)
	if e != nil {
		return nil, "", "", fail(410, "artifact_gone", "Artifact is no longer available")
	}
	return file, mime, hash, nil
}
func (s *Store) PublicTask(ctx context.Context, t StoredTask) (Task, error) {
	artifacts, e := s.Artifacts(ctx, t.ID)
	if e != nil {
		return Task{}, e
	}
	public := publicTaskValue(t, artifacts)
	var raw []byte
	e = s.DB.QueryRowContext(ctx, `SELECT details FROM media_ledger_entries WHERE operation_key=$1`, t.ID+":finalize").Scan(&raw)
	if e != nil && e != sql.ErrNoRows {
		return Task{}, e
	}
	if len(raw) > 0 {
		var calc Calculation
		if json.Unmarshal(raw, &calc) == nil {
			public.Billing["calculation"] = calc
		}
	}
	return public, nil
}

func publicTaskValue(t StoredTask, artifacts []Artifact) Task {
	status := t.State
	if status == "submitting" {
		status = "queued"
	}
	progress := 0
	if values := t.Quantities["upstream_progress"]; len(values) > 0 {
		if n, err := strconv.Atoi(values[0]); err == nil && n >= 0 && n <= 100 {
			progress = n
		}
	}
	if status == "completed" {
		progress = 100
	}
	public := Task{ID: t.ID, Object: Object(t.Snapshot.Capability.MediaType), Model: t.Snapshot.Capability.Model, Status: status, CreatedAt: t.Created.Unix(), Progress: progress, Artifacts: artifacts, Diagnostic: publicDiagnostic(t), Billing: map[string]any{"currency": "USD", "reserved": t.Reserved, "accepted": t.Accepted, "settled": t.Captured, "released": t.Released, "refunded": t.Refunded, "maximum_charge": t.Snapshot.Maximum, "price_version": t.Snapshot.Sale.Version, "multiplier": t.Identity.Multiplier, "time_multiplier": t.Identity.TimeMultiplier, "state": t.Settlement, "quantities": t.Quantities, "version": t.Version}}
	switch {
	case t.UpstreamID != "":
		public.SubmissionPhase = "upstream_accepted"
	case t.State == "queued":
		public.SubmissionPhase = "local_pending"
	case t.State == "submitting":
		public.SubmissionPhase = "submitting"
		if t.LeaseUntil == nil || t.LeaseUntil.Before(time.Now()) {
			public.SubmissionPhase = "reconciling"
		}
	case t.State == "unknown":
		public.SubmissionPhase = "reconciling"
	}
	if t.Snapshot.Inputs != nil {
		copy := *t.Snapshot.Inputs
		copy.Stage = "unrecorded"
		public.Inputs = &copy
	}
	if t.Completed != nil {
		unix := t.Completed.Unix()
		public.CompletedAt = &unix
	}
	return public
}

func (s *Store) History(ctx context.Context, kind, id string) ([]ConfigRecord, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,version,NULL::bigint,status,config FROM media_config_versions WHERE kind=$1 AND id=$2 ORDER BY version DESC`, kind, id)
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	items := []ConfigRecord{}
	for rows.Next() {
		var r ConfigRecord
		if e = rows.Scan(&r.ID, &r.Version, &r.PublishedVersion, &r.Status, &r.Config); e != nil {
			return nil, e
		}
		items = append(items, r)
	}
	return items, rows.Err()
}
func (s *Store) DisableConfig(ctx context.Context, kind, id string, expected, actor int64) (ConfigRecord, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return ConfigRecord{}, e
	}
	defer func() { _ = tx.Rollback() }()
	var current int64
	var raw []byte
	e = tx.QueryRowContext(ctx, `SELECT h.version,v.config FROM media_config_heads h JOIN media_config_versions v ON v.id=h.id AND v.kind=h.kind AND v.version=h.version WHERE h.id=$1 AND h.kind=$2 FOR UPDATE OF h`, id, kind).Scan(&current, &raw)
	if e != nil {
		return ConfigRecord{}, e
	}
	if current != expected {
		return ConfigRecord{}, fail(409, "version_conflict", "Refresh configuration")
	}
	var object map[string]any
	_ = json.Unmarshal(raw, &object)
	object["status"] = "disabled"
	if kind == "capability" {
		object["enabled"] = false
	}
	object["version"] = fmt.Sprint(current + 1)
	raw, e = json.Marshal(object)
	if e != nil {
		return ConfigRecord{}, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO media_config_versions(kind,id,version,status,config,actor_id) VALUES($1,$2,$3,'disabled',$4,$5)`, kind, id, current+1, raw, actor)
	if e != nil {
		return ConfigRecord{}, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE media_config_heads SET version=$1,published_version=$1 WHERE kind=$2 AND id=$3`, current+1, kind, id)
	if e != nil {
		return ConfigRecord{}, e
	}
	if e = tx.Commit(); e != nil {
		return ConfigRecord{}, e
	}
	version := current + 1
	return ConfigRecord{ID: id, Version: version, PublishedVersion: &version, Status: "disabled", Config: raw}, nil
}
func (s *Store) Reconcile(ctx context.Context, id, upstreamID, decision, evidence string, version, actor int64) error {
	if evidence == "" || len(evidence) > 2000 {
		return fail(400, "evidence_required", "Reconciliation requires a recorded reason and evidence")
	}
	if decision != "query" && decision != "failed" {
		return fail(400, "invalid_decision", "Confirm failure or bind the existing provider task for query")
	}
	if decision == "query" && (upstreamID == "" || len(upstreamID) > 200) {
		return fail(400, "upstream_id_required", "Existing upstream task ID required")
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
	if t.Version != version || t.Settlement != "manual_review" {
		return fail(409, "reconcile_conflict", "Task must be in manual review at the submitted version")
	}
	state := "in_progress"
	if decision == "failed" {
		state = "failed"
	}
	r, e := tx.ExecContext(ctx, `UPDATE media_tasks SET state=$1,settlement_state='held',upstream_id=coalesce(nullif($2,''),upstream_id),next_poll_at=now(),lease_until=NULL,lease_epoch=lease_epoch+1,version=version+1 WHERE id=$3`, state, upstreamID, id)
	if e = requireOne(r, e); e != nil {
		return e
	}
	details, _ := json.Marshal(map[string]any{"decision": decision, "evidence": evidence, "at": time.Now().UTC()})
	_, e = tx.ExecContext(ctx, `INSERT INTO media_ledger_entries(operation_key,task_id,kind,amount,details,actor_id) VALUES($1,$2,'reconcile',0,$3,$4)`, fmt.Sprintf("%s:reconcile:%d", id, version), id, details, actor)
	if e != nil {
		return e
	}
	return tx.Commit()
}
