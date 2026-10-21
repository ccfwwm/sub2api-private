package media

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func videoFormatCapability(format string) Capability {
	c := Capability{ID: "format-test", Version: "1", Model: "test-video", UpstreamModel: "upstream-video", Adapter: "v6k_video_gateway", RequestFormat: format, MediaType: "video", Operations: []string{"video"}, Duration: Duration{Mode: "enum", Values: []int{5, 30}}, AspectRatios: []string{"9:16"}, Materials: map[string]Material{}}
	for _, k := range []string{"image", "video", "audio"} {
		c.Materials[k] = Material{Formats: []string{"data_url", "data_url_in_url_object", "public_url_object", "public_url_string"}, MaxCount: pointer(3), MaxFileBytes: pointer(int64(1 << 20)), MaxDuration: pointer("30")}
	}
	return c
}

func TestPrepareVideoRequestMeasuresRealMaterialsForQuotes(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is required for real media fixtures")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is required for server-side measurements")
	}
	cap := videoFormatCapability("yunfei_multimodal")
	blobs, err := NewBlobStore(t.TempDir(), strings.Repeat("s", 32))
	require.NoError(t, err)
	engine := &Engine{Blobs: blobs, Adapter: &Adapter{Client: SafeClient(true)}}
	p := map[string]any{"prompt": "animate", "duration": 5, "reference_images": []any{"data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG(t))}}
	for _, kind := range []string{"video", "audio"} {
		ext, mime, args := ".wav", "audio/wav", []string{"-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono", "-t", "1"}
		if kind == "video" {
			ext, mime, args = ".mp4", "video/mp4", []string{"-f", "lavfi", "-i", "color=c=blue:s=32x32:r=10", "-t", "1", "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p"}
		}
		file := filepath.Join(t.TempDir(), "reference"+ext)
		cmd := exec.Command("ffmpeg", append([]string{"-v", "error", "-nostdin"}, append(args, file)...)...)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		raw, err := os.ReadFile(file)
		require.NoError(t, err)
		p["reference_"+kind+"s"] = []any{map[string]any{"url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw), "duration_seconds": 999}}
	}
	for i := 0; i < 2; i++ {
		req, err := engine.PrepareRequest(context.Background(), cap, "video", p, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"1"}, req.Quantities["accepted_input_video_duration"])
		require.Equal(t, []string{"1"}, req.Quantities["accepted_input_audio_duration"])
		require.Equal(t, []string{"1"}, req.Quantities["accepted_input_images"])
	}
	m := cap.Materials["video"]
	m.MaxDuration = pointer("0.5")
	cap.Materials["video"] = m
	_, err = engine.PrepareRequest(context.Background(), cap, "video", p, nil)
	require.NoError(t, err)
}

func TestPrepareVideoRequestRejectsHTMLInsteadOfAnImage(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>login required</html>")) }))
	defer srv.Close()
	blobs, err := NewBlobStore(t.TempDir(), strings.Repeat("s", 32))
	require.NoError(t, err)
	engine := &Engine{Blobs: blobs, Adapter: &Adapter{Client: srv.Client()}}
	p := map[string]any{"prompt": "animate", "duration": 5, "reference_images": []any{srv.URL + "/reference.png"}}
	_, err = engine.PrepareRequest(context.Background(), videoFormatCapability("yunfei_multimodal"), "video", p, nil)
	// The public URL guard must reject loopback before fetching it.
	require.ErrorContains(t, err, "invalid_reference")
	p["reference_images"] = []any{"data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("<html>login required</html>"))}
	_, err = engine.PrepareRequest(context.Background(), videoFormatCapability("yunfei_multimodal"), "video", p, nil)
	require.ErrorContains(t, err, "invalid_material")
}

func TestExplicitVideoProtocols(t *testing.T) {
	for _, format := range []string{"yunfei_multimodal", "v6k_url"} {
		t.Run(format, func(t *testing.T) {
			cap := videoFormatCapability(format)
			p := map[string]any{"model": "test-video", "prompt": "animate", "duration": 5, "aspect_ratio": "9:16", "images": []any{map[string]any{"url": "https://media.example.com/ref.png"}}}
			if format == "yunfei_multimodal" {
				delete(p, "images")
				p["reference_images"] = []any{"data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG(t))}
				p["reference_videos"] = []any{"https://media.example.com/ref.mp4"}
				p["reference_audios"] = []any{"https://media.example.com/ref.mp3"}
			}
			req, err := Normalize(cap, "video", p)
			require.NoError(t, err)
			require.Equal(t, "5", req.Duration)
			require.Equal(t, 1, req.InputImages)
			count := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count++
				require.Equal(t, "/v1/videos", r.URL.Path)
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, "upstream-video", body["model"])
				require.Nil(t, body["public_profile_id"])
				require.Nil(t, body["request_format"])
				if format == "yunfei_multimodal" {
					require.Equal(t, float64(5), body["duration"])
					require.Equal(t, "9:16", body["aspect_ratio"])
					require.Nil(t, body["seconds"])
					require.Nil(t, body["images"])
					require.Len(t, body["reference_videos"], 1)
					require.Len(t, body["reference_audios"], 1)
				} else {
					require.Equal(t, float64(5), body["seconds"])
					require.Equal(t, "9:16", body["ratio"])
					require.Equal(t, []any{map[string]any{"url": "https://media.example.com/ref.png"}}, body["images"])
					require.Nil(t, body["duration"])
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "upstream-id", "status": "queued"})
			}))
			defer server.Close()
			adapter := Adapter{Client: SafeClient(true)}
			res, err := adapter.Submit(context.Background(), cap, req, Account{BaseURL: server.URL, Key: "fixture"})
			require.NoError(t, err)
			require.True(t, res.Accepted)
			require.Equal(t, 1, count)
		})
	}
}

