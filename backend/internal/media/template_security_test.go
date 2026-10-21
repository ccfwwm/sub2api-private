package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func templateSecurityRequest(t *testing.T, capability Capability) Request {
	t.Helper()
	request, err := Normalize(capability, "video", map[string]any{"prompt": "fixture", "seconds": 5})
	require.NoError(t, err)
	wire, err := BuildTemplateRequest(context.Background(), nil, capability, request)
	require.NoError(t, err)
	request.Wire = &wire
	return request
}

func TestTemplateFailedHTTP200AndPollRedactAccountCredentials(t *testing.T) {
	const key = "fixture-private-account-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "failed", "error": map[string]any{"message": "account " + key + " rejected; Bearer sk-private-token; REF_UNSUPPORTED"}})
	}))
	defer server.Close()
	template := templateFixture(t, "video-arrays")
	capability := Capability{Model: "public-video", UpstreamModel: "private-video", MediaType: "video", Template: &template}
	adapter := &Adapter{Client: SafeClient(true)}
	account := Account{BaseURL: server.URL, Key: key}
	for _, operation := range []string{"submit", "poll"} {
		t.Run(operation, func(t *testing.T) {
			var result UpstreamResult
			var err error
			if operation == "submit" {
				result, err = adapter.Submit(context.Background(), capability, templateSecurityRequest(t, capability), account)
			} else {
				result, err = adapter.Poll(context.Background(), StoredTask{UpstreamID: "existing-task", Snapshot: Snapshot{Capability: capability}}, account)
			}
			require.NoError(t, err)
			require.Equal(t, "failed", result.Status)
			require.Contains(t, result.Diagnostic, "REF_UNSUPPORTED")
			require.NotContains(t, result.Diagnostic, key)
			require.NotContains(t, result.Diagnostic, "sk-private-token")
		})
	}
	public := publicTaskValue(StoredTask{Diagnostic: "old failure Bearer sk-private-token at https://provider.invalid/private?key=hidden", Snapshot: Snapshot{Capability: capability}}, nil)
	raw, err := json.Marshal(public)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "sk-private-token")
	require.NotContains(t, string(raw), "provider.invalid")
}

