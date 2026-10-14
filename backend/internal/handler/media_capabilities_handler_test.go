package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mediaGroupsStub struct{ groups []service.Group }

func (s mediaGroupsStub) GetAvailableGroups(context.Context, int64) ([]service.Group, error) {
	return s.groups, nil
}

func TestMediaCapabilitiesAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		user   bool
		query  string
		groups []service.Group
		status int
	}{
		{"no user", false, "group_id=7", nil, 401},
		{"invalid", true, "group_id=-1", nil, 400},
		{"not bindable", true, "group_id=7", []service.Group{{ID: 8, Platform: "imagevideo", Status: "active"}}, 403},
		{"inactive", true, "group_id=7", []service.Group{{ID: 7, Platform: "imagevideo", Status: "inactive"}}, 403},
		{"chat", true, "group_id=7", []service.Group{{ID: 7, Platform: "openai", Status: "active"}}, 403},
		{"allowed", true, "group_id=7", []service.Group{{ID: 7, Platform: "imagevideo", Status: "active"}}, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("GET", "/media/capabilities?"+tc.query, nil)
			if tc.user {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
			}
			h := &MediaHandler{groups: mediaGroupsStub{tc.groups}, Engine: &media.Engine{Config: func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) { return nil, nil, nil }}}
			h.Capabilities(c)
			require.Equal(t, tc.status, recorder.Code)
		})
	}
}
