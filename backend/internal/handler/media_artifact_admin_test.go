package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestArtifactTicketScopeExpiryAndRevocation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, task, key string
		expiry          int64
		valid           bool
		storage         bool
	}{
		{"valid", "task-a", "video", time.Now().Add(time.Minute).Unix(), true, false},
		{"expired", "task-a", "video", time.Now().Add(-time.Minute).Unix(), false, false},
		{"other task", "task-b", "video", time.Now().Add(time.Minute).Unix(), false, false},
		{"other artifact", "task-a", "image", time.Now().Add(time.Minute).Unix(), false, false},
		{"storage substitution", "task-a", "video", time.Now().Add(time.Minute).Unix(), false, true},
		{"revoked administrator", "task-a", "video", time.Now().Add(time.Minute).Unix(), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("s", 32))
			require.NoError(t, err)
			h := &MediaHandler{Engine: &media.Engine{Store: &media.Store{DB: db}, Blobs: blobs}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/", nil)
			credentialQuery := `SELECT email,password_hash FROM users`
			mock.ExpectQuery(credentialQuery).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"email", "password_hash"}).AddRow("admin@example.invalid", "hash"))
			credential, err := h.adminCredential(c, 1)
			require.NoError(t, err)
			ticket := adminArtifactTicket{Admin: 1, Task: tc.task, Key: tc.key, Expires: tc.expiry, Credential: credential}
			raw, _ := json.Marshal(ticket)
			var sealed []byte
			if tc.storage {
				sealed, err = blobs.Seal(raw)
			} else {
				sealed, err = blobs.SealAccess(raw)
			}
			require.NoError(t, err)
			if tc.valid {
				mock.ExpectQuery(credentialQuery).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"email", "password_hash"}).AddRow("admin@example.invalid", "hash"))
			}
			if tc.name == "revoked administrator" {
				mock.ExpectQuery(credentialQuery).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"email", "password_hash"}))
			}
			router := gin.New()
			router.GET("/tasks/:id/artifacts/:key", h.ArtifactAccessAuth, func(c *gin.Context) { require.True(t, requireMediaAdmin(c)); c.Status(204) })
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/tasks/task-a/artifacts/video?access="+base64.RawURLEncoding.EncodeToString(sealed), nil))
			expected := 403
			if tc.valid {
				expected = 204
			}
			require.Equal(t, expected, w.Code)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestArtifactRangeHeadAndGone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, method := range []string{"GET", "HEAD"} {
		t.Run(method, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			path := filepath.Join(t.TempDir(), "movie.mp4")
			require.NoError(t, os.WriteFile(path, []byte("0123456789"), 0600))
			mock.ExpectQuery(`SELECT file_path,mime_type,content_hash,available FROM media_artifacts`).WithArgs("task-a", "video").WillReturnRows(sqlmock.NewRows([]string{"file_path", "mime_type", "content_hash", "available"}).AddRow(path, "video/mp4", "fixture-hash", true))
			h := &MediaHandler{Engine: &media.Engine{Store: &media.Store{DB: db}}}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "key", Value: "video"}}
			c.Request = httptest.NewRequest(method, "/content", nil)
			if method == "GET" {
				c.Request.Header.Set("Range", "bytes=2-5")
			}
			h.serveArtifact(c, media.StoredTask{ID: "task-a", State: "completed"})
			if method == "GET" {
				require.Equal(t, http.StatusPartialContent, w.Code)
				require.Equal(t, "2345", w.Body.String())
				require.Equal(t, "bytes 2-5/10", w.Header().Get("Content-Range"))
			} else {
				require.Equal(t, 200, w.Code)
				require.Empty(t, w.Body.String())
			}
			require.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestMediaReviewSummaryDropsSecretsAndInternalLocations(t *testing.T) {
	raw := map[string]any{"model": "test", "api_key": "secret", "password": "secret", "images": []any{"data:image/png;base64,AAAA", "/app/private/file.png", "https://user:secret@example.com/a.png?token=secret"}, "usage": map[string]any{"input_tokens": 12, "credential": "secret"}}
	clean := mediaReviewValue(raw, 0)
	encoded, err := json.Marshal(clean)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret")
	require.NotContains(t, string(encoded), "/app/private")
	require.NotContains(t, string(encoded), "base64,AAAA")
	require.Contains(t, string(encoded), "input_tokens")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	(&MediaHandler{}).AdminTaskExport(c)
	require.Equal(t, 403, w.Code)
}
