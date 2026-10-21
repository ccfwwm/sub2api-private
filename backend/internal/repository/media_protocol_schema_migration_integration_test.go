//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration249PreservesExistingMediaDataAndSupportsReplay(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	_, err := tx.ExecContext(ctx, `
CREATE TEMP TABLE media_config_versions (
	id text NOT NULL,
	kind text NOT NULL,
	config jsonb NOT NULL,
	CONSTRAINT media_config_versions_kind_check CHECK(kind IN ('capability','price'))
);
CREATE TEMP TABLE media_artifacts (
	id text NOT NULL,
	sha256 text NOT NULL
);
INSERT INTO media_config_versions(id,kind,config) VALUES
	('old-price','price','{"charges":[{"price":"1.25"}]}'),
	('old-capability','capability','{"model":"public","enabled":true}');
INSERT INTO media_artifacts(id,sha256) VALUES ('old-artifact','fixture-sha256');
`)
	require.NoError(t, err)
	sql, err := dbmigrations.FS.ReadFile("249_media_protocol_templates.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = tx.ExecContext(ctx, string(sql))
		require.NoError(t, err)
	}
	var value, filename, checksum string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT config #>> '{charges,0,price}' FROM media_config_versions WHERE id='old-price'`).Scan(&value))
	require.Equal(t, "1.25", value)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT config->>'model' FROM media_config_versions WHERE id='old-capability'`).Scan(&value))
	require.Equal(t, "public", value)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT filename,sha256 FROM media_artifacts WHERE id='old-artifact'`).Scan(&filename, &checksum))
	require.Empty(t, filename)
	require.Equal(t, "fixture-sha256", checksum)
	_, err = tx.ExecContext(ctx, `INSERT INTO media_config_versions(id,kind,config) VALUES ('new-template','template','{}'),('new-binding','binding','{}')`)
	require.NoError(t, err)
}
