package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMediaTaskListParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		query                 string
		admin, paged, invalid bool
	}{
		{"", false, false, false}, {"before=t1", false, false, false},
		{"page_size=20", false, true, false}, {"page=3&page_size=20", true, true, false},
		{"page=1&before=", false, true, true}, {"page=", false, true, true}, {"page_size=0", false, true, true}, {"page=-1", true, true, true},
		{"page_size=1001", true, true, true}, {"page=1&user_id=99", false, true, true}, {"account_id=2", false, false, true},
		{"page=1&status=queued,in_progress", true, true, true}, {"media_type=image,video", true, false, true},
		{"page=1&sort_order=desc%3BDROP", true, true, true}, {"sort_by=model", true, false, true}, {"api_key_id=abc", false, false, true},
		{"start_time=2026-10-15T00:00:00Z&end_time=2026-10-14T00:00:00Z", false, false, true}, {"end_time=bad", false, false, true},
		{"page=2&user_id=99&account_id=8&api_key_id=3&group_id=66&status=queued&media_type=video&settlement_state=held&sort_order=asc", true, true, false},
	} {
		t.Run(tc.query, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/media/tasks?"+tc.query, nil)
			f, paged, err := parseMediaTaskFilter(c, 42, tc.admin)
			require.Equal(t, tc.paged, paged)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if !tc.admin {
				require.EqualValues(t, 42, f.UserID)
			}
		})
	}
}

func TestMediaTaskListUserEnvelopeAndLegacyCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, paged := range []bool{true, false} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		query := "before=cursor"
		if paged {
			query = "page=3&page_size=20"
			mock.ExpectQuery(`SELECT COUNT\(\*\).*user_id=\$1`).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(45))
			mock.ExpectQuery(`SELECT .*user_id=\$1 ORDER BY created_at DESC,id DESC LIMIT \$2 OFFSET \$3`).WithArgs(int64(42), 20, 40).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		} else {
			mock.ExpectQuery(`SELECT .*created_at,id.*ORDER BY created_at DESC,id DESC`).WithArgs(int64(42), 50, "cursor").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/media/tasks?"+query, nil)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		h := &MediaHandler{Engine: &media.Engine{Store: &media.Store{DB: db}}}
		h.UserTasks(c)
		require.Equal(t, 200, w.Code)
		if paged {
			require.Contains(t, w.Body.String(), `"total":45`)
			require.Contains(t, w.Body.String(), `"pages":3`)
			require.Contains(t, w.Body.String(), `"items":[]`)
		} else {
			require.Contains(t, w.Body.String(), `"data":[]`)
			require.NotContains(t, w.Body.String(), `"items"`)
		}
		require.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	}
}

func TestMediaTaskListRejectsMissingOrInvalidUser(t *testing.T) {
	for _, id := range []int64{0, -1} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/media/tasks?page=1", nil)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: id})
		(&MediaHandler{}).UserTasks(c)
		require.Equal(t, 401, c.Writer.Status())
	}
}
