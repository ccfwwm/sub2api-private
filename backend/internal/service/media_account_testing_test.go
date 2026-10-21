//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mediaTestRepo struct {
	AccountRepository
	account *Account
}

func (r mediaTestRepo) GetByID(context.Context, int64) (*Account, error)         { return r.account, nil }
func (r mediaTestRepo) UpdateExtra(context.Context, int64, map[string]any) error { return nil }

func testMediaTester(t *testing.T, cap media.Capability, base string) (*AccountTestService, *mediaAccountTester) {
	t.Helper()
	account := &Account{ID: 42, Platform: PlatformImageVideo, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": base, "api_key": "unit-secret"}, Extra: map[string]any{"media_models": []media.Capability{cap}}}
	blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("a", 32))
	require.NoError(t, err)
	svc := &AccountTestService{accountRepo: mediaTestRepo{account: account}}
	tester := &mediaAccountTester{service: svc, blobs: blobs, adapter: &media.Adapter{Client: media.SafeClient(true)}, active: map[int64]string{}}
	svc.mediaTests = tester
	return svc, tester
}
func testMediaRecord(t *testing.T, svc *AccountTestService, cap media.Capability) *mediaTestRecord {
	t.Helper()
	account, _ := svc.accountRepo.GetByID(context.Background(), 42)
	return &mediaTestRecord{Result: MediaTestResult{ID: "mt_unittest", AccountID: 42, Model: cap.Model, Operation: cap.Operations[0], Kind: "minimal", State: "submitting", StartedAt: time.Now().UTC()}, Capability: cap, Fingerprint: testFingerprint(account, cap), Parameters: minimalTestParameters(cap, "custom prompt"), Files: map[string]string{}}
}
func TestMediaTestDisabledUnpricedUsesImageAdapterAndValidatesOutput(t *testing.T) {
	cap, ok := media.ModelPreset("gpt-image-2")
	require.True(t, ok)
	cap.Enabled = false
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/images/generations", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		posts.Add(1)
		var p map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&p))
		require.Equal(t, "custom prompt", p["prompt"])
		require.Equal(t, float64(1), p["n"])
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(raw.Bytes())}}})
	}))
	defer server.Close()
	svc, tester := testMediaTester(t, cap, server.URL+"/v1")
	record := testMediaRecord(t, svc, cap)
	tester.work(record, true)
	require.Equal(t, int32(1), posts.Load())
	require.Equal(t, "completed", record.Result.State)
	require.True(t, record.Result.Artifacts[0].Available)
	require.NotContains(t, record.Result.Artifacts[0].ContentURL, "unit-secret")
	result, err := svc.MediaTestResult(context.Background(), 42, record.Result.ID)
	require.NoError(t, err)
	require.False(t, result.ConfigChanged)
	_, _, err = svc.MediaTestArtifact(context.Background(), 43, record.Result.ID, record.Result.Artifacts[0].Key)
	require.Error(t, err)
	encrypted, err := os.ReadFile(filepath.Join(tester.blobs.Root, record.Result.ID, "state"))
	require.NoError(t, err)
	require.NotContains(t, string(encrypted), "custom prompt")
}
func TestMediaTestAcceptedWithoutArtifactNeverPassesOrResubmits(t *testing.T) {
	cap, _ := media.ModelPreset("omni-flash")
	cap.Delivery.PollSeconds = 1
	seconds := 2
	cap.Limits.ObservationSeconds = &seconds
	var posts atomic.Int32
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts.Add(1)
		} else {
			gets.Add(1)
		}
		_, _ = w.Write([]byte(`{"id":"remote","status":"processing"}`))
	}))
	defer server.Close()
	svc, tester := testMediaTester(t, cap, server.URL)
	r := testMediaRecord(t, svc, cap)
	tester.work(r, true)
	require.Equal(t, "unknown", r.Result.State)
	require.Equal(t, int32(1), posts.Load())
	require.Positive(t, gets.Load())
	require.Equal(t, "remote", r.UpstreamID)
	// A restart may resume GET only. It never sends another generation request.
	r.Result.StartedAt = time.Now().UTC()
	tester.work(r, false)
	require.Equal(t, int32(1), posts.Load())
}
func TestMediaTestResumeStoredSourceAndRejectInvalidArtifact(t *testing.T) {
	cap, _ := media.ModelPreset("gpt-image-2")
	svc, tester := testMediaTester(t, cap, "https://example.invalid")
	r := testMediaRecord(t, svc, cap)
	r.Result.State = "in_progress"
	r.Result.UpstreamStatus = "completed"
	r.Result.Artifacts = []media.Artifact{{Key: "output_0", Type: "image", Source: "data:image/png;base64,QUJD"}}
	tester.work(r, false)
	require.Equal(t, "failed", r.Result.State)
	require.Contains(t, r.Result.Error, "invalid image artifact")
}
func TestMediaUnifiedMappingRoundtripAndRemoval(t *testing.T) {
	cap, _ := media.ModelPreset("omni-flash")
	cap.Model = "alias"
	cap.Enabled = true
	cap.UpstreamModel = "omni-flash"
	creds, extra, err := NormalizeMediaAccountConfig(PlatformImageVideo, map[string]any{"api_key": "key"}, map[string]any{"media_models": []media.Capability{cap}}, false)
	require.NoError(t, err)
	require.Equal(t, "omni-flash", creds["model_mapping"].(map[string]any)["alias"])
	creds["model_mapping"] = map[string]any{"alias": "veo-3.1-fast"}
	_, updated, err := NormalizeMediaAccountConfig(PlatformImageVideo, creds, extra, true)
	require.NoError(t, err)
	caps, err := accountMediaModels(updated)
	require.NoError(t, err)
	require.Len(t, caps, 1)
	require.True(t, caps[0].Enabled)
	require.Equal(t, "veo-3.1-fast", caps[0].UpstreamModel)
	creds, updated, err = NormalizeMediaAccountConfig(PlatformImageVideo, creds, map[string]any{"media_models": []any{}}, false)
	require.NoError(t, err)
	require.Empty(t, creds["model_mapping"])
	require.Empty(t, updated["media_models"])
	creds, updated, err = NormalizeMediaAccountConfig(PlatformImageVideo, map[string]any{"model_mapping": map[string]any{"unknown": "provider-unknown"}}, nil, true)
	require.NoError(t, err)
	require.Equal(t, "provider-unknown", creds["model_mapping"].(map[string]any)["unknown"])
	caps, err = accountMediaModels(updated)
	require.NoError(t, err)
	require.Len(t, caps, 1)
	require.False(t, caps[0].Enabled)
	require.Equal(t, "unknown", caps[0].Model)
	require.Equal(t, "provider-unknown", caps[0].UpstreamModel)
	require.Equal(t, "待配置协议模板", caps[0].ValidationStatus)
	require.Empty(t, caps[0].Adapter)
	require.Nil(t, caps[0].TemplateRef)
}
func TestMediaMinimumDurationAndParameterValidation(t *testing.T) {
	for model, want := range map[string]int{"omni-flash": 4, "veo-3.1-lite": 8} {
		cap, _ := media.ModelPreset(model)
		p := minimalTestParameters(cap, "")
		require.Equal(t, want, p["seconds"])
		_, err := media.Normalize(cap, "video", p)
		require.NoError(t, err)
		p["untrusted"] = 1
		req, err := media.Normalize(cap, "video", p)
		require.NoError(t, err)
		require.Equal(t, float64(1), req.Parameters["untrusted"])
		for _, seconds := range []any{0, -1, 1.5, 3601, "invalid"} {
			p["seconds"] = seconds
			_, err = media.Normalize(cap, "video", p)
			require.ErrorContains(t, err, "invalid_seconds")
		}
	}
}

