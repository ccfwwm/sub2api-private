//go:build integration

package media_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/stretchr/testify/require"
)

type videoReferenceTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t videoReferenceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "reference.example.com" {
		r = r.Clone(r.Context())
		u := *r.URL
		u.Scheme = t.target.Scheme
		u.Host = t.target.Host
		r.URL = &u
	}
	return t.base.RoundTrip(r)
}

// Exercises both new wire formats through durable reservation, worker restart,
// polling, artifact download/probe and settlement. No supplier calls are made.
func TestVideoFormatsDurableEndToEnd(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		_, err := exec.LookPath(tool)
		require.NoError(t, err, "install media test prerequisites")
	}
	file := filepath.Join(t.TempDir(), "result.mp4")
	output, err := exec.Command("ffmpeg", "-v", "error", "-nostdin", "-f", "lavfi", "-i", "color=c=blue:s=32x32:r=10", "-t", "1", "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", file).CombinedOutput()
	require.NoError(t, err, string(output))
	video, err := os.ReadFile(file)
	require.NoError(t, err)
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 32, 32))))
	for _, format := range []string{"yunfei_multimodal", "v6k_url"} {
		t.Run(format, func(t *testing.T) {
			admin := testDB(t)
			database := "video_formats_" + strings.ReplaceAll(media.NewTaskID(), "-", "_")
			_, err := admin.Exec("CREATE DATABASE " + database)
			require.NoError(t, err)
			dsn, err := url.Parse(os.Getenv("MEDIA_TEST_DATABASE_URL"))
			require.NoError(t, err)
			dsn.Path = "/" + database
			db, err := sql.Open("postgres", dsn.String())
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = db.Close()
				_, err := admin.Exec("DROP DATABASE " + database + " WITH (FORCE)")
				require.NoError(t, err)
			})
			require.NoError(t, repository.ApplyMigrations(context.Background(), db))
			snap := fixture(t, db, "10")
			ctx := context.Background()
			cap := snap.Capability
			cap.RequestFormat = format
			cap.Adapter = "v6k_video_gateway"
			cap.Version = "wire-v1"
			cap.Duration = media.Duration{Mode: "enum", Values: []int{5}}
			cap.Enabled = true
			cap.Materials = map[string]media.Material{"image": {Formats: []string{"data_url", "public_url_string", "public_url_object"}, MaxCount: ptr(1), MaxFileBytes: ptr(int64(1 << 20)), MaxDuration: ptr("30")}}
			cap.Delivery.PollSeconds = 0
			cap.Limits.ObservationSeconds = ptr(600)
			var submits, polls, downloads atomic.Int32
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/reference.png":
					w.Header().Set("Content-Type", "image/png")
					_, _ = w.Write(pngBytes.Bytes())
				case r.Method == "POST" && r.URL.Path == "/v1/videos":
					submits.Add(1)
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Nil(t, body["request_format"])
					if format == "yunfei_multimodal" {
						require.Equal(t, float64(5), body["duration"])
						require.Len(t, body["reference_images"], 1)
						require.Nil(t, body["seconds"])
					} else {
						require.Equal(t, float64(5), body["seconds"])
						require.Equal(t, []any{map[string]any{"url": "https://reference.example.com/reference.png"}}, body["images"])
						require.Nil(t, body["duration"])
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"supplier-task","status":"queued"}`)
				case r.URL.Path == "/v1/videos/supplier-task":
					polls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					result := map[string]any{"id": "supplier-task", "status": "completed"}
					if format == "v6k_url" {
						result["url"] = "/result.mp4"
					} else {
						result["output"] = []any{map[string]any{"url": "/result.mp4"}}
					}
					require.NoError(t, json.NewEncoder(w).Encode(result))
				case r.URL.Path == "/result.mp4":
					downloads.Add(1)
					require.Equal(t, "Bearer isolated-fixture", r.Header.Get("Authorization"))
					w.Header().Set("Content-Type", "video/mp4")
					_, _ = w.Write(video)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := media.SafeClient(true)
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			client.Transport = videoReferenceTransport{target, client.Transport}
			blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("s", 32))
			require.NoError(t, err)
			card := snap.Sale
			card.ID = "quote"
			card.Version = "1"
			card.Status = "enabled"
			card.Purpose = "sale"
			card.MediaType = "video"
			card.Operation = "video"
			card.Currency = "USD"
			card.Rules = []media.Rule{snap.SaleRule}
			card.ModelMatch.Kind = "exact"
			card.ModelMatch.Value = cap.Model
			card.Scope.Kind = "group"
			card.Scope.Reference = fmt.Sprint(snap.Identity.GroupID)
			card.MultiplierPolicy = "effective_media_then_time"
			card.SuccessPolicy = "all_outputs"
			makeEngine := func() *media.Engine {
				return &media.Engine{Store: &media.Store{DB: db}, Blobs: blobs, Adapter: &media.Adapter{Client: client}, Config: func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
					return []media.Capability{cap}, []media.PriceCard{card}, nil
				}, Lookup: func(id, group int64) (media.Account, error) {
					return media.Account{ID: id, Allowed: true, Key: "isolated-fixture", BaseURL: server.URL, CostMultiplier: "1"}, nil
				}}
			}
			engine := makeEngine()
			p := map[string]any{"model": cap.Model, "prompt": "animate", "duration": 5}
			if format == "yunfei_multimodal" {
				p["reference_images"] = []any{"data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes.Bytes())}
			} else {
				p["images"] = []any{map[string]any{"url": "https://reference.example.com/reference.png"}}
			}
			req, err := engine.PrepareRequest(ctx, cap, "video", p, nil)
			require.NoError(t, err)
			_, _, quote, err := engine.Snapshot(ctx, cap, req, snap.Identity)
			require.NoError(t, err)
			require.Equal(t, "1.00000000", quote.Total)
			task, err := engine.Create(ctx, cap, "video", p, nil, snap.Identity, "same-request")
			require.NoError(t, err)
			require.Equal(t, "in_progress", task.State)
			require.NotEmpty(t, task.UpstreamID)
			require.EqualValues(t, 1, submits.Load(), "Create submits upstream before returning")
			for i := 0; i < 1; i++ {
				engine = makeEngine()
				claimed, err := engine.Store.Claim(ctx)
				require.NoError(t, err)
				require.NotNil(t, claimed)
				require.Equal(t, task.ID, claimed.ID)
				require.NoError(t, engine.Process(ctx, *claimed))
			}
			done, err := engine.Store.GetTask(ctx, task.ID)
			require.NoError(t, err)
			require.Equal(t, "completed", done.State)
			require.Equal(t, "settled", done.Settlement)
			artifacts, err := engine.Store.Artifacts(ctx, task.ID)
			require.NoError(t, err)
			require.Len(t, artifacts, 1)
			require.True(t, artifacts[0].Available)
			require.NotNil(t, artifacts[0].Duration)
			require.Equal(t, "1", *artifacts[0].Duration)
			again, err := makeEngine().Create(ctx, cap, "video", p, nil, snap.Identity, "same-request")
			require.NoError(t, err)
			require.Equal(t, task.ID, again.ID)
			require.EqualValues(t, 1, submits.Load())
			require.EqualValues(t, 1, polls.Load())
			require.EqualValues(t, 1, downloads.Load())
			balance, held := moneyState(t, db, snap.Identity.UserID)
			require.Equal(t, "9.00000000", balance)
			require.Equal(t, "0.00000000", held)
		})
	}
}
