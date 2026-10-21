package media

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestRuntimeTemplateIsolationKeepsOtherAccountsAndRoutingOwnership(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	store := &Store{DB: db}
	store.templateSeed.Do(func() {})
	cap := Capability{ID: "shared", Model: "public-video", MediaType: "video", Operations: []string{"video"}, GroupIDs: []int64{9}, AccountIDs: []int64{4, 5}, Enabled: true, Status: "enabled", TemplateRef: &TemplateRef{ID: "video-arrays", Version: 1}}
	broken := ModelBinding{ID: "broken", GroupID: 9, Model: cap.Model, AccountID: 4, Status: "enabled", Rules: []ParameterRule{{Phase: "wire", Op: "rename", Source: "request", Target: "request.nested"}}}
	brokenRaw, err := json.Marshal(broken)
	require.NoError(t, err)
	templateRaw, err := json.Marshal(templateFixture(t, "video-arrays"))
	require.NoError(t, err)
	price := testCard(baseCharge("second", "request_duration", "0"))
	price.ModelMatch.Value, price.Scope.Reference = cap.Model, "9"
	engine := &Engine{Store: store, Config: func(context.Context, int64) ([]Capability, []PriceCard, error) {
		return []Capability{cap}, []PriceCard{price}, nil
	}}
	expectConfig := func() {
		mock.ExpectQuery(`SELECT h.id,h.version,h.published_version,v.status,v.config FROM media_config_heads h JOIN media_config_versions v ON v.kind=h.kind AND v.id=h.id AND v.version=h.published_version WHERE h.kind=$1 ORDER BY h.id`).WithArgs("binding").WillReturnRows(sqlmock.NewRows([]string{"id", "version", "published_version", "status", "config"}).AddRow("broken", 1, 1, "enabled", brokenRaw))
		mock.ExpectQuery(`SELECT id,version,NULL::bigint,status,config FROM media_config_versions WHERE kind=$1 AND id=$2 AND version=$3`).WithArgs("template", "video-arrays", int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "version", "published_version", "status", "config"}).AddRow("video-arrays", 1, nil, "enabled", templateRaw))
	}
	expectConfig()
	catalog, err := engine.Catalog(context.Background(), 9)
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.Equal(t, cap.Model, catalog[0].Model)
	expectConfig()
	selected, err := engine.Capability(context.Background(), cap.Model, "video", Identity{GroupID: 9})
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, []int64{5}, selected.AccountIDs)
	expectConfig()
	managed, err := engine.ManagedModels(context.Background(), 9, "")
	require.NoError(t, err)
	require.True(t, managed[cap.Model])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestIsolatedTemplateResolutionPropagatesStorageFailure(t *testing.T) {
	cap := Capability{TemplateRef: &TemplateRef{ID: "template", Version: 1}}
	failure := errors.New("database unavailable")
	resolved, _, err := ResolveTemplateBindingsIsolated([]Capability{cap}, nil, func(TemplateRef) (ProtocolTemplate, error) { return ProtocolTemplate{}, failure })
	require.ErrorIs(t, err, failure)
	require.Nil(t, resolved)
}
