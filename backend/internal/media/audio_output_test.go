package media

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func pcmFixtureConfig() *PCMToWAVConfig {
	return &PCMToWAVConfig{MIMETypes: []string{"audio/l16"}, SampleFormat: "s16", Endianness: "little", SampleRate: 24000, Channels: 1, RateParameter: "rate", ChannelsParameter: "channels"}
}

func TestPCMConversionExplicitConfigurationAndEndian(t *testing.T) {
	for _, tc := range []struct {
		name, format, endian string
		raw, want            []byte
	}{
		{"little_s16", "s16", "little", []byte{0x34, 0x12, 0xcc, 0xed}, []byte{0x34, 0x12, 0xcc, 0xed}},
		{"big_s16", "s16", "big", []byte{0x12, 0x34, 0xed, 0xcc}, []byte{0x34, 0x12, 0xcc, 0xed}},
		{"big_s24_padded", "s24", "big", []byte{0x12, 0x34, 0x56}, []byte{0x56, 0x34, 0x12}},
		{"big_s32", "s32", "big", []byte{0x12, 0x34, 0x56, 0x78}, []byte{0x78, 0x56, 0x34, 0x12}},
		{"u8_padded", "u8", "little", []byte{128, 255, 0}, []byte{128, 255, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := pcmFixtureConfig()
			config.SampleFormat, config.Endianness = tc.format, tc.endian
			wav, summary, err := pcmOutputToWAV(tc.raw, "audio/L16;rate=8000;channels=1", config)
			require.NoError(t, err)
			require.Equal(t, tc.want, wav[44:44+len(tc.want)])
			require.Equal(t, uint32(len(tc.raw)), binary.LittleEndian.Uint32(wav[40:44]))
			require.Equal(t, uint32(8000), binary.LittleEndian.Uint32(wav[24:28]))
			require.True(t, completeWAV(wav))
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(tc.raw)), summary.SourceSHA256)
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wav)), summary.OutputSHA256)
			require.Equal(t, tc.endian, summary.Endianness)
		})
	}
}

func TestPCMNoConversionWithoutMatchingConfiguration(t *testing.T) {
	raw := []byte{0x12, 0x34}
	for _, tc := range []struct {
		name, mime string
		config     *PCMToWAVConfig
	}{
		{"no_config", "audio/l16;rate=8000", nil},
		{"unmatched_type", "audio/other", pcmFixtureConfig()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "artifact")
			require.NoError(t, os.WriteFile(path, raw, 0600))
			cap := Capability{Template: &ProtocolTemplate{Response: TemplateResponse{PCMToWAV: tc.config}}}
			artifact, hash, summary, err := normalizeOutputArtifact(cap, Artifact{MIME: tc.mime}, path, "original-hash")
			require.NoError(t, err)
			require.Nil(t, summary)
			require.Equal(t, "original-hash", hash)
			require.Equal(t, tc.mime, artifact.MIME)
			stored, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, raw, stored)
		})
	}
}

func TestPCMOnlyConfiguredMIMEParametersOverrideFormat(t *testing.T) {
	config := pcmFixtureConfig()
	config.RateParameter, config.ChannelsParameter = "sample_rate", "channel_count"
	wav, summary, err := pcmOutputToWAV([]byte{1, 2, 3, 4}, "audio/l16;rate=8000;channels=2", config)
	require.NoError(t, err)
	require.Equal(t, 24000, summary.SampleRate)
	require.Equal(t, 1, summary.Channels)
	require.Equal(t, uint32(24000), binary.LittleEndian.Uint32(wav[24:28]))
	wav, summary, err = pcmOutputToWAV([]byte{1, 2, 3, 4}, "audio/l16;sample_rate=16000;channel_count=2", config)
	require.NoError(t, err)
	require.Equal(t, 16000, summary.SampleRate)
	require.Equal(t, 2, summary.Channels)
	require.Equal(t, uint16(2), binary.LittleEndian.Uint16(wav[22:24]))
	for _, contentType := range []string{
		"audio/l16;sample_rate=zero", "audio/l16;sample_rate=0", "audio/l16;sample_rate=384001",
		"audio/l16;sample_rate=\"\"", "audio/l16;channel_count=-1", "audio/l16;channel_count=33",
	} {
		t.Run(contentType, func(t *testing.T) {
			_, _, err := pcmOutputToWAV([]byte{1, 2, 3, 4}, contentType, config)
			require.Error(t, err)
		})
	}
}

