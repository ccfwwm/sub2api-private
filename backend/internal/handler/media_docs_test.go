package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const validDocument = `[{"id":"start","group":"Guide","title":"Start","blocks":[{"kind":"text","text":"Hello"},{"kind":"endpoint","method":"POST","path":"/v1/images/edits"},{"kind":"code","language":"bash","text":"curl ..."},{"kind":"table","headers":["A","B"],"rows":[["1","2"]]}]}]`

func docsContext(method, path, action, body string, admin bool) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	if action != "" {
		c.Params = gin.Params{{Key: "action", Value: action}}
	}
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	if admin {
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
	} else {
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
	}
	return c, w
}

func TestAdminImageVideoDocumentRejectsNonAdminBeforeDatabaseAccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	c, w := docsContext(http.MethodGet, "/docs", "get", "", false)
	(&MediaHandler{db: db}).AdminImageVideoDocument(c)
	require.Equal(t, http.StatusForbidden, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPublicImageVideoDocumentReturnsPublishedContentOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM public_documents WHERE slug=\$1\)`).
		WithArgs(imageVideoDocumentSlug).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT published_version,published_content FROM public_documents WHERE slug=\$1`).
		WithArgs(imageVideoDocumentSlug).WillReturnRows(sqlmock.NewRows([]string{"version", "content"}).AddRow(7, []byte(validDocument)))
	c, w := docsContext(http.MethodGet, "/api/v1/public-docs/imagevideo", "", "", false)
	(&MediaHandler{db: db}).PublicImageVideoDocument(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"version":7`)
	require.NotContains(t, w.Body.String(), `"draft_content"`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestImageVideoPublishUsesExpectedVersionAndAtomicallyUpdatesPublishedContent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM public_documents WHERE slug=\$1\)`).
		WithArgs(imageVideoDocumentSlug).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT draft_version,published_version FROM public_documents WHERE slug=\$1 FOR UPDATE`).
		WithArgs(imageVideoDocumentSlug).WillReturnRows(sqlmock.NewRows([]string{"draft", "published"}).AddRow(4, 3))
	mock.ExpectExec(`UPDATE public_documents SET draft_version=\$1,draft_content=\$2,published_version=\$1,published_content=\$2,published_at=NOW\(\),updated_at=NOW\(\) WHERE slug=\$3`).
		WithArgs(int64(5), sqlmock.AnyArg(), imageVideoDocumentSlug).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO public_document_versions\(slug,version,action,content,actor_user_id\)`).
		WithArgs(imageVideoDocumentSlug, int64(5), "publish", sqlmock.AnyArg(), int64(42)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	c, w := docsContext(http.MethodPost, "/admin/docs/imagevideo/publish", "publish", `{"expected_version":4,"content":`+validDocument+`}`, true)
	(&MediaHandler{db: db}).AdminImageVideoDocument(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"version":5`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestImageVideoPublishVersionConflictDoesNotMutateDocument(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM public_documents WHERE slug=\$1\)`).
		WithArgs(imageVideoDocumentSlug).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT draft_version,published_version FROM public_documents WHERE slug=\$1 FOR UPDATE`).
		WithArgs(imageVideoDocumentSlug).WillReturnRows(sqlmock.NewRows([]string{"draft", "published"}).AddRow(8, 7))
	mock.ExpectRollback()
	c, w := docsContext(http.MethodPost, "/admin/docs/imagevideo/publish", "publish", `{"expected_version":4,"content":`+validDocument+`}`, true)
	(&MediaHandler{db: db}).AdminImageVideoDocument(c)
	require.Equal(t, http.StatusConflict, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestImageVideoDraftSaveDoesNotPublish(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM public_documents WHERE slug=\$1\)`).
		WithArgs(imageVideoDocumentSlug).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT draft_version,published_version FROM public_documents WHERE slug=\$1 FOR UPDATE`).
		WithArgs(imageVideoDocumentSlug).WillReturnRows(sqlmock.NewRows([]string{"draft", "published"}).AddRow(2, 1))
	mock.ExpectExec(`UPDATE public_documents SET draft_version=\$1,draft_content=\$2,updated_at=NOW\(\) WHERE slug=\$3`).
		WithArgs(int64(3), sqlmock.AnyArg(), imageVideoDocumentSlug).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO public_document_versions\(slug,version,action,content,actor_user_id\)`).
		WithArgs(imageVideoDocumentSlug, int64(3), "draft", sqlmock.AnyArg(), int64(42)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	c, w := docsContext(http.MethodPut, "/admin/docs/imagevideo/draft", "save", `{"expected_version":2,"content":`+validDocument+`}`, true)
	(&MediaHandler{db: db}).AdminImageVideoDocument(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestImageVideoDocumentRejectsInvalidStructuredContent(t *testing.T) {
	for _, content := range []string{
		`[{"id":"same","group":"Guide","title":"One","blocks":[{"kind":"text","text":"A"}]},{"id":"same","group":"Guide","title":"Two","blocks":[{"kind":"text","text":"B"}]}]`,
		`[{"id":"start","group":"Guide","title":"Start","blocks":[{"kind":"endpoint","method":"GET","path":"https://elsewhere.example"}]}]`,
		`[{"id":"start","group":"Guide","title":"Start","blocks":[{"kind":"table","headers":["A","B"],"rows":[["only one"]]}]}]`,
	} {
		t.Run(content[:min(len(content), 32)], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM public_documents WHERE slug=\$1\)`).
				WithArgs(imageVideoDocumentSlug).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT draft_version,published_version FROM public_documents WHERE slug=\$1 FOR UPDATE`).
				WithArgs(imageVideoDocumentSlug).WillReturnRows(sqlmock.NewRows([]string{"draft", "published"}).AddRow(1, 1))
			mock.ExpectRollback()
			c, w := docsContext(http.MethodPut, "/admin/docs/imagevideo/draft", "save", `{"expected_version":1,"content":`+content+`}`, true)
			(&MediaHandler{db: db}).AdminImageVideoDocument(c)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
