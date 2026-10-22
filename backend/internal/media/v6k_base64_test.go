package media

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestV6KBase64ReferencesReachUpstreamWithoutCatalogPermission(t *testing.T) {
	image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG(t))
	video := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString([]byte("fixture-video"))
	for _, shape := range []string{"string", "url", "b64_json", "bare_base64"} {
		t.Run(shape, func(t *testing.T) {
			wrap := func(source string) any {
				switch shape {
				case "url":
					return map[string]any{"url": source}
				case "b64_json":
					return map[string]any{"b64_json": source}
				case "bare_base64":
					return map[string]any{"b64_json": strings.SplitN(source, ",", 2)[1]}
				default:
					return source
				}
			}
			cap := videoFormatCapability("v6k_url")
			cap.Materials = map[string]Material{"image": {Formats: []string{"public_url_object"}, MaxCount: pointer(1)}}
			images := []any{wrap(image), wrap(image), wrap(image), wrap(image)}
			videos := []any{wrap(video)}
			request, err := Normalize(cap, "video", map[string]any{"model": cap.Model, "prompt": "retain five inputs", "seconds": 5, "images": images, "videos": videos})
			require.NoError(t, err)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, float64(5), body["seconds"])
				require.Equal(t, cap.UpstreamModel, body["model"])
				require.Equal(t, []any{image, image, image, image}, body["images"])
				wireVideos, ok := body["videos"].([]any)
				require.True(t, ok)
				require.Len(t, wireVideos, 1)
				actual, ok := wireVideos[0].(string)
				require.True(t, ok)
				if shape == "bare_base64" {
					// Fixture bytes are not a real MP4; production probes real MIME
					// before reservation. This unit covers lossless wire conversion.
					require.True(t, strings.HasPrefix(actual, "data:"))
					require.Equal(t, strings.SplitN(video, ",", 2)[1], strings.SplitN(actual, ",", 2)[1])
				} else {
					require.Equal(t, video, actual)
				}
				_, _ = w.Write([]byte(`{"id":"accepted-once","status":"queued"}`))
			}))
			defer server.Close()
			var receipt Submission
			result, err := (&Adapter{Client: SafeClient(true)}).Submit(context.Background(), cap, request, Account{BaseURL: server.URL, Key: "fixture", ObserveSubmission: func(s Submission) error { receipt = s; return nil }})
			require.NoError(t, err)
			require.True(t, result.Accepted)
			require.Equal(t, 1, calls)
			require.Equal(t, 4, receipt.References["image"])
			require.Equal(t, 1, receipt.References["video"])
			raw, err := json.Marshal(receipt)
			require.NoError(t, err)
			require.Contains(t, string(raw), image)
			// Non-MP4 fixture bytes are sniffed as text/plain; the receipt must
			// preserve their payload without inventing a media MIME type.
			require.Contains(t, string(raw), strings.SplitN(video, ",", 2)[1])
		})
	}
}

func TestV6KBase64ObjectRetainsAdditionalAttributes(t *testing.T) {
	image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG(t))
	cap := videoFormatCapability("v6k_url")
	request, err := Normalize(cap, "video", map[string]any{"model": cap.Model, "prompt": "retain weight", "seconds": 5, "images": []any{map[string]any{"b64_json": image, "weight": 0.7}}})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, []any{map[string]any{"url": image, "weight": 0.7}}, body["images"])
		_, _ = w.Write([]byte(`{"id":"accepted","status":"queued"}`))
	}))
	defer server.Close()
	_, err = (&Adapter{Client: SafeClient(true)}).Submit(context.Background(), cap, request, Account{BaseURL: server.URL, Key: "fixture"})
	require.NoError(t, err)
}
