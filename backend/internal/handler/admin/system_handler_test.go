//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type systemHandlerUpdateServiceStub struct {
	performErr            error
	updateInfo            *service.UpdateInfo
	checkErr              error
	checkForces           []bool
	performCall           int
	performCtxErr         error
	performHasDeadline    bool
	rollbackCall          int
	rollbackToCall        int
	rollbackToCtxErr      error
	rollbackToHasDeadline bool
	rollbackToVersions    []string
	rollbackToErr         error
	rollbackVersions      []service.RollbackVersion
	rollbackVersionsErr   error
	rollbackVersionsCall  int
}

func (s *systemHandlerUpdateServiceStub) CheckUpdate(_ context.Context, force bool) (*service.UpdateInfo, error) {
	s.checkForces = append(s.checkForces, force)
	return s.updateInfo, s.checkErr
}

func (s *systemHandlerUpdateServiceStub) PerformUpdate(ctx context.Context) error {
	s.performCall++
	s.performCtxErr = ctx.Err()
	_, s.performHasDeadline = ctx.Deadline()
	return s.performErr
}

func (s *systemHandlerUpdateServiceStub) Rollback() error {
	s.rollbackCall++
	return nil
}

func (s *systemHandlerUpdateServiceStub) ListRollbackVersions(context.Context) ([]service.RollbackVersion, error) {
	s.rollbackVersionsCall++
	return s.rollbackVersions, s.rollbackVersionsErr
}

func (s *systemHandlerUpdateServiceStub) RollbackToVersion(ctx context.Context, version string) error {
	s.rollbackToCall++
	s.rollbackToCtxErr = ctx.Err()
	_, s.rollbackToHasDeadline = ctx.Deadline()
	s.rollbackToVersions = append(s.rollbackToVersions, version)
	return s.rollbackToErr
}

type systemUpdateResponseEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Message         string `json:"message"`
		AlreadyUpToDate bool   `json:"already_up_to_date"`
		CurrentVersion  string `json:"current_version"`
		LatestVersion   string `json:"latest_version"`
		OperationID     string `json:"operation_id"`
	} `json:"data"`
}

type systemUpdateErrorEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newSystemHandlerTestRouter(t *testing.T, updateSvc *systemHandlerUpdateServiceStub, repo *memoryIdempotencyRepoStub) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() {
		service.SetDefaultIdempotencyCoordinator(nil)
	})

	lockSvc := service.NewSystemOperationLockService(repo, service.IdempotencyConfig{
		ProcessingTimeout:  time.Second,
		SystemOperationTTL: time.Minute,
	})
	handler := NewSystemHandler(updateSvc, lockSvc)

	router := gin.New()
	router.POST("/api/v1/admin/system/update", handler.PerformUpdate)
	router.POST("/api/v1/admin/system/rollback", handler.Rollback)
	router.GET("/api/v1/admin/system/rollback-versions", handler.GetRollbackVersions)
	return router
}

func requireSystemLockStatus(t *testing.T, repo *memoryIdempotencyRepoStub, wantStatus string) {
	t.Helper()
	repo.mu.Lock()
	defer repo.mu.Unlock()

	for _, record := range repo.data {
		if record.Status == wantStatus {
			return
		}
	}
	t.Fatalf("system lock status %q not found in records: %#v", wantStatus, repo.data)
}

func TestSystemHandlerBlocksLegacyMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &systemHandlerUpdateServiceStub{}
	h := NewSystemHandler(svc, nil)
	for _, handler := range []gin.HandlerFunc{h.PerformUpdate, h.Rollback, h.RestartService} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		handler(c)
		require.Equal(t, http.StatusConflict, w.Code)
	}
	require.Zero(t, svc.performCall)
	require.Zero(t, svc.rollbackCall)
	require.Zero(t, svc.rollbackToCall)
}

func TestSystemHandlerGetRollbackVersions(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		rollbackVersions: []service.RollbackVersion{
			{Version: "0.1.146", PublishedAt: "2026-07-07T00:00:00Z", HTMLURL: "https://example.com/v0.1.146"},
			{Version: "0.1.145", PublishedAt: "2026-07-06T00:00:00Z", HTMLURL: "https://example.com/v0.1.145"},
		},
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/rollback-versions", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, updateSvc.rollbackVersionsCall)

	var body struct {
		Code int `json:"code"`
		Data struct {
			Versions []service.RollbackVersion `json:"versions"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Len(t, body.Data.Versions, 2)
	require.Equal(t, "0.1.146", body.Data.Versions[0].Version)
}

func TestSystemHandlerGetRollbackVersionsError(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		rollbackVersionsErr: errors.New("github unavailable"),
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/rollback-versions", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
