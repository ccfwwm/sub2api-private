//go:build unit

package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/branding"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type settingHandlerPublicRepoStub struct {
	values map[string]string
}

func TestPublicLogoIsCachedSeparatelyFromSettingsAndInjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII="
	data, err := base64.StdEncoding.DecodeString(png)
	require.NoError(t, err)
	// Uploaded branding images can be much larger than the application HTML.
	data = append(data, make([]byte, 150000)...)
	source := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	repo := &settingHandlerPublicRepoStub{values: map[string]string{service.SettingKeySiteLogo: source}}
	svc := service.NewSettingService(repo, &config.Config{})
	h := NewSettingHandler(svc, "test-version")
	r := gin.New()
	r.GET("/api/v1/settings/public", h.GetPublicSettings)
	r.GET(branding.LogoPath+":asset", h.GetSiteLogo)
	r.HEAD(branding.LogoPath+":asset", h.GetSiteLogo)
	settings := httptest.NewRecorder()
	r.ServeHTTP(settings, httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil))
	require.Equal(t, http.StatusOK, settings.Code)
	require.Less(t, settings.Body.Len(), 10000)
	var payload struct {
		Data struct {
			SiteLogo string `json:"site_logo"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(settings.Body.Bytes(), &payload))
	url := payload.Data.SiteLogo
	require.Equal(t, branding.PublicLogoURL(source), url)
	injected, err := svc.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)
	require.Equal(t, url, injected.(*service.PublicSettingsInjectionPayload).SiteLogo)
	image := httptest.NewRecorder()
	r.ServeHTTP(image, httptest.NewRequest(http.MethodGet, url, nil))
	require.Equal(t, http.StatusOK, image.Code)
	require.Equal(t, data, image.Body.Bytes())
	require.Equal(t, "image/png", image.Header().Get("Content-Type"))
	require.Contains(t, image.Header().Get("Cache-Control"), "immutable")
	for _, tag := range []string{image.Header().Get("ETag"), "W/" + image.Header().Get("ETag")} {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("If-None-Match", tag)
		cached := httptest.NewRecorder()
		r.ServeHTTP(cached, req)
		require.Equal(t, http.StatusNotModified, cached.Code)
		require.Zero(t, cached.Body.Len())
	}
	head := httptest.NewRecorder()
	r.ServeHTTP(head, httptest.NewRequest(http.MethodHead, url, nil))
	require.Equal(t, http.StatusOK, head.Code)
	require.Zero(t, head.Body.Len())
	raw, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, source, raw.SiteLogo)
	require.Equal(t, source, repo.values[service.SettingKeySiteLogo])
	// Changing branding invalidates its URL without changing cached image bytes.
	repo.values[service.SettingKeySiteLogo] = "/new-logo.png"
	stale := httptest.NewRecorder()
	r.ServeHTTP(stale, httptest.NewRequest(http.MethodGet, url, nil))
	require.Equal(t, http.StatusNotFound, stale.Code)
}

func (s *settingHandlerPublicRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *settingHandlerPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingHandlerPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingHandlerPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingHandlerPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingHandlerPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingHandlerPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingHandler_GetPublicSettings_ExposesForceEmailOnThirdPartySignup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyForceEmailOnThirdPartySignup: "true",
		},
	}
	h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ForceEmailOnThirdPartySignup bool `json:"force_email_on_third_party_signup"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.ForceEmailOnThirdPartySignup)
}

func TestSettingHandler_GetPublicSettings_ExposesTencentCaptchaConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyTencentCaptchaEnabled: "true",
			service.SettingKeyTencentCaptchaAppID:   "123456789",
			service.SettingKeyTencentCaptchaRegion:  service.TencentCaptchaRegionINTL,
		},
	}
	h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			TencentCaptchaEnabled bool   `json:"tencent_captcha_enabled"`
			TencentCaptchaAppID   string `json:"tencent_captcha_app_id"`
			TencentCaptchaRegion  string `json:"tencent_captcha_region"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.TencentCaptchaEnabled)
	require.Equal(t, "123456789", resp.Data.TencentCaptchaAppID)
	require.Equal(t, service.TencentCaptchaRegionINTL, resp.Data.TencentCaptchaRegion)
}

func TestSettingHandler_GetPublicSettings_ExposesWeChatOAuthModeCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyWeChatConnectEnabled:             "true",
			service.SettingKeyWeChatConnectAppID:               "wx-mp-app",
			service.SettingKeyWeChatConnectAppSecret:           "wx-mp-secret",
			service.SettingKeyWeChatConnectMode:                "mp",
			service.SettingKeyWeChatConnectScopes:              "snsapi_base",
			service.SettingKeyWeChatConnectOpenEnabled:         "true",
			service.SettingKeyWeChatConnectMPEnabled:           "true",
			service.SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
			service.SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			WeChatOAuthEnabled     bool `json:"wechat_oauth_enabled"`
			WeChatOAuthOpenEnabled bool `json:"wechat_oauth_open_enabled"`
			WeChatOAuthMPEnabled   bool `json:"wechat_oauth_mp_enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.WeChatOAuthEnabled)
	require.True(t, resp.Data.WeChatOAuthOpenEnabled)
	require.True(t, resp.Data.WeChatOAuthMPEnabled)
}
