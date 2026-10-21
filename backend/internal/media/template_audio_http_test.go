package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A complete PCM WAV fixture, not unrelated bytes with an audio Content-Type.
func templateAudioWAV(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	pcm := make([]byte, 1600)
	_, _ = out.WriteString("RIFF")
	require.NoError(t, binary.Write(&out, binary.LittleEndian, uint32(36+len(pcm))))
	_, _ = out.WriteString("WAVEfmt ")
	for _, field := range []any{uint32(16), uint16(1), uint16(1), uint32(8000), uint32(16000), uint16(2), uint16(16)} {
		require.NoError(t, binary.Write(&out, binary.LittleEndian, field))
	}
	_, _ = out.WriteString("data")
	require.NoError(t, binary.Write(&out, binary.LittleEndian, uint32(len(pcm))))
	_, _ = out.Write(pcm)
	return out.Bytes()
}

func TestGeminiAudioTemplateNativeHTTPAndPlayableArtifact(t *testing.T) {
	template := templateFixture(t, "gemini-audio-parts")
	require.NoError(t, ValidateTemplate(template, true))
	require.Equal(t, []string{"audio"}, template.MediaTypes)
	require.Equal(t, []string{"audio"}, template.Operations)
	wav := templateAudioWAV(t)
	for _, variant := range []struct {
		name, mime string
		data       []byte
		voice      bool
	}{
		{"default_audio", "audio/wav", wav, false},
		{"preserve_voice", "audio/wav", wav, true},
		{"native_pcm", "audio/L16;codec=pcm;rate=8000;channels=1", wav[44:], true},
	} {
		t.Run(variant.name, func(t *testing.T) {
			posts := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/v1beta/models/arbitrary-upstream:generateContent", r.URL.Path)
				require.Equal(t, "fixture-credential", r.Header.Get("x-goog-api-key"))
				posts++
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.NotContains(t, body, "model")
				require.NotContains(t, body, "prompt")
				config := body["generationConfig"].(map[string]any)
				require.Equal(t, []any{"AUDIO"}, config["responseModalities"])
				if variant.voice {
					require.Equal(t, map[string]any{"voice": "administrator-choice"}, config["speechConfig"])
				}
				contents := body["contents"].([]any)
				parts := contents[0].(map[string]any)["parts"].([]any)
				require.Equal(t, "朗读这句话", parts[0].(map[string]any)["text"])
				w.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{
						map[string]any{"inlineData": map[string]any{"mimeType": variant.mime, "data": base64String(variant.data)}},
					}}}},
				}))
			}))
			defer server.Close()
			capability := Capability{Model: "public", UpstreamModel: "arbitrary-upstream", MediaType: "audio", Template: &template}
			input := map[string]any{"model": "public", "prompt": "朗读这句话"}
			if variant.voice {
				input["generationConfig"] = map[string]any{"speechConfig": map[string]any{"voice": "administrator-choice"}}
			}
			request, err := Normalize(capability, "audio", input)
			require.NoError(t, err)
			prepared, err := BuildTemplateRequest(context.Background(), server.Client(), capability, request)
			require.NoError(t, err)
			request.Wire = &prepared
			result, err := (&Adapter{Client: server.Client()}).Submit(context.Background(), capability, request, Account{BaseURL: server.URL, Key: "fixture-credential"})
			require.NoError(t, err)
			require.Equal(t, 1, posts)
			require.Equal(t, "completed", result.Status)
			require.True(t, result.Accepted)
			require.Len(t, result.Artifacts, 1)
			artifact := result.Artifacts[0]
			require.Equal(t, "audio", artifact.Type)
			require.Equal(t, variant.mime, artifact.MIME)
			decoded, mime, err := decodeArtifact(artifact.Source)
			require.NoError(t, err)
			require.Equal(t, variant.mime, mime)
			require.Equal(t, variant.data, decoded)
			path := filepath.Join(t.TempDir(), "speech.wav")
			require.NoError(t, os.WriteFile(path, decoded, 0600))
			artifact, hash, summary, err := normalizeOutputArtifact(capability, artifact, path, "")
			require.NoError(t, err)
			if variant.name == "native_pcm" {
				require.Equal(t, variant.mime, summary.SourceMIME)
				require.Equal(t, "audio/wav", summary.OutputMIME)
				require.Equal(t, 8000, summary.SampleRate)
				require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wav)), hash)
			} else {
				require.Nil(t, summary)
			}
			stored, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, wav, stored)
			t.Run("artifact_validation", func(t *testing.T) {
				if _, err := exec.LookPath("ffprobe"); err != nil {
					t.Skip("ffprobe is required to validate decoded audio")
				}
				require.NoError(t, validateArtifact(context.Background(), path, &artifact))
				require.NotNil(t, artifact.Duration)
				require.Equal(t, "0.1", *artifact.Duration)
				if _, err := exec.LookPath("ffmpeg"); err != nil {
					t.Skip("ffmpeg is required for full audio decode")
				}
				output, err := exec.Command("ffmpeg", "-v", "error", "-nostdin", "-i", path, "-f", "null", "-").CombinedOutput()
				require.NoError(t, err, string(output))
			})
		})
	}
}