func TestMediaAccountCapabilityAndLegacyMappingUpdatesPreserveCredentials(t *testing.T) {
	for _, mappingOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(mappingOnly), func(t *testing.T) {
			cap, ok := media.ModelPreset("omni-flash")
			require.True(t, ok)
			cap.Enabled = false
			account := &Account{ID: 42, Platform: PlatformImageVideo, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://img.yunfei.best", "api_key": "private-key", "api_protocol": "chat_completions", "model_mapping": map[string]any{cap.Model: cap.UpstreamModel}}, Extra: map[string]any{"media_models": []media.Capability{cap}, "media_last_test": map[string]any{"test_id": "existing-real-test"}}}
			before := testFingerprint(account, cap)
			repo := &longContextBillingRepoStub{account: account}
			input := &UpdateAccountInput{}
			if mappingOnly {
				input.Credentials = map[string]any{"model_mapping": map[string]any{cap.Model: cap.UpstreamModel}}
			} else {
				cap.Enabled = true
				input.Extra = map[string]any{"media_models": []media.Capability{cap}, "media_last_test": map[string]any{"test_id": "fake-overwrite"}}
			}
			updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), 42, input)
			require.NoError(t, err)
			require.Equal(t, "https://img.yunfei.best", updated.GetCredential("base_url"))
			require.Equal(t, "private-key", updated.GetCredential("api_key"))
			require.Equal(t, "chat_completions", updated.GetCredential("api_protocol"))
			stored, err := accountTestCapability(updated, cap.Model)
			require.NoError(t, err)
			require.Equal(t, before, testFingerprint(updated, stored))
			require.Equal(t, map[string]any{"test_id": "existing-real-test"}, updated.Extra["media_last_test"])
		})
	}
}

func TestMediaConnectionRejectsHTTP200ErrorEnvelope(t *testing.T) {
	for _, body := range []string{`{"error":{"message":"denied"}}`, `{"message":"login required"}`, `null`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "/v1/models", r.URL.Path)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			cfg.Security.URLAllowlist.AllowPrivateHosts = true
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/test", nil)
			svc := &AccountTestService{cfg: cfg}
			err := svc.testConnectionOnly(c, &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": server.URL, "api_key": "unit-secret"}})
			require.Error(t, err)
			require.False(t, c.GetBool("account_test_completed_success"))
		})
	}
}