func TestPCMRejectsInvalidConfigurationAndFrames(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*PCMToWAVConfig)
	}{
		{"mime_required", func(c *PCMToWAVConfig) { c.MIMETypes = nil }},
		{"container_mime", func(c *PCMToWAVConfig) { c.MIMETypes = []string{"audio/wav"} }},
		{"mime_parameters", func(c *PCMToWAVConfig) { c.MIMETypes = []string{"audio/l16;rate=8000"} }},
		{"format_required", func(c *PCMToWAVConfig) { c.SampleFormat = "" }},
		{"endian_required", func(c *PCMToWAVConfig) { c.Endianness = "" }},
		{"rate_required", func(c *PCMToWAVConfig) { c.SampleRate = 0 }},
		{"rate_bound", func(c *PCMToWAVConfig) { c.SampleRate = 384001 }},
		{"channels_required", func(c *PCMToWAVConfig) { c.Channels = 0 }},
		{"channels_bound", func(c *PCMToWAVConfig) { c.Channels = 33 }},
		{"parameter_syntax", func(c *PCMToWAVConfig) { c.RateParameter = "bad value" }},
		{"parameter_collision", func(c *PCMToWAVConfig) { c.ChannelsParameter = c.RateParameter }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := pcmFixtureConfig()
			tc.change(config)
			require.Error(t, validatePCMToWAV(config))
		})
	}
	_, _, err := pcmOutputToWAV(nil, "audio/l16", pcmFixtureConfig())
	require.Error(t, err)
	_, _, err = pcmOutputToWAV([]byte{1, 2, 3}, "audio/l16", pcmFixtureConfig())
	require.ErrorContains(t, err, "complete sample frames")
	_, _, err = pcmOutputToWAV([]byte{1, 2}, "audio/l16;channels=2", pcmFixtureConfig())
	require.ErrorContains(t, err, "complete sample frames")
}

func TestPCMExistingWAVIsPreservedAndMalformedWAVRejected(t *testing.T) {
	wav := templateAudioWAV(t)
	result, summary, err := pcmOutputToWAV(wav, "audio/l16", pcmFixtureConfig())
	require.NoError(t, err)
	require.Equal(t, wav, result)
	require.True(t, summary.AlreadyWAV)
	require.Empty(t, summary.SampleFormat)
	require.Zero(t, summary.SampleRate)
	require.Equal(t, summary.SourceSHA256, summary.OutputSHA256)
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"short_riff", []byte("RIFF")},
		{"truncated_data", wav[:len(wav)-1]},
		{"incomplete_trailing_chunk", append(append([]byte(nil), wav...), 1, 2)},
		{"missing_odd_padding", func() []byte {
			v, _, err := pcmOutputToWAV([]byte{1, 2, 3}, "audio/l16", &PCMToWAVConfig{MIMETypes: []string{"audio/l16"}, SampleFormat: "u8", Endianness: "little", SampleRate: 8000, Channels: 1})
			require.NoError(t, err)
			return v[:len(v)-1]
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.raw) >= 8 {
				binary.LittleEndian.PutUint32(tc.raw[4:8], uint32(len(tc.raw)-8))
			}
			_, _, err := pcmOutputToWAV(tc.raw, "audio/l16", pcmFixtureConfig())
			require.ErrorContains(t, err, "invalid WAV artifact header")
		})
	}
}

func TestConfiguredPCMHTTPDownloadCapturesPlayableWAV(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is required for artifact validation")
	}
	wav := templateAudioWAV(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/speech", r.URL.Path)
		w.Header().Set("Content-Type", "audio/L16;rate=8000;channels=1")
		_, _ = w.Write(wav[44:])
	}))
	defer server.Close()
	template := templateFixture(t, "gemini-audio-parts")
	cap := Capability{MediaType: "audio", Template: &template}
	blobs := BlobStore{Root: t.TempDir()}
	artifact, path, hash, err := blobs.CaptureTestArtifact(context.Background(), server.Client(), NewTaskID(), cap,
		Artifact{Key: "audio-1", Type: "audio", Source: server.URL + "/speech"}, Account{BaseURL: server.URL})
	require.NoError(t, err)
	require.True(t, artifact.Available)
	require.Equal(t, "audio/wav", artifact.MIME)
	require.Equal(t, "0.1", *artifact.Duration)
	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, wav, stored)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wav)), hash)
}
