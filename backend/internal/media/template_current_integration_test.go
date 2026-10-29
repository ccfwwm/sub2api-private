//go:build integration

package media_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/stretchr/testify/require"
)

func TestCurrentTemplateEditsWithoutBackupsAndDetachesOnDelete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	store := &media.Store{DB: db}
	snap := fixture(t, db, "20")
	template := media.BuiltinTemplates()[0]
	template.ID = media.NewTaskID()
	raw, err := json.Marshal(template)
	require.NoError(t, err)
	first, err := store.UpdateTemplate(ctx, template.ID, 0, nil, raw, snap.Identity.UserID, false)
	require.NoError(t, err)
	cap := snap.Capability
	cap.TemplateRef = &media.TemplateRef{ID: template.ID, Version: 1}
	models, err := json.Marshal([]media.Capability{cap})
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE accounts SET extra=jsonb_build_object('media_models',$1::jsonb) WHERE id=$2`, models, cap.AccountIDs[0])
	require.NoError(t, err)
	monitorCap := cap
	monitorCap.TemplateRef = &media.TemplateRef{ID: template.ID, Version: 0}
	monitorConfig, err := json.Marshal(map[string]any{"media_monitor": map[string]any{
		"mode":   "generation",
		"models": map[string]any{cap.Model: map[string]any{"capability": monitorCap, "operation": "video"}},
	}})
	require.NoError(t, err)
	var monitorID int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO channel_monitors(name,provider,endpoint,api_key_encrypted,primary_model,interval_seconds,created_by,body_override_mode,body_override) VALUES('media-template-test','imagevideo','https://example.invalid','encrypted-test-key',$1,60,$2,'replace',$3) RETURNING id`, cap.Model, snap.Identity.UserID, monitorConfig).Scan(&monitorID))
	// An accepted task keeps the original execution template after future edits.
	snap.Capability.Template = &template
	snap.Capability.TemplateRef = cap.TemplateRef
	task, err := store.CreateTask(ctx, snap, cap.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	var frozen string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT snapshot::text FROM media_tasks WHERE id=$1`, task.ID).Scan(&frozen))
	template.Name, template.Description = "修改后的协议标题", "图片、视频和音频协议介绍"
	raw, err = json.Marshal(template)
	require.NoError(t, err)
	updated, err := store.UpdateTemplate(ctx, template.ID, first.Version, first.Config, raw, snap.Identity.UserID, false)
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM media_config_versions WHERE kind='template' AND id=$1`, template.ID).Scan(&count))
	require.Equal(t, 1, count, "saving must not create backup records")
	_, err = store.UpdateTemplate(ctx, template.ID, first.Version, first.Config, raw, snap.Identity.UserID, false)
	require.ErrorContains(t, err, "template_conflict", "stale editors cannot overwrite a current template")
	var monitorTemplateVersion string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT entry.value#>>'{capability,template_ref,version}' FROM channel_monitors m CROSS JOIN LATERAL jsonb_each(m.body_override#>'{media_monitor,models}') entry WHERE m.id=$1 AND entry.key=$2`, monitorID, cap.Model).Scan(&monitorTemplateVersion))
	require.Equal(t, "1", monitorTemplateVersion, "monitor snapshots follow the current template")
	_, err = store.UpdateTemplate(ctx, template.ID, updated.Version, updated.Config, nil, snap.Identity.UserID, true)
	require.NoError(t, err)
	var enabled bool
	var ref string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT (extra#>>'{media_models,0,enabled}')::boolean,coalesce(extra#>>'{media_models,0,template_ref,id}','') FROM accounts WHERE id=$1`, cap.AccountIDs[0]).Scan(&enabled, &ref))
	require.False(t, enabled)
	require.Empty(t, ref)
	var monitorMode string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT body_override#>>'{media_monitor,mode}' FROM channel_monitors WHERE id=$1`, monitorID).Scan(&monitorMode))
	require.Equal(t, "connection", monitorMode, "deleted templates disable generation probes")
	var after string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT snapshot::text FROM media_tasks WHERE id=$1`, task.ID).Scan(&after))
	require.Equal(t, frozen, after)
	// A deleted built-in tombstone would also survive seeding on restart.
	require.NoError(t, store.EnsureBuiltinTemplates(ctx))
	rows, err := store.Configs(ctx, "template", true)
	require.NoError(t, err)
	for _, row := range rows {
		require.NotEqual(t, template.ID, row.ID)
	}
}
