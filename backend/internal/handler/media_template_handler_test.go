package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProtocolEndpointsDenyMissingAdministratorBeforeAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &MediaHandler{}
	for _, handler := range []gin.HandlerFunc{h.AdminProtocolList, h.AdminProtocolHistory, h.AdminProtocolSave, h.AdminProtocolExport, h.AdminProtocolArchive, h.AdminProtocolPreview, h.AdminProtocolDiscover, h.AdminProtocolTest, h.AdminProtocolEffective} {
		for _, identity := range []struct {
			name   string
			role   string
			userID int64
		}{
			{name: "missing role"},
			{name: "ordinary user", role: "user", userID: 10},
			{name: "admin without identity", role: "admin"},
			{name: "admin with invalid identity", role: "admin", userID: 0},
		} {
			t.Run(identity.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest("GET", "/", nil)
				if identity.role != "" {
					c.Set(string(middleware.ContextKeyUserRole), identity.role)
				}
				if identity.userID != 0 {
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: identity.userID})
				}
				handler(c)
				require.Equal(t, 403, w.Code)
			})
		}
	}
}
