package media

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestBindingPublicationValidatesSelectedCapabilitiesBeforeWriting(t *testing.T) {
	validCapability := Capability{
		ID: "capability", Model: "public-video", MediaType: "video", Operations: []string{"video"},
		GroupIDs: []int64{7}, AccountIDs: []int64{9}, Status: "enabled", Enabled: true,
	}
	for _, tc := range []struct {
		name      string
		runtime   bool
		modify    func(*Capability)
		missing   bool
		lookupErr error
		conflict  bool
		wantError string
	}{
		{name: "published profile"},
		{name: "account mode", runtime: true},
		{name: "missing model", missing: true, wantError: "binding_capability_missing"},
		{name: "disabled model", runtime: true, modify: func(c *Capability) { c.Enabled = false }, wantError: "binding_capability_missing"},
		{name: "unpublished capability", modify: func(c *Capability) { c.Status = "draft" }, wantError: "binding_capability_missing"},
		{name: "different group", modify: func(c *Capability) { c.GroupIDs = []int64{8} }, wantError: "binding_capability_missing"},
		{name: "different public model", runtime: true, modify: func(c *Capability) { c.Model = "other-video" }, wantError: "binding_capability_missing"},
		{name: "different account", runtime: true, modify: func(c *Capability) { c.AccountIDs = []int64{10} }, wantError: "binding_capability_missing"},
		{name: "unsupported media", runtime: true, modify: func(c *Capability) { c.MediaType = "audio" }, wantError: "binding_template_media_conflict"},
		{name: "unsupported operation", modify: func(c *Capability) { c.Operations = []string{"speech"} }, wantError: "binding_template_operation_conflict"},
		{name: "runtime lookup failure", runtime: true, lookupErr: errors.New("runtime capability lookup failed"), wantError: "runtime capability lookup failed"},
		{name: "selector conflict", conflict: true, wantError: "binding_conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			store := &Store{DB: db}
			binding := ModelBinding{ID: "binding", GroupID: 7, Model: validCapability.Model, AccountID: 9, Template: &TemplateRef{ID: "template", Version: 2}}
			template := ProtocolTemplate{ID: "template", Version: "2", Status: "enabled", MediaTypes: []string{"video"}, Operations: []string{"video"}}
			capability := validCapability
			if tc.modify != nil {
				tc.modify(&capability)
			}
			caps := []Capability{capability}
			if tc.missing {
				caps = nil
			}
			if tc.runtime {
				store.RuntimeCapabilities = func(_ context.Context, groupID int64) ([]Capability, error) {
					require.EqualValues(t, binding.GroupID, groupID)
					return caps, tc.lookupErr
				}
			}
			mock.ExpectBegin()
			mock.ExpectExec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`).WithArgs("media-config").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery(`SELECT version,published_version FROM media_config_heads WHERE kind=$1 AND id=$2 FOR UPDATE`).WithArgs("binding", binding.ID).
				WillReturnRows(sqlmock.NewRows([]string{"version", "published_version"}))
			templateRaw, err := json.Marshal(template)
			require.NoError(t, err)
			mock.ExpectQuery(`SELECT status,config FROM media_config_versions WHERE kind='template' AND id=$1 AND version=$2`).WithArgs(template.ID, binding.Template.Version).
				WillReturnRows(sqlmock.NewRows([]string{"status", "config"}).AddRow("enabled", templateRaw))
			publishedQuery := `SELECT h.id,h.version,h.published_version,v.status,v.config FROM media_config_heads h JOIN media_config_versions v ON v.kind=h.kind AND v.id=h.id AND v.version=h.published_version WHERE h.kind=$1 ORDER BY h.id`
			rows := sqlmock.NewRows([]string{"id", "version", "published_version", "status", "config"})
			if !tc.runtime {
				for _, c := range caps {
					raw, err := json.Marshal(c)
					require.NoError(t, err)
					rows.AddRow(c.ID, 1, 1, c.Status, raw)
				}
			}
			mock.ExpectQuery(publishedQuery).WithArgs("capability").WillReturnRows(rows)
			if tc.wantError == "" || tc.conflict {
				bindings := sqlmock.NewRows([]string{"id", "version", "published_version", "status", "config"})
				if tc.conflict {
					other := binding
					other.ID, other.Status = "other-binding", "enabled"
					raw, err := json.Marshal(other)
					require.NoError(t, err)
					bindings.AddRow(other.ID, 1, 1, other.Status, raw)
				}
				mock.ExpectQuery(publishedQuery).WithArgs("binding").WillReturnRows(bindings)
			}
			if tc.wantError == "" {
				mock.ExpectExec(`INSERT INTO media_config_versions(kind,id,version,status,config,actor_id) VALUES($1,$2,$3,$4,$5,$6)`).
					WithArgs("binding", binding.ID, int64(1), "enabled", sqlmock.AnyArg(), int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`INSERT INTO media_config_heads(kind,id,version,published_version) VALUES($1,$2,$3,$4) ON CONFLICT(kind,id) DO UPDATE SET version=excluded.version,published_version=excluded.published_version`).
					WithArgs("binding", binding.ID, int64(1), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			raw, err := json.Marshal(binding)
			require.NoError(t, err)
			record, err := store.SaveProtocolConfig(context.Background(), "binding", binding.ID, 0, 42, raw, true, nil)
			if tc.wantError == "" {
				require.NoError(t, err)
				require.EqualValues(t, 1, *record.PublishedVersion)
			} else {
				require.ErrorContains(t, err, tc.wantError)
				require.Empty(t, record)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestBindingPublicationValidatesEffectiveInheritedConfiguration(t *testing.T) {
	ref := &TemplateRef{ID: "inherited", Version: 2}
	conflicting := ParameterRule{Phase: "wire", Op: "fixed", Target: "request", Value: map[string]any{}}
	nested := ParameterRule{Phase: "wire", Op: "fixed", Target: "request.flag", Value: true}
	for _, tc := range []struct {
		name         string
		capRef       bool
		bindingRef   bool
		groupRef     bool
		accountRef   bool
		reservedRule bool
		wantError    string
	}{
		{name: "capability template parent conflict", capRef: true, wantError: "parameter_rule_conflict"},
		{name: "group template parent conflict", groupRef: true, wantError: "parameter_rule_conflict"},
		{name: "explicit template parent conflict", bindingRef: true, wantError: "parameter_rule_conflict"},
		{name: "higher account template takes priority", capRef: true, accountRef: true},
		{name: "inherited account rule cannot replace billing model", capRef: true, reservedRule: true, wantError: "model_mapping_reserved"},
		{name: "missing inherited template", wantError: "binding_template_missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			store := &Store{DB: db}
			capability := Capability{ID: "capability", Model: "public-video", MediaType: "video", Operations: []string{"video"}, GroupIDs: []int64{7, 8}, AccountIDs: []int64{9}, Status: "enabled", Enabled: true}
			binding := ModelBinding{ID: "new-binding", GroupID: 7, Model: capability.Model, Status: "enabled", Version: "1", Rules: []ParameterRule{nested}}
			if tc.capRef {
				capability.TemplateRef = ref
			}
			if tc.bindingRef {
				binding.Template = ref
			}
			if tc.reservedRule {
				capability.TemplateRules = []ParameterRule{{Phase: "semantic", Op: "fixed", Target: "model", Value: "free-model"}}
			}
			inherited := ProtocolTemplate{ID: ref.ID, Version: "2", Status: "enabled", MediaTypes: []string{"video"}, Operations: []string{"video"}, Rules: []ParameterRule{conflicting}}
			expectTemplate := func(template ProtocolTemplate, version int64) {
				raw, marshalErr := json.Marshal(template)
				require.NoError(t, marshalErr)
				mock.ExpectQuery(`SELECT status,config FROM media_config_versions WHERE kind='template' AND id=$1 AND version=$2`).WithArgs(template.ID, version).
					WillReturnRows(sqlmock.NewRows([]string{"status", "config"}).AddRow("enabled", raw))
			}
			mock.ExpectBegin()
			if tc.bindingRef {
				expectTemplate(inherited, ref.Version)
			}
			publishedQuery := `SELECT h.id,h.version,h.published_version,v.status,v.config FROM media_config_heads h JOIN media_config_versions v ON v.kind=h.kind AND v.id=h.id AND v.version=h.published_version WHERE h.kind=$1 ORDER BY h.id`
			capRaw, err := json.Marshal(capability)
			require.NoError(t, err)
			mock.ExpectQuery(publishedQuery).WithArgs("capability").WillReturnRows(sqlmock.NewRows([]string{"id", "version", "published_version", "status", "config"}).AddRow(capability.ID, 1, 1, "enabled", capRaw))
			rows := sqlmock.NewRows([]string{"id", "version", "published_version", "status", "config"})
			if tc.groupRef || tc.accountRef {
				other := ModelBinding{ID: "existing", GroupID: 7, Status: "enabled", Version: "1", Template: ref}
				if tc.accountRef {
					other.Model, other.AccountID = capability.Model, 9
					other.Template = &TemplateRef{ID: "account-template", Version: 3}
				}
				raw, err := json.Marshal(other)
				require.NoError(t, err)
				rows.AddRow(other.ID, 1, 1, "enabled", raw)
			}
			mock.ExpectQuery(publishedQuery).WithArgs("binding").WillReturnRows(rows)
			if tc.accountRef {
				accountTemplate := inherited
				accountTemplate.ID, accountTemplate.Version, accountTemplate.Rules = "account-template", "3", nil
				expectTemplate(accountTemplate, 3)
			} else if !tc.bindingRef && (tc.capRef || tc.groupRef) {
				expectTemplate(inherited, ref.Version)
			}
			mock.ExpectRollback()
			tx, err := db.Begin()
			require.NoError(t, err)
			err = store.validateBindingPublication(context.Background(), tx, binding)
			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantError)
			}
			require.NoError(t, tx.Rollback())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
