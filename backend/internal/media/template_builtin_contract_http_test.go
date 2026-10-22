package media

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These fixtures are complete, playable files, not image bytes labelled as video
// or audio. They keep the HTTP contract tests independent of an ffmpeg install.
// MP4: 16x16 red, H.264, two frames at 5 fps; MP3: 440 Hz, 22.05 kHz mono.
const builtinContractMP4 = "AAAAIGZ0eXBpc29tAAACAGlzb21pc28yYXZjMW1wNDEAAAMpbW9vdgAAAGxtdmhkAAAAAAAAAAAAAAAAAAAD6AAAAZAAAQAAAQAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAgAAAlN0cmFrAAAAXHRraGQAAAADAAAAAAAAAAAAAAABAAAAAAAAAZAAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAABAAAAAABAAAAAQAAAAAAAkZWR0cwAAABxlbHN0AAAAAAAAAAEAAAGQAAAAAAABAAAAAAHLbWRpYQAAACBtZGhkAAAAAAAAAAAAAAAAAAAoAAAAEABVxAAAAAAALWhkbHIAAAAAAAAAAHZpZGUAAAAAAAAAAAAAAABWaWRlb0hhbmRsZXIAAAABdm1pbmYAAAAUdm1oZAAAAAEAAAAAAAAAAAAAACRkaW5mAAAAHGRyZWYAAAAAAAAAAQAAAAx1cmwgAAAAAQAAATZzdGJsAAAAtnN0c2QAAAAAAAAAAQAAAKZhdmMxAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAABAAEABIAAAASAAAAAAAAAABFUxhdmM2MC4zMy4xMDAgbGlieDI2NAAAAAAAAAAAAAAAGP//AAAALGF2Y0MBQsAK/+EAFWdCwAraewEQAAADABAAAAMAoPEiagEABGjOD8gAAAAQcGFzcAAAAAEAAAABAAAAFGJ0cnQAAAAAAAAwNAAAMDQAAAAYc3R0cwAAAAAAAAABAAAAAgAACAAAAAAUc3RzcwAAAAAAAAABAAAAAQAAABxzdHNjAAAAAAAAAAEAAAABAAAAAgAAAAEAAAAcc3RzegAAAAAAAAAAAAAAAgAAAmAAAAAJAAAAFHN0Y28AAAAAAAAAAQAAA1kAAABidWR0YQAAAFptZXRhAAAAAAAAACFoZGxyAAAAAAAAAABtZGlyYXBwbAAAAAAAAAAAAAAAAC1pbHN0AAAAJal0b28AAAAdZGF0YQAAAAEAAAAATGF2ZjYwLjE3LjEwMAAAAAhmcmVlAAACcW1kYXQAAAJFBgX//0HcRem95tlIt5Ys2CDZI+7veDI2NCAtIGNvcmUgMTY0IC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyMyAtIGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTAgcmVmPTEgZGVibG9jaz0wOjA6MCBhbmFseXNlPTA6MCBtZT1kaWEgc3VibWU9MCBwc3k9MSBwc3lfcmQ9MS4wMDowLjAwIG1peGVkX3JlZj0wIG1lX3JhbmdlPTE2IGNocm9tYV9tZT0xIHRyZWxsaXM9MCA4eDhkY3Q9MCBjcW09MCBkZWFkem9uZT0yMSwxMSBmYXN0X3Bza2lwPTEgY2hyb21hX3FwX29mZnNldD0wIHRocmVhZHM9MSBsb29rYWhlYWRfdGhyZWFkcz0xIHNsaWNlZF90aHJlYWRzPTAgbnI9MCBkZWNpbWF0ZT0xIGludGVybGFjZWQ9MCBibHVyYXlfY29tcGF0PTAgY29uc3RyYWluZWRfaW50cmE9MCBiZnJhbWVzPTAgd2VpZ2h0cD0wIGtleWludD0yNTAga2V5aW50X21pbj01IHNjZW5lY3V0PTAgaW50cmFfcmVmcmVzaD0wIHJjPWNyZiBtYnRyZWU9MCBjcmY9MjMuMCBxY29tcD0wLjYwIHFwbWluPTAgcXBtYXg9NjkgcXBzdGVwPTQgaXBfcmF0aW89MS40MCBhcT0wAIAAAAATZYiEOhGKAAIY8cAAQPY4AAh5YAAAAAVBmiA+lA=="
const builtinContractMP3 = "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"