func TestExplicitFrameRolesAndEncodedReferences(t *testing.T) {
	cap := videoFormatCapability("yunfei_multimodal")
	p := map[string]any{"prompt": "transition", "duration": 5, "reference_images": []any{map[string]any{"url": "https://media.example.com/first.png", "role": "first_frame"}, map[string]any{"url": "https://media.example.com/last.png", "role": "last_frame"}}}
	req, err := Normalize(cap, "video", p)
	require.NoError(t, err)
	require.Equal(t, "first_frame", req.MaterialRefs["image"][0].(map[string]any)["role"])
	p["reference_images"] = append(p["reference_images"].([]any), "https://media.example.com/ordinary.png")
	_, err = Normalize(cap, "video", p)
	require.ErrorContains(t, err, "frame_reference_conflict")
	cap.RequestFormat = "v6k_url"
	p["reference_images"] = []any{"data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG(t))}
	_, err = Normalize(cap, "video", p)
	require.NoError(t, err)
	delete(p, "reference_images")
	p["videos"] = []any{"https://media.example.com/video.mp4"}
	_, err = Normalize(cap, "video", p)
	require.NoError(t, err)
	m := cap.Materials["video"]
	m.ReferenceField = "videos"
	cap.Materials["video"] = m
	_, err = Normalize(cap, "video", p)
	require.NoError(t, err)
}

func TestYunfeiOutputArrayIsDeduplicatedAndNormalized(t *testing.T) {
	p := map[string]any{"id": "provider-id", "status": "completed", "url": "https://media.example.com/result.mp4", "output": []any{map[string]any{"url": "https://media.example.com/result.mp4"}, map[string]any{"url": "https://media.example.com/second.mp4"}}}
	r := parseResult(p, videoFormatCapability("yunfei_multimodal"))
	require.Equal(t, "completed", r.Status)
	require.Len(t, r.Artifacts, 2)
}

func TestExplicitFormatsNeverDiscardReferenceFields(t *testing.T) {
	cap := videoFormatCapability("yunfei_multimodal")
	for _, field := range []string{"image", "image_url", "image_urls", "input_reference"} {
		req, err := Normalize(cap, "video", map[string]any{"prompt": "test", "duration": 5, field: "https://example.com/a.png"})
		require.NoError(t, err)
		require.Len(t, req.MaterialRefs["image"], 1)
	}
	for _, fields := range []map[string]any{{"images": []any{}, "reference_images": []any{"https://example.com/a.png"}}, {"reference_images": []any{}, "input_reference": "https://example.com/a.png"}} {
		fields["prompt"] = "test"
		fields["duration"] = 5
		_, err := Normalize(cap, "video", fields)
		require.ErrorContains(t, err, "alias_conflict")
	}
	for _, u := range []string{"https://127.0.0.1/a", "https://10.0.0.1/a", "https://[::1]/a", "https://example.local/a", "https://example.com:8443/a", "https://[bad/a"} {
		_, err := referenceFormat(u, "string")
		require.Error(t, err)
	}
}

func TestOnlyExplicitSubmissionFormatRejectionRequestsURL(t *testing.T) {
	for _, status := range []int{400, 401, 403, 415, 422, 429, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"base64_not_supported","message":"Bearer fixture-provider-key https://example.com/private?token=hidden"}}`))
		}))
		a := Adapter{Client: SafeClient(true)}
		result, err := a.Submit(context.Background(), videoFormatCapability("yunfei_multimodal"), Request{Operation: "video", Parameters: map[string]any{"prompt": "x"}}, Account{BaseURL: srv.URL, Key: "fixture-provider-key"})
		require.NoError(t, err)
		require.NotContains(t, result.Diagnostic, "fixture-provider-key")
		require.NotContains(t, result.Diagnostic, "token=hidden")
		require.Equal(t, status == 400 || status == 415 || status == 422, result.Diagnostic == "reference_url_required")
		require.False(t, result.Accepted)
		srv.Close()
	}
}
