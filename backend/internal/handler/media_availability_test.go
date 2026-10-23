package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func mediaAvailabilityHandler(db *sql.DB) *ModelPlazaHandler {
	price := media.PriceCard{ID: "retail", Version: "1", Purpose: "sale", Currency: "USD", MediaType: "video", Operation: "video", Status: "enabled"}
	price.ModelMatch.Kind, price.ModelMatch.Value = "exact", "video-a"
	price.Scope.Kind, price.Scope.Reference = "group", "19"
	return &ModelPlazaHandler{media: &MediaHandler{db: db, Engine: &media.Engine{Config: func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
		return []media.Capability{{Model: "video-a", MediaType: "video", Enabled: true, Status: "enabled", GroupIDs: []int64{19}, Operations: []string{"video"}}}, []media.PriceCard{price}, nil
	}}}}
}

func mediaAvailabilityContext(body, role string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	if role != "" {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
		c.Set(string(middleware.ContextKeyUserRole), role)
	}
	c.Request = httptest.NewRequest("PUT", "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestAttachMediaAvailabilityDefaultsAndIsolatesGroups(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	h := &ModelPlazaHandler{media: &MediaHandler{db: db}}
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(mediaDescriptionKey(19)).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"version":4,"models":{"video-a":"description"},"reference_prices":{"video-a":{"amount":"0.1","unit":"sec"}},"availability":{"video-a":"unavailable"}}`))
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(mediaDescriptionKey(20)).WillReturnError(sql.ErrNoRows)
	groups := []modelPlazaGroup{
		{ID: 19, MediaModels: []modelPlazaMediaModel{{CatalogEntry: media.CatalogEntry{Model: "video-a"}}}},
		{ID: 20, MediaModels: []modelPlazaMediaModel{{CatalogEntry: media.CatalogEntry{Model: "video-a"}}}},
	}
	require.NoError(t, h.attachDescriptions(context.Background(), groups))
	require.Equal(t, "unavailable", groups[0].MediaModels[0].Availability)
	require.Equal(t, "available", groups[1].MediaModels[0].Availability)
	require.Equal(t, "description", groups[0].MediaModels[0].DisplayDescription)
	require.Equal(t, &mediaReferencePrice{Amount: "0.1", Unit: "sec"}, groups[0].MediaModels[0].OfficialReferencePrice)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveMediaAvailabilityValidatesAdminAndStatus(t *testing.T) {
	for _, role := range []string{"", "user"} {
		c, w := mediaAvailabilityContext(`{}`, role)
		(&ModelPlazaHandler{}).SaveMediaAvailability(c)
		require.Equal(t, 403, w.Code)
	}
	for _, body := range []string{
		`{}`,
		`{"group_id":0,"model":"video-a","availability":"available","expected_version":0}`,
		`{"group_id":19,"model":"video-a","availability":"unknown","expected_version":0}`,
		`{"group_id":19,"model":"video-a","availability":"available","expected_version":-1}`,
	} {
		c, w := mediaAvailabilityContext(body, "admin")
		(&ModelPlazaHandler{}).SaveMediaAvailability(c)
		require.Equal(t, 400, w.Code, body)
	}
}

func TestSaveMediaAvailabilityPersistsAndPreservesOtherMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	initial := `{"version":7,"models":{"video-a":"keep description","other":"other description"},"reference_prices":{"video-a":{"amount":"2","unit":"sec"}},"availability":{"other":"unavailable"}}`
	var metadata mediaDescriptions
	require.NoError(t, json.Unmarshal([]byte(initial), &metadata))
	metadata.Version++
	metadata.Availability["video-a"] = "unavailable"
	encoded, err := json.Marshal(metadata)
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO settings").WithArgs(mediaDescriptionKey(19)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT value FROM settings.*FOR UPDATE").WithArgs(mediaDescriptionKey(19)).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(initial))
	mock.ExpectExec("UPDATE settings").WithArgs(mediaDescriptionKey(19), string(encoded)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	c, w := mediaAvailabilityContext(`{"group_id":19,"model":"video-a","availability":"unavailable","expected_version":7}`, "admin")
	mediaAvailabilityHandler(db).SaveMediaAvailability(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"availability":"unavailable"`)
	require.Contains(t, w.Body.String(), `"description_version":8`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveMediaAvailabilityRejectsWrongGroupAndVersionConflict(t *testing.T) {
	for _, action := range []string{"wrong-group", "conflict"} {
		t.Run(action, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			groupID, expectedVersion, wantStatus := 19, 2, 409
			if action == "wrong-group" {
				groupID, wantStatus = 20, 404
			} else {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO settings").WithArgs(mediaDescriptionKey(19)).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery("SELECT value FROM settings.*FOR UPDATE").WithArgs(mediaDescriptionKey(19)).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"version":3,"models":{}}`))
				mock.ExpectRollback()
			}
			c, w := mediaAvailabilityContext(fmt.Sprintf(`{"group_id":%d,"model":"video-a","availability":"available","expected_version":%d}`, groupID, expectedVersion), "admin")
			mediaAvailabilityHandler(db).SaveMediaAvailability(c)
			require.Equal(t, wantStatus, w.Code, w.Body.String())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
