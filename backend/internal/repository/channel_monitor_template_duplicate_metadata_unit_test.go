//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestApplyChannelMonitorTemplatePreservesDuplicateOperationMetadataAtomically(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	const templateID int64 = 7
	monitorIDs := []int64{41, 42}

	mock.ExpectBegin()
	expectChannelMonitorTemplateForApply(mock, templateID)
	mock.ExpectExec(`(?s)UPDATE "channel_monitors" SET "custom_body" = NULL, "body_override" = NULL, "updated_at" = \$1, "api_mode" = \$2, "probe_config" = \$3, "probe_type" = \$4, "image_probe_schedule_enabled" = \$5, "custom_method" = \$6, "custom_path" = \$7, "custom_response_json_path" = \$8, "custom_response_require_non_empty" = \$9, "body_override_mode" = \$10 WHERE .*"template_id" = \$11.*"id" IN \(\$12, \$13\).*"provider" = \$14.*"api_mode" = \$15`).
		WithArgs(
			sqlmock.AnyArg(),
			service.MonitorAPIModeResponses,
			sqlmock.AnyArg(),
			service.MonitorProbeTypeText,
			false,
			"POST",
			"",
			"",
			true,
			service.MonitorBodyOverrideModeOff,
			templateID,
			monitorIDs[0],
			monitorIDs[1],
			service.MonitorProviderOpenAI,
			service.MonitorAPIModeResponses,
		).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`(?s)UPDATE channel_monitors\s+SET extra_headers = \$1::jsonb \|\| CASE\s+WHEN COALESCE\(extra_headers, '\{\}'::jsonb\) \? \(\$2::text\)\s+THEN jsonb_build_object\(\$2::text, COALESCE\(extra_headers, '\{\}'::jsonb\) -> \(\$2::text\)\)\s+ELSE '\{\}'::jsonb\s+END\s+WHERE template_id = \$3\s+AND id = ANY\(\$4\)\s+AND provider = \$5\s+AND api_mode = \$6`).
		WithArgs(
			`{"User-Agent":"template-client"}`,
			service.ChannelMonitorDuplicateOperationIDMetadataKey,
			templateID,
			`{41,42}`,
			service.MonitorProviderOpenAI,
			service.MonitorAPIModeResponses,
		).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	repo := NewChannelMonitorRequestTemplateRepository(client, db)
	affected, err := repo.ApplyToMonitors(context.Background(), templateID, monitorIDs)

	require.NoError(t, err)
	require.Equal(t, int64(2), affected)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyChannelMonitorTemplateRollsBackWhenHeaderRowCountDiffers(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	const templateID int64 = 7

	mock.ExpectBegin()
	expectChannelMonitorTemplateForApply(mock, templateID)
	mock.ExpectExec(`(?s)UPDATE "channel_monitors" SET .*WHERE `).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`(?s)UPDATE channel_monitors\s+SET extra_headers = \$1::jsonb \|\| CASE.*jsonb_build_object\(\$2::text,.*WHERE template_id = \$3.*AND id = ANY\(\$4\)`).
		WithArgs(
			`{"User-Agent":"template-client"}`,
			service.ChannelMonitorDuplicateOperationIDMetadataKey,
			templateID,
			`{41,42}`,
			service.MonitorProviderOpenAI,
			service.MonitorAPIModeResponses,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	repo := NewChannelMonitorRequestTemplateRepository(client, db)
	affected, err := repo.ApplyToMonitors(context.Background(), templateID, []int64{41, 42})

	require.Zero(t, affected)
	require.EqualError(t, err, "apply template headers: affected 1 rows, expected 2")
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectChannelMonitorTemplateForApply(mock sqlmock.Sqlmock, templateID int64) {
	now := time.Now()
	mock.ExpectQuery(`(?s)SELECT .* FROM "channel_monitor_request_templates" WHERE "channel_monitor_request_templates"\."id" = \$1 LIMIT 2`).
		WithArgs(templateID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "name", "provider", "api_mode", "description",
			"extra_headers", "body_override_mode", "body_override",
			"probe_type", "image_probe_schedule_enabled", "custom_method", "custom_path", "custom_body", "custom_response_json_path", "custom_response_require_non_empty",
		}).AddRow(
			templateID, now, now, "monitor-template", service.MonitorProviderOpenAI,
			service.MonitorAPIModeResponses, "", []byte(`{"User-Agent":"template-client"}`),
			service.MonitorBodyOverrideModeOff, nil,
			service.MonitorProbeTypeText, false, "POST", "", nil, "", true,
		))
}