func TestTemplateAuthenticationStaysOnOriginAcrossSubmissionDiscoveryAndDownload(t *testing.T) {
	const key = "fixture-redirect-key"
	for _, operation := range []string{"submit", "discover", "download"} {
		for _, mode := range []string{"header", "query"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				auth := TemplateAuth{Mode: mode, Name: "X-Unusual-Auth", Prefix: "custom "}
				var calls []string
				cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls = append(calls, "cdn")
					require.Empty(t, r.Header.Get(auth.Name))
					require.Empty(t, r.Header.Get("Authorization"))
					require.NotContains(t, r.Referer(), key)
					require.NotContains(t, r.URL.RawQuery, key)
					require.Equal(t, "cdn-signed-value", r.URL.Query().Get("cdn_signature"))
					if operation == "download" {
						w.Header().Set("Content-Type", "image/png")
						_, _ = w.Write(tinyPNG(t))
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"id":"remote","status":"queued","data":[{"id":"public-video"}]}`))
					}
				}))
				defer cdn.Close()
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls = append(calls, r.URL.Path)
					if mode == "header" {
						require.Equal(t, auth.Prefix+key, r.Header.Get(auth.Name))
					} else {
						require.Equal(t, auth.Prefix+key, r.URL.Query().Get(auth.Name))
					}
					if r.URL.Path != "/same-origin" {
						path := "/same-origin"
						if mode == "query" {
							path += "?" + r.URL.Query().Encode()
						}
						http.Redirect(w, r, path, http.StatusFound)
						return
					}
					// Even if an origin copies its authentication query parameter
					// into a Location, it cannot pass that key to a different origin.
					http.Redirect(w, r, cdn.URL+"?cdn_signature=cdn-signed-value&"+r.URL.Query().Encode(), http.StatusFound)
				}))
				defer origin.Close()
				template := templateFixture(t, "video-arrays")
				template.Auth = auth
				capability := Capability{Model: "public-video", UpstreamModel: "private-video", MediaType: "video", Template: &template}
				account := Account{BaseURL: origin.URL, Key: key}
				adapter := &Adapter{Client: SafeClient(true)}
				switch operation {
				case "submit":
					result, err := adapter.Submit(context.Background(), capability, templateSecurityRequest(t, capability), account)
					require.NoError(t, err)
					require.Equal(t, "remote", result.ID)
				case "discover":
					template.Discovery = &TemplateEndpoint{Method: http.MethodGet, Path: "/models"}
					template.ModelIDs = []string{"data[].id"}
					models, err := adapter.DiscoverTemplate(context.Background(), template, account)
					require.NoError(t, err)
					require.Equal(t, []string{"public-video"}, models)
				case "download":
					capability.Template.DownloadAuth = &auth
					blobs, err := NewBlobStore(t.TempDir(), strings.Repeat("s", 32))
					require.NoError(t, err)
					_, _, _, err = blobs.Download(context.Background(), adapter.Client, StoredTask{ID: NewTaskID(), Snapshot: Snapshot{Capability: capability}}, Artifact{Key: "image", Type: "image", Source: origin.URL + "/content"}, account, 1024)
					require.NoError(t, err)
				}
				require.Len(t, calls, 3)
				require.Equal(t, []string{"/same-origin", "cdn"}, calls[1:])
			})
		}
	}
}

func TestTemplateConfiguredQueriesReachSubmitPollAndDiscovery(t *testing.T) {
	const model = "private/model&version=2"
	const taskID = "task/a?query=escape"
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		require.Equal(t, "a b&c", r.URL.Query().Get("fixed"))
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models") {
			_, _ = w.Write([]byte(`{"data":[{"id":"public"}]}`))
			return
		}
		require.Equal(t, model, r.URL.Query().Get("model"))
		if r.Method == http.MethodGet {
			require.Equal(t, taskID, r.URL.Query().Get("task"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": taskID, "status": "queued"})
	}))
	defer upstream.Close()
	template := templateFixture(t, "video-arrays")
	template.Submit.Query = map[string]string{"fixed": "a b&c", "model": "{model}"}
	template.Query = &TemplateEndpoint{Method: http.MethodGet, Path: "/v1/videos/{task_id}", Query: map[string]string{"fixed": "a b&c", "model": "{model}", "task": "{task_id}"}}
	template.Discovery = &TemplateEndpoint{Method: http.MethodGet, Path: "/models", Query: map[string]string{"fixed": "a b&c"}}
	template.ModelIDs = []string{"data[].id"}
	capability := Capability{Model: "public", UpstreamModel: model, MediaType: "video", Template: &template}
	adapter := &Adapter{Client: SafeClient(true)}
	account := Account{BaseURL: upstream.URL + "/proxy/v1", Key: "fixture-key"}
	result, err := adapter.Submit(context.Background(), capability, templateSecurityRequest(t, capability), account)
	require.NoError(t, err)
	require.Equal(t, taskID, result.ID)
	_, err = adapter.Poll(context.Background(), StoredTask{UpstreamID: taskID, Snapshot: Snapshot{Capability: capability}}, account)
	require.NoError(t, err)
	models, err := adapter.DiscoverTemplate(context.Background(), template, account)
	require.NoError(t, err)
	require.Equal(t, []string{"public"}, models)
	require.Equal(t, []string{"/proxy/v1/videos", "/proxy/v1/videos/" + url.PathEscape(taskID), "/proxy/v1/models"}, paths)
	for _, endpoint := range []string{"https://other.invalid/replace", "//other.invalid/replace", "/jobs#fragment"} {
		_, err = originURL(account, endpoint)
		require.Error(t, err)
	}
}
