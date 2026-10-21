//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration250PinsOnlyExplicitLegacyMediaProtocols(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	_, err := tx.ExecContext(ctx, `
CREATE TEMP TABLE accounts (
	id bigint PRIMARY KEY,
	platform text NOT NULL,
	extra jsonb NOT NULL DEFAULT '{}'
);
INSERT INTO accounts(id, platform, extra) VALUES
	(1, 'imagevideo', '{"media_models":[{"model":"unrelated-name","media_type":"video","operations":["video"],"request_format":"v6k_url","adapter":"unknown","enabled":true}]}'),
	(2, 'imagevideo', '{"media_models":[{"model":"grok-imagine-video-1.5","media_type":"video","operations":["video"],"request_format":"legacy","adapter":"yunfei_grok","enabled":true}]}'),
	(3, 'imagevideo', '{"media_models":[{"model":"known-adapter-but-unknown-format","request_format":"vendor_new","adapter":"yunfei_grok","enabled":true}]}'),
	(4, 'imagevideo', '{"media_models":[{"model":"grok-imagine-video-1.5","provider":"yunfei","base_url":"https://example.invalid","enabled":true}]}'),
	(5, 'imagevideo', '{"media_models":[{"model":"arbitrary","adapter":"unknown","template_ref":{"id":"custom-v4","version":4},"enabled":true}]}'),
	(6, 'imagevideo', '{"media_models":[{"model":"image-model","media_type":"image","adapter":"yunfei_gpt_images","operations":["image_edit"],"enabled":true}]}'),
	(8, 'imagevideo', '{"media_models":[{"model":"misconfigured-format","media_type":"image","operations":["image"],"request_format":"v6k_url","adapter":"unknown","enabled":true}]}'),
	(9, 'imagevideo', '{"media_models":[{"model":"misconfigured-adapter","media_type":"image","operations":["image"],"request_format":"legacy","adapter":"yunfei_grok","enabled":true}]}'),
	(10, 'imagevideo', '{"media_models":[{"model":"empty-template-ref","media_type":"video","operations":["video"],"request_format":"legacy","adapter":"yunfei_grok","template_ref":null,"enabled":true}]}'),
	(11, 'imagevideo', '{"media_models":[{"model":"invalid-template-ref","media_type":"video","operations":["video"],"request_format":"legacy","adapter":"yunfei_grok","template_ref":{"id":"custom","version":0},"enabled":true}]}'),
	(7, 'openai', '{"media_models":[{"model":"grok-imagine-video-1.5","enabled":true}]}');
`)
	require.NoError(t, err)

	sql, err := dbmigrations.FS.ReadFile("250_migrate_legacy_media_template_refs.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = tx.ExecContext(ctx, string(sql))
		require.NoError(t, err, "migration must be replayable and preserve already-pinned references")
	}

	type migratedCapability struct {
		Model            string         `json:"model"`
		TemplateRef      map[string]any `json:"template_ref"`
		Enabled          bool           `json:"enabled"`
		ValidationStatus string         `json:"validation_status"`
	}
	read := func(id int) migratedCapability {
		var raw []byte
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT extra->'media_models'->0 FROM accounts WHERE id=$1`, id).Scan(&raw))
		var cap migratedCapability
		require.NoError(t, json.Unmarshal(raw, &cap))
		return cap
	}
	assertPinned := func(id int, templateID string, version float64) {
		t.Helper()
		cap := read(id)
		require.True(t, cap.Enabled)
		require.Equal(t, templateID, cap.TemplateRef["id"])
		require.Equal(t, version, cap.TemplateRef["version"])
	}
	assertPinned(1, "video-arrays", 1)
	assertPinned(2, "video-single-image", 1)
	assertPinned(5, "custom-v4", 4)
	assertPinned(6, "image-json-multipart", 1)

	for _, id := range []int{3, 4, 8, 9, 10, 11} {
		cap := read(id)
		require.False(t, cap.Enabled, "ambiguous model %d must remain disabled", id)
		require.Equal(t, "待配置协议模板", cap.ValidationStatus)
	}
	require.Nil(t, read(10).TemplateRef)
	require.Equal(t, "custom", read(11).TemplateRef["id"], "invalid explicit references are retained rather than guessed over")
	require.Equal(t, float64(0), read(11).TemplateRef["version"])
	var nonMedia json.RawMessage
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT extra->'media_models'->0 FROM accounts WHERE id=7`).Scan(&nonMedia))
	var cap map[string]any
	require.NoError(t, json.Unmarshal(nonMedia, &cap))
	require.NotContains(t, cap, "template_ref", "non-media account data must not be migrated")
}