// Unlike the generic transport test, this matrix never rewrites a built-in
// endpoint, authentication mode, material mapping or response extraction rule.
// Explicit expected paths and payloads make a broken shipped preset observable.
func TestBuiltinTemplateNativeHTTPContracts(t *testing.T) {
	ctx := context.Background()
	png := pngFixture(t)
	wav := templateAudioWAV(t)
	mp4, err := base64.StdEncoding.DecodeString(builtinContractMP4)
	require.NoError(t, err)
	mp3, err := base64.StdEncoding.DecodeString(builtinContractMP3)
	require.NoError(t, err)
	imageURL := "data:image/png;base64," + base64String(png)
	videoURL := "data:video/mp4;base64," + base64String(mp4)
	audioURL := "data:audio/wav;base64," + base64String(wav)
	optional := map[string]any{"flag": false, "count": float64(0), "empty": []any{}, "null": nil}
	images := []any{imageURL, map[string]any{"url": imageURL, "role": "reference", "weight": float64(0)}, imageURL, imageURL}
	videos, audios := []any{videoURL}, []any{audioURL}
	first := map[string]any{"url": imageURL, "role": "first_frame", "weight": float64(0)}
	last := map[string]any{"url": imageURL, "role": "last_frame"}
	frameImages := []any{first, last}
	referenceImage := map[string]any{"url": imageURL, "role": "reference"}
	type contract struct {
		id, operation, mediaType, submit, query, encoding string
		refs, expected                                    map[string]any
		absent                                            []string
		output                                            []byte
		mime                                              string
		inline                                            bool
	}
	cases := []contract{
		{id: "video-arrays", operation: "video", mediaType: "video", submit: "/v1/videos", query: "/v1/videos/job/a", refs: map[string]any{"images": images, "videos": videos, "audios": audios}, expected: map[string]any{"images": images, "videos": videos, "audios": audios, "seconds": float64(5), "ratio": "16:9"}},
		{id: "v6k-video-reference-string", operation: "video", mediaType: "video", submit: "/v1/videos", query: "/v1/videos/job/a", refs: map[string]any{"images": []any{referenceImage}}, expected: map[string]any{"images": []any{imageURL}}},
		{id: "video-reference-arrays", operation: "video", mediaType: "video", submit: "/v1/videos", query: "/v1/videos/job/a", refs: map[string]any{"images": images, "videos": videos, "audios": audios}, expected: map[string]any{"reference_images": images, "reference_videos": videos, "reference_audios": audios, "duration": float64(5), "aspect_ratio": "16:9"}, absent: []string{"images", "videos", "audios", "seconds", "ratio"}},
		{id: "video-single-image", operation: "video", mediaType: "video", submit: "/v1/videos", query: "/v1/videos/job/a", refs: map[string]any{"images": []any{imageURL}}, expected: map[string]any{"image": map[string]any{"url": imageURL}}, absent: []string{"images"}},
		{id: "video-frame-roles", operation: "video", mediaType: "video", submit: "/v1/videos", query: "/v1/videos/job/a", refs: map[string]any{"images": frameImages}, expected: map[string]any{"images": frameImages}},
		{id: "video-frame-fields", operation: "video", mediaType: "video", submit: "/v1/videos", query: "/v1/videos/job/a", refs: map[string]any{"images": frameImages}, expected: map[string]any{"first_frame": first, "last_frame": last}, absent: []string{"images"}},
		{id: "media-content-blocks", operation: "video", mediaType: "video", submit: "/api/v3/contents/generations/tasks", query: "/api/v3/contents/generations/tasks/job/a", refs: map[string]any{"images": []any{imageURL}, "videos": videos, "audios": audios}, expected: map[string]any{"content": []any{map[string]any{"type": "text", "text": "contract prompt"}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}}, map[string]any{"type": "video_url", "video_url": map[string]any{"url": videoURL}}, map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": audioURL}}}, "duration": float64(5)}, absent: []string{"prompt", "images", "videos", "audios", "seconds"}},
		{id: "image-json", operation: "image", mediaType: "image", submit: "/v1/images/generations"},
		{id: "image-edit-json", operation: "image_edit", mediaType: "image", submit: "/v1/images/edits", refs: map[string]any{"images": images, "mask": imageURL}, expected: map[string]any{"images": images, "mask": map[string]any{"url": imageURL}}},
		{id: "image-edit-multipart", operation: "image_edit", mediaType: "image", submit: "/v1/images/edits", encoding: "multipart", refs: map[string]any{"images": []any{imageURL, imageURL}, "mask": imageURL}},
		{id: "gemini-parts", operation: "gemini_image", mediaType: "image", submit: "/v1beta/models/contract-upstream-model:generateContent", refs: map[string]any{"images": []any{imageURL}, "audios": audios}, expected: map[string]any{"generationConfig": map[string]any{"responseModalities": []any{"TEXT", "IMAGE"}}, "contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "contract prompt"}, map[string]any{"inlineData": map[string]any{"data": base64String(png), "mimeType": "image/png"}}, map[string]any{"inlineData": map[string]any{"data": base64String(wav), "mimeType": "audio/wav"}}}}}}, absent: []string{"model", "prompt", "images", "audios"}, inline: true},
		{id: "gemini-audio-parts", operation: "audio", mediaType: "audio", submit: "/v1beta/models/contract-upstream-model:generateContent", refs: map[string]any{"images": []any{imageURL}, "audios": audios}, expected: map[string]any{"generationConfig": map[string]any{"responseModalities": []any{"AUDIO"}}, "contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "contract prompt"}, map[string]any{"inlineData": map[string]any{"data": base64String(png), "mimeType": "image/png"}}, map[string]any{"inlineData": map[string]any{"data": base64String(wav), "mimeType": "audio/wav"}}}}}}, absent: []string{"model", "prompt", "images", "audios"}, output: wav, mime: "audio/wav", inline: true},
		{id: "music-chat-json", operation: "audio", mediaType: "audio", submit: "/v1/chat/completions", refs: map[string]any{"images": []any{imageURL}}, expected: map[string]any{"messages": []any{map[string]any{"role": "user", "content": "contract prompt"}}, "stream": false, "images": []any{imageURL}}, absent: []string{"prompt"}},
		{id: "audio-async", operation: "audio", mediaType: "audio", submit: "/v1/audio/generations", query: "/v1/audio/generations/job/a"},
		{id: "speech-binary", operation: "speech", mediaType: "audio", submit: "/v1/audio/speech", expected: map[string]any{"input": "汉😀é", "voice": "configured-voice", "response_format": "wav"}, output: wav, mime: "audio/wav", inline: true},
		{id: "image-json-multipart", operation: "image", mediaType: "image", submit: "/v1/images/generations"},
		{id: "image-json-multipart", operation: "image_edit", mediaType: "image", submit: "/v1/images/edits", encoding: "multipart", refs: map[string]any{"images": []any{imageURL, imageURL}, "mask": imageURL}},
	}
	covered := map[string]bool{"custom-blank": true}
	for _, tc := range cases {
		covered[tc.id] = true
		t.Run(tc.id+"/"+tc.operation, func(t *testing.T) {
			template := templateFixture(t, tc.id)
			require.NoError(t, ValidateTemplate(template, true))
			original, err := json.Marshal(template)
			require.NoError(t, err)
			output, mime := tc.output, tc.mime
			if output == nil {
				switch tc.mediaType {
				case "video":
					output, mime = mp4, "video/mp4"
				case "audio":
					output, mime = mp3, "audio/mpeg"
				default:
					output, mime = png, "image/png"
				}
			}
			calls := []string{}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method+" "+r.URL.Path)
				if strings.HasPrefix(tc.id, "gemini-") {
					assert.Equal(t, "fixture-credential", r.Header.Get("x-goog-api-key"))
					assert.Empty(t, r.Header.Get("Authorization"))
				} else {
					assert.Equal(t, "Bearer fixture-credential", r.Header.Get("Authorization"))
				}
				writeJSON := func(body any) {
					w.Header().Set("Content-Type", "application/json")
					assert.NoError(t, json.NewEncoder(w).Encode(body))
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
					writeJSON(map[string]any{"data": []any{map[string]any{"id": "contract-upstream-model"}}})
				case r.Method == http.MethodPost && r.URL.Path == tc.submit:
					assert.Equal(t, "fixture-idempotency", r.Header.Get("Idempotency-Key"))
					if tc.encoding == "multipart" {
						assert.True(t, strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;"))
						if !assert.NoError(t, r.ParseMultipartForm(1<<20)) {
							http.Error(w, "bad multipart", http.StatusBadRequest)
							return
						}
						defer func() { _ = r.MultipartForm.RemoveAll() }()
						assert.Equal(t, "contract-upstream-model", r.FormValue("model"))
						assert.Equal(t, "contract prompt", r.FormValue("prompt"))
						assert.JSONEq(t, `{"flag":false,"count":0,"empty":[],"null":null}`, r.FormValue("optional"))
						assert.Len(t, r.MultipartForm.File["image"], 2)
						assert.Len(t, r.MultipartForm.File["mask"], 1)
						for _, field := range []string{"image", "mask"} {
							for _, file := range r.MultipartForm.File[field] {
								assert.Equal(t, "image/png", file.Header.Get("Content-Type"))
								f, e := file.Open()
								if assert.NoError(t, e) {
									data, e := io.ReadAll(f)
									_ = f.Close()
									assert.NoError(t, e)
									assert.Equal(t, png, data)
								}
							}
						}
					} else {
						assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
						var body map[string]any
						if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
							http.Error(w, "bad JSON", http.StatusBadRequest)
							return
						}
						if !strings.HasPrefix(tc.id, "gemini-") {
							assert.Equal(t, "contract-upstream-model", body["model"])
						}
						assert.Equal(t, optional, body["optional"])
						for field, value := range tc.expected {
							assert.Equal(t, value, body[field], "wire field %s", field)
						}
						for _, field := range tc.absent {
							assert.NotContains(t, body, field)
						}
					}
					if tc.id == "speech-binary" {
						w.Header().Set("Content-Type", mime)
						_, _ = w.Write(output)
					} else if tc.query != "" {
						writeJSON(map[string]any{"id": "job/a", "status": "queued", "progress": 0})
					} else if strings.HasPrefix(tc.id, "gemini-") {
						writeJSON(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"inlineData": map[string]any{"mimeType": mime, "data": base64String(output)}}}}}}})
					} else if tc.id == "music-chat-json" {
						writeJSON(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"audio": map[string]any{"url": "/generated/contract"}}}}})
					} else {
						writeJSON(map[string]any{"data": []any{map[string]any{"url": "/generated/contract"}}})
					}
				case r.Method == http.MethodGet && tc.query != "" && r.URL.Path == tc.query:
					// The slash belongs to the task ID and must be escaped on the wire.
					assert.Contains(t, r.RequestURI, "job%2Fa")
					switch tc.id {
					case "media-content-blocks":
						writeJSON(map[string]any{"id": "job/a", "status": "succeeded", "progress": 100, "output": map[string]any{"video_url": "/generated/contract"}})
					case "audio-async":
						writeJSON(map[string]any{"id": "job/a", "status": "completed", "progress": 100, "data": map[string]any{"url": "/generated/contract"}})
					default:
						// No URL: exercise the shipped /content endpoint fallback.
						writeJSON(map[string]any{"id": "job/a", "status": "completed", "progress": 100})
					}
				case r.Method == http.MethodGet && (r.URL.Path == "/generated/contract" || r.URL.Path == "/v1/videos/job/a/content"):
					w.Header().Set("Content-Type", mime)
					_, _ = w.Write(output)
				default:
					t.Errorf("unexpected built-in HTTP request: %s %s", r.Method, r.RequestURI)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			account := Account{BaseURL: server.URL, Key: "fixture-credential", IdempotencyKey: "fixture-idempotency"}
			adapter := &Adapter{Client: server.Client()}
			models, err := adapter.DiscoverTemplate(ctx, template, account)
			require.NoError(t, err)
			require.Equal(t, []string{"contract-upstream-model"}, models)
			capability := CapabilityDefaults(Capability{Model: "public", UpstreamModel: "contract-upstream-model", MediaType: tc.mediaType, Template: &template})
			input := map[string]any{"model": "public", "prompt": "contract prompt", "optional": optional}
			if tc.operation == "video" {
				input["seconds"], input["ratio"] = 5, "16:9"
			}
			if tc.operation == "speech" {
				input["input"], input["voice"], input["response_format"] = "汉😀é", "configured-voice", "wav"
			}
			for field, value := range tc.refs {
				input[field] = value
			}
			request, err := NormalizeUploads(capability, tc.operation, input, nil)
			require.NoError(t, err)
			prepared, err := BuildTemplateRequest(ctx, server.Client(), capability, request)
			require.NoError(t, err)
			request.Wire = &prepared
			result, err := adapter.Submit(ctx, capability, request, account)
			require.NoError(t, err)
			expectedCalls := []string{"GET /v1/models", "POST " + tc.submit}
			if tc.query != "" {
				require.Equal(t, "job/a", result.ID)
				require.Equal(t, "in_progress", result.Status)
				require.True(t, result.Accepted)
				require.Empty(t, result.Artifacts)
				result, err = adapter.Poll(ctx, StoredTask{UpstreamID: result.ID, Snapshot: Snapshot{Capability: capability}}, account)
				require.NoError(t, err)
				require.Equal(t, []string{"100"}, result.Quantities["upstream_progress"])
				expectedCalls = append(expectedCalls, "GET "+tc.query)
			}
			require.Equal(t, "completed", result.Status)
			require.True(t, result.Accepted)
			require.Len(t, result.Artifacts, 1)
			artifact := result.Artifacts[0]
			require.Equal(t, tc.mediaType, artifact.Type)
			blobs, err := NewBlobStore(t.TempDir(), "fixture-encryption-key-stable-at-least-32-bytes")
			require.NoError(t, err)
			var path, hash string
			if tc.inline {
				data, actualMIME, err := decodeArtifact(artifact.Source)
				require.NoError(t, err)
				require.Equal(t, output, data)
				require.Equal(t, mime, actualMIME)
				path, err = blobs.Path(NewTaskID(), artifact.Key)
				require.NoError(t, err)
				require.NoError(t, atomicWrite(path, data))
				if tc.id == "speech-binary" {
					var saved map[string]any
					require.NoError(t, json.Unmarshal(result.Response, &saved))
					require.Equal(t, result.Artifacts, ParseTemplateResponse(saved, capability).Artifacts)
				}
			} else {
				artifact, path, hash, err = blobs.Download(ctx, server.Client(), StoredTask{ID: NewTaskID(), Snapshot: Snapshot{Capability: capability}}, artifact, account, 1<<20)
				require.NoError(t, err)
				require.True(t, artifact.Available)
				sum := sha256.Sum256(output)
				require.Equal(t, hex.EncodeToString(sum[:]), hash)
				downloadPath := "/generated/contract"
				if strings.HasPrefix(tc.id, "video-") || tc.id == "v6k-video-reference-string" {
					downloadPath = "/v1/videos/job/a/content"
				}
				expectedCalls = append(expectedCalls, "GET "+downloadPath)
			}
			stored, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, output, stored)
			require.Equal(t, mime, artifact.MIME)
			require.Equal(t, expectedCalls, calls)
			unchanged, err := json.Marshal(template)
			require.NoError(t, err)
			require.Equal(t, original, unchanged, "contract test must use the unmodified built-in")
		})
	}
	t.Run("custom-blank-remains-unpublishable", func(t *testing.T) {
		blank := templateFixture(t, "custom-blank")
		require.Equal(t, "draft", blank.Status)
		require.NoError(t, ValidateTemplate(blank, false))
		require.Error(t, ValidateTemplate(blank, true))
	})
	for _, template := range BuiltinTemplates() {
		require.True(t, covered[template.ID], "new built-in requires a native HTTP contract case: %s", template.ID)
	}

	// Byte-level HTTP tests always run. When ffprobe is available, additionally
	// verify actual codecs/duration without making CI contract coverage optional.
	t.Run("playable-fixtures", func(t *testing.T) {
		if _, err := exec.LookPath("ffprobe"); err != nil {
			t.Skip("HTTP matrix passed; optional fixture codec verification requires ffprobe")
		}
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			t.Skip("HTTP matrix passed; optional fixture full decoding requires ffmpeg")
		}
		for name, data := range map[string][]byte{"video.mp4": mp4, "audio.mp3": mp3, "audio.wav": wav} {
			path := filepath.Join(t.TempDir(), name)
			require.NoError(t, os.WriteFile(path, data, 0600))
			out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "stream=codec_name,codec_type:format=duration", "-of", "json", path).CombinedOutput()
			require.NoError(t, err, "%s: %s", name, out)
			require.Contains(t, string(out), "codec_name")
			require.Contains(t, string(out), "duration")
			decoded, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput()
			require.NoError(t, err, "%s: %s", name, decoded)
			require.Empty(t, strings.TrimSpace(string(decoded)))
		}
	})
}
