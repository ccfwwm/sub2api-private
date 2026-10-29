package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func TestUserArtifactTicketAuthorizationAndRange(t *testing.T) {
	for _, tc := range []struct {
		name     string
		expires  int64
		audience string
		owner    int64
		method   string
		status   int
	}{
		{"range", time.Now().Add(time.Minute).Unix(), "user_media_artifact", 7, "GET", 206},
		{"head", time.Now().Add(time.Minute).Unix(), "user_media_artifact", 7, "HEAD", 200},
		{"expired", time.Now().Add(-time.Minute).Unix(), "user_media_artifact", 7, "GET", 403},
		{"wrong audience", time.Now().Add(time.Minute).Unix(), "admin", 7, "GET", 403},
		{"wrong owner", time.Now().Add(time.Minute).Unix(), "user_media_artifact", 8, "GET", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("s", 32))
			require.NoError(t, err)
			h := &MediaHandler{Engine: &media.Engine{Store: &media.Store{DB: db}, Blobs: blobs}}
			credential := fmt.Sprintf("%x", sha256.Sum256([]byte("u@example.org\nhash")))
			if tc.status != 403 || tc.name == "wrong owner" {
				mock.ExpectQuery("SELECT email,password_hash FROM users").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"email", "password_hash"}).AddRow("u@example.org", "hash"))
				columns := make([]string, 23)
				for i := range columns {
					columns[i] = fmt.Sprint(i)
				}
				snapshot, _ := json.Marshal(media.Snapshot{Identity: media.Identity{UserID: tc.owner}})
				mock.ExpectQuery("SELECT .* FROM media_tasks WHERE id").WithArgs("mt_user").WillReturnRows(sqlmock.NewRows(columns).AddRow("mt_user", "completed", "settled", "", "", "", tc.owner, 1, 19, 0, 2114, snapshot, []byte(`{}`), "0", "0", "0", "0", "0", 1, 0, time.Now(), nil, nil))
			}
			if tc.status != 403 {
				path := filepath.Join(t.TempDir(), "artifact")
				require.NoError(t, os.WriteFile(path, []byte("0123456789"), 0600))
				mock.ExpectQuery("SELECT file_path,mime_type,content_hash,available FROM media_artifacts").WithArgs("mt_user", "video1").WillReturnRows(sqlmock.NewRows([]string{"file_path", "mime_type", "content_hash", "available"}).AddRow(path, "video/mp4", "hash", true))
			}
			raw, _ := json.Marshal(userArtifactTicket{Audience: tc.audience, User: 7, Task: "mt_user", Key: "video1", Expires: tc.expires, Credential: credential})
			sealed, err := blobs.SealAccess(raw)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "id", Value: "mt_user"}, {Key: "key", Value: "video1"}}
			c.Request = httptest.NewRequest(tc.method, "/content?access="+base64.RawURLEncoding.EncodeToString(sealed), nil)
			if tc.method == "GET" {
				c.Request.Header.Set("Range", "bytes=2-5")
			}
			h.UserArtifactContent(c)
			require.Equal(t, tc.status, w.Code)
			if tc.status == 206 {
				require.Equal(t, "2345", w.Body.String())
				require.Equal(t, "bytes 2-5/10", w.Header().Get("Content-Range"))
			}
			if tc.method == "HEAD" {
				require.Empty(t, w.Body.String())
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
