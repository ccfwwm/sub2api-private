package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
)

// UpdateTemplate edits a single template in place. Tasks already contain their
// resolved protocol, so editing a template never changes an accepted task.
func (s *Store) UpdateTemplate(ctx context.Context, id string, expected int64, previous, data json.RawMessage, actor int64, remove bool) (ConfigRecord, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return ConfigRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "media-config"); err != nil {
		return ConfigRecord{}, err
	}
	var current int64
	var old json.RawMessage
	err = tx.QueryRowContext(ctx, `SELECT h.version,v.config FROM media_config_heads h JOIN media_config_versions v ON v.kind=h.kind AND v.id=h.id AND v.version=h.version WHERE h.kind='template' AND h.id=$1 FOR UPDATE OF h`, id).Scan(&current, &old)
	if err != nil && err != sql.ErrNoRows {
		return ConfigRecord{}, err
	}
	if current != expected || (current > 0 && len(previous) == 0) {
		return ConfigRecord{}, fail(409, "template_conflict", "模板已被修改，请刷新后重试")
	}
	if current > 0 {
		var same bool
		if err = tx.QueryRowContext(ctx, `SELECT $1::jsonb=$2::jsonb`, []byte(old), []byte(previous)).Scan(&same); err != nil {
			return ConfigRecord{}, err
		}
		if !same {
			return ConfigRecord{}, fail(409, "template_conflict", "模板已被修改，请刷新后重试")
		}
	}
	// Include account capabilities and group bindings, including disabled models:
	// a later model enable must not silently restore an obsolete protocol.
	var references int
	if err = tx.QueryRowContext(ctx, `SELECT
	 (SELECT count(*) FROM accounts a CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(a.extra->'media_models')='array' THEN a.extra->'media_models' ELSE '[]'::jsonb END) c WHERE a.deleted_at IS NULL AND c#>>'{template_ref,id}'=$1) +
	 (SELECT count(*) FROM media_config_heads h JOIN media_config_versions v ON v.kind=h.kind AND v.id=h.id AND v.version=h.version WHERE h.kind IN ('binding','capability') AND v.status<>'disabled' AND coalesce(v.config#>>'{template,id}',v.config#>>'{template_ref,id}')=$1)`, id).Scan(&references); err != nil {
		return ConfigRecord{}, err
	}
	if remove {
		if current == 0 {
			return ConfigRecord{}, fail(404, "template_missing", "模板不存在")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE accounts a SET extra=jsonb_set(a.extra,'{media_models}',(SELECT jsonb_agg(CASE WHEN c#>>'{template_ref,id}'=$1 THEN (c-'template_ref')||'{"enabled":false,"validation_status":"待配置协议模板"}'::jsonb ELSE c END ORDER BY n) FROM jsonb_array_elements(a.extra->'media_models') WITH ORDINALITY AS x(c,n))) WHERE a.deleted_at IS NULL AND jsonb_typeof(a.extra->'media_models')='array' AND a.extra->'media_models' @> jsonb_build_array(jsonb_build_object('template_ref',jsonb_build_object('id',$1::text)))`, id); err != nil {
			return ConfigRecord{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE media_config_versions SET status='disabled',config=config||'{"status":"disabled"}'::jsonb||CASE WHEN kind='capability' THEN '{"enabled":false}'::jsonb ELSE '{}'::jsonb END WHERE kind IN ('binding','capability') AND coalesce(config#>>'{template,id}',config#>>'{template_ref,id}')=$1`, id); err != nil {
			return ConfigRecord{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE media_config_heads h SET published_version=NULL WHERE h.kind IN ('binding','capability') AND EXISTS(SELECT 1 FROM media_config_versions v WHERE v.kind=h.kind AND v.id=h.id AND v.version=h.version AND v.status='disabled' AND coalesce(v.config#>>'{template,id}',v.config#>>'{template_ref,id}')=$1)`, id); err != nil {
			return ConfigRecord{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE media_config_versions SET status='disabled',config=jsonb_set(config,'{status}','"disabled"') WHERE kind='template' AND id=$1 AND version=$2`, id, current); err != nil {
			return ConfigRecord{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE media_config_heads SET published_version=NULL WHERE kind='template' AND id=$1`, id); err != nil {
			return ConfigRecord{}, err
		}
		return ConfigRecord{}, tx.Commit()
	}
	var template ProtocolTemplate
	if err = decodeConfig(data, &template); err != nil {
		return ConfigRecord{}, err
	}
	if template.ID != id {
		return ConfigRecord{}, fail(400, "config_identity_conflict", "模板 ID 不一致")
	}
	template.Status = "enabled"
	if err = ValidateTemplate(template, true); err != nil {
		return ConfigRecord{}, err
	}
	if references > 0 {
		var before ProtocolTemplate
		if err = json.Unmarshal(old, &before); err != nil {
			return ConfigRecord{}, err
		}
		for _, op := range before.Operations {
			if !contains(template.Operations, op) {
				return ConfigRecord{}, fail(409, "template_in_use", "已绑定模板不能移除原有操作，请复制模板后修改")
			}
		}
		for _, kind := range before.MediaTypes {
			if !contains(template.MediaTypes, kind) {
				return ConfigRecord{}, fail(409, "template_in_use", "已绑定模板不能移除原有媒体类型，请复制模板后修改")
			}
		}
	}
	if current == 0 {
		current = 1
	}
	var object map[string]any
	if err = json.Unmarshal(data, &object); err != nil {
		return ConfigRecord{}, err
	}
	object["version"], object["status"] = strconv.FormatInt(current, 10), "enabled"
	data, err = json.Marshal(object)
	if err != nil {
		return ConfigRecord{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO media_config_versions(kind,id,version,status,config,actor_id) VALUES('template',$1,$2,'enabled',$3,$4) ON CONFLICT(kind,id,version) DO UPDATE SET status='enabled',config=excluded.config,actor_id=excluded.actor_id`, id, current, []byte(data), actor); err != nil {
		return ConfigRecord{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO media_config_heads(kind,id,version,published_version) VALUES('template',$1,$2,$2) ON CONFLICT(kind,id) DO UPDATE SET published_version=excluded.published_version`, id, current); err != nil {
		return ConfigRecord{}, err
	}
	// Move legacy account/group references to the one current template; no backup
	// records are created. Existing task snapshots remain self-contained.
	if _, err = tx.ExecContext(ctx, `UPDATE accounts a SET extra=jsonb_set(a.extra,'{media_models}',(SELECT jsonb_agg(CASE WHEN c#>>'{template_ref,id}'=$1 THEN jsonb_set(c,'{template_ref,version}',to_jsonb($2::bigint)) ELSE c END ORDER BY n) FROM jsonb_array_elements(a.extra->'media_models') WITH ORDINALITY AS x(c,n))) WHERE a.deleted_at IS NULL AND jsonb_typeof(a.extra->'media_models')='array' AND a.extra->'media_models' @> jsonb_build_array(jsonb_build_object('template_ref',jsonb_build_object('id',$1::text)))`, id, current); err != nil {
		return ConfigRecord{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE media_config_versions SET config=CASE WHEN kind='binding' THEN jsonb_set(config,'{template,version}',to_jsonb($2::bigint)) ELSE jsonb_set(config,'{template_ref,version}',to_jsonb($2::bigint)) END WHERE kind IN ('binding','capability') AND coalesce(config#>>'{template,id}',config#>>'{template_ref,id}')=$1`, id, current); err != nil {
		return ConfigRecord{}, err
	}
	if err = tx.Commit(); err != nil {
		return ConfigRecord{}, err
	}
	return ConfigRecord{ID: id, Version: current, PublishedVersion: &current, Status: "enabled", Config: data}, nil
}
