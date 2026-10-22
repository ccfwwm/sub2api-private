package handler

import (
	"context"
	"database/sql"
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

func TestMediaDescriptionsStayWithinGroupAndDisplayDTO(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	h := &ModelPlazaHandler{media: &MediaHandler{db: db}}
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(mediaDescriptionKey(19)).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"version":2,"models":{"same":"9图 / 720P"},"reference_prices":{"same":{"amount":"0.10","unit":"秒"}}}`))
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(mediaDescriptionKey(20)).WillReturnError(sql.ErrNoRows)
	groups := []modelPlazaGroup{{ID: 19, MediaModels: []modelPlazaMediaModel{{CatalogEntry: media.CatalogEntry{Model: "same"}}}}, {ID: 20, MediaModels: []modelPlazaMediaModel{{CatalogEntry: media.CatalogEntry{Model: "same"}}}}}
	require.NoError(t, h.attachDescriptions(context.Background(), groups))
	require.Equal(t, int64(2), groups[0].DescriptionVersion)
	require.Equal(t, "9图 / 720P", groups[0].MediaModels[0].DisplayDescription)
	require.Empty(t, groups[1].MediaModels[0].DisplayDescription)
	require.Equal(t, &mediaReferencePrice{Amount: "0.10", Unit: "秒"}, groups[0].MediaModels[0].OfficialReferencePrice)
	require.Nil(t, groups[1].MediaModels[0].OfficialReferencePrice)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDescriptionEditRejectsUnauthenticated(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	(&ModelPlazaHandler{}).SaveMediaDescription(c)
	require.Equal(t, 403, w.Code)
}

func TestDescriptionEditUsesVersionAndPreservesOtherModels(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			price := media.PriceCard{ID: "retail", Version: "1", Purpose: "sale", Currency: "USD", MediaType: "video", Operation: "video", Status: "enabled"}
			price.ModelMatch.Kind = "exact"
			price.ModelMatch.Value = "same"
			price.Scope.Kind = "group"
			price.Scope.Reference = "19"
			h := &ModelPlazaHandler{media: &MediaHandler{db: db, Engine: &media.Engine{Config: func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
				return []media.Capability{{Model: "same", MediaType: "video", Enabled: true, Status: "enabled", GroupIDs: []int64{19}, Operations: []string{"video"}}}, []media.PriceCard{price}, nil
			}}}}
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO settings").WithArgs(mediaDescriptionKey(19)).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("SELECT value FROM settings.*FOR UPDATE").WithArgs(mediaDescriptionKey(19)).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"version":2,"models":{"other":"preserved"},"reference_prices":{"same":{"amount":"0","unit":"次"}}}`))
			if conflict {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec("UPDATE settings").WithArgs(mediaDescriptionKey(19), `{"version":3,"models":{"other":"preserved","same":"new description"},"reference_prices":{"same":{"amount":"0","unit":"次"}}}`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			expected := 2
			if conflict {
				expected = 1
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
			c.Set(string(middleware.ContextKeyUserRole), "admin")
			c.Request = httptest.NewRequest("PUT", "/", strings.NewReader(fmt.Sprintf(`{"group_id":19,"model":"same","description":"new description","expected_version":%d}`, expected)))
			c.Request.Header.Set("Content-Type", "application/json")
			h.SaveMediaDescription(c)
			if conflict {
				require.Equal(t, 409, w.Code)
			} else {
				require.Equal(t, 200, w.Code)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
