//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration249IsReplayable(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	_, err := tx.ExecContext(ctx, `
CREATE TEMP TABLE media_config_versions (
	kind text NOT NULL,
	CONSTRAINT media_config_versions_kind_check CHECK(kind IN ('capability','price'))
);
CREATE TEMP TABLE media_artifacts (key text NOT NULL);
`)
	require.NoError(t, err)

	sql, err := dbmigrations.FS.ReadFile("249_media_protocol_templates.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = tx.ExecContext(ctx, string(sql))
		require.NoError(t, err, "migration 249 must be safe to replay")
	}

	var filenameColumn bool
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1 FROM information_schema.columns
	WHERE table_schema = (SELECT nspname FROM pg_namespace WHERE oid = pg_my_temp_schema())
	  AND table_name = 'media_artifacts'
	  AND column_name = 'filename'
)
`).Scan(&filenameColumn))
	require.True(t, filenameColumn)

	for _, kind := range []string{"capability", "price", "template", "binding"} {
		_, err = tx.ExecContext(ctx, "INSERT INTO media_config_versions(kind) VALUES ($1)", kind)
		require.NoErrorf(t, err, "kind %q should be allowed", kind)
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO media_config_versions(kind) VALUES ('unknown')")
	require.Error(t, err, "the widened constraint must still reject unknown kinds")
}
