package media

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"strconv"
	"strings"
)

// PCM conversion is opt-in template configuration, never a model/host preset.
// MIME parameters may override rate/channels only at the configured names.
type PCMToWAVConfig struct {
	MIMETypes         []string `json:"mime_types"`
	SampleFormat      string   `json:"sample_format"` // u8, s16, s24, s32
	Endianness        string   `json:"endianness"`    // little, big
	SampleRate        int      `json:"sample_rate"`
	Channels          int      `json:"channels"`
	RateParameter     string   `json:"rate_parameter,omitempty"`
	ChannelsParameter string   `json:"channels_parameter,omitempty"`
}

type ArtifactNormalization struct {
	SourceMIME   string `json:"source_mime"`
	OutputMIME   string `json:"output_mime"`
	SourceBytes  int    `json:"source_bytes"`
	OutputBytes  int    `json:"output_bytes"`
	SourceSHA256 string `json:"source_sha256"`
	OutputSHA256 string `json:"output_sha256"`
	SampleFormat string `json:"sample_format,omitempty"`
	Endianness   string `json:"source_endianness,omitempty"`
	SampleRate   int    `json:"sample_rate,omitempty"`
	Channels     int    `json:"channels,omitempty"`
	AlreadyWAV   bool   `json:"already_wav"`
}

func validatePCMToWAV(c *PCMToWAVConfig) error {
	if c == nil {
		return nil
	}
	if len(c.MIMETypes) == 0 || !contains([]string{"u8", "s16", "s24", "s32"}, c.SampleFormat) || !contains([]string{"little", "big"}, c.Endianness) || c.SampleRate < 1 || c.SampleRate > 384000 || c.Channels < 1 || c.Channels > 32 {
		return fail(400, "invalid_pcm_conversion", "PCM conversion requires MIME types, sample format, byte order, sample rate and channels")
	}
	for _, kind := range c.MIMETypes {
		base, params, err := mime.ParseMediaType(kind)
		if err != nil || !strings.HasPrefix(base, "audio/") || len(params) != 0 || contains([]string{"audio/wav", "audio/wave", "audio/x-wav", "audio/mpeg", "audio/mp3", "audio/ogg", "audio/flac", "audio/mp4", "audio/webm"}, base) {
			return fail(400, "invalid_pcm_conversion", "PCM conversion MIME list must identify raw audio, not containers")
		}
	}
	for _, name := range []string{c.RateParameter, c.ChannelsParameter} {
		if name != "" && !regexpPCMParameter(name) {
			return fail(400, "invalid_pcm_conversion", "Invalid PCM MIME parameter name")
		}
	}
	if c.RateParameter != "" && c.RateParameter == c.ChannelsParameter {
		return fail(400, "invalid_pcm_conversion", "Rate and channels must use distinct MIME parameters")
	}
	return nil
}

func regexpPCMParameter(name string) bool {
	for _, char := range name {
		if char < 'a' || char > 'z' {
			if char != '_' && char != '-' {
				return false
			}
		}
	}
	return name != ""
}

func pcmMIMEConfigured(c *PCMToWAVConfig, contentType string) bool {
	if c == nil {
		return false
	}
	base, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	for _, allowed := range c.MIMETypes {
		if strings.EqualFold(allowed, base) {
			return true
		}
	}
	return false
}

func normalizeOutputArtifact(cap Capability, artifact Artifact, path, hash string) (Artifact, string, *ArtifactNormalization, error) {
	if cap.Template == nil || !pcmMIMEConfigured(cap.Template.Response.PCMToWAV, artifact.MIME) {
		return artifact, hash, nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return artifact, hash, nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(file, (256<<20)+1))
	_ = file.Close()
	if err != nil || len(raw) == 0 || len(raw) > 256<<20 {
		return artifact, hash, nil, fmt.Errorf("invalid PCM artifact size")
	}
	wav, summary, err := pcmOutputToWAV(raw, artifact.MIME, cap.Template.Response.PCMToWAV)
	if err != nil {
		return artifact, hash, nil, err
	}
	if err = atomicWrite(path, wav); err != nil {
		return artifact, hash, nil, err
	}
	size := int64(len(wav))
	artifact.MIME, artifact.Size = "audio/wav", &size
	return artifact, summary.OutputSHA256, summary, nil
}

func pcmOutputToWAV(raw []byte, contentType string, c *PCMToWAVConfig) ([]byte, *ArtifactNormalization, error) {
	if err := validatePCMToWAV(c); err != nil {
		return nil, nil, err
	}
	if !pcmMIMEConfigured(c, contentType) {
		return raw, nil, nil
	}
	if len(raw) == 0 || len(raw) > 256<<20 {
		return nil, nil, fmt.Errorf("invalid PCM artifact size")
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, nil, err
	}
	parameter := func(name string, fallback, maximum int) (int, error) {
		if name == "" {
			return fallback, nil
		}
		text, exists := params[name]
		if !exists {
			return fallback, nil
		}
		value, err := strconv.Atoi(text)
		if err != nil || value < 1 || value > maximum {
			return 0, fmt.Errorf("invalid PCM %s parameter", name)
		}
		return value, nil
	}
	rate, err := parameter(c.RateParameter, c.SampleRate, 384000)
	if err != nil {
		return nil, nil, err
	}
	channels, err := parameter(c.ChannelsParameter, c.Channels, 32)
	if err != nil {
		return nil, nil, err
	}
	summary := &ArtifactNormalization{SourceMIME: contentType, OutputMIME: "audio/wav", SourceBytes: len(raw), SampleFormat: c.SampleFormat, Endianness: c.Endianness, SampleRate: rate, Channels: channels}
	inputHash := sha256.Sum256(raw)
	summary.SourceSHA256 = hex.EncodeToString(inputHash[:])
	var wav []byte
	if len(raw) >= 4 && string(raw[:4]) == "RIFF" {
		if !completeWAV(raw) {
			return nil, nil, fmt.Errorf("invalid WAV artifact header")
		}
		wav, summary.AlreadyWAV = raw, true
		summary.SampleFormat, summary.Endianness, summary.SampleRate, summary.Channels = "", "", 0, 0
	} else {
		width := map[string]int{"u8": 1, "s16": 2, "s24": 3, "s32": 4}[c.SampleFormat]
		if len(raw)%(width*channels) != 0 {
			return nil, nil, fmt.Errorf("PCM artifact is not aligned to complete sample frames")
		}
		wav = make([]byte, 44+len(raw)+len(raw)%2)
		copy(wav, "RIFF")
		binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
		copy(wav[8:16], "WAVEfmt ")
		binary.LittleEndian.PutUint32(wav[16:20], 16)
		binary.LittleEndian.PutUint16(wav[20:22], 1)
		binary.LittleEndian.PutUint16(wav[22:24], uint16(channels))
		binary.LittleEndian.PutUint32(wav[24:28], uint32(rate))
		binary.LittleEndian.PutUint32(wav[28:32], uint32(rate*channels*width))
		binary.LittleEndian.PutUint16(wav[32:34], uint16(channels*width))
		binary.LittleEndian.PutUint16(wav[34:36], uint16(width*8))
		copy(wav[36:40], "data")
		binary.LittleEndian.PutUint32(wav[40:44], uint32(len(raw)))
		copy(wav[44:], raw)
		if c.Endianness == "big" {
			for start := 44; start < 44+len(raw); start += width {
				for left, right := start, start+width-1; left < right; left, right = left+1, right-1 {
					wav[left], wav[right] = wav[right], wav[left]
				}
			}
		}
	}
	outputHash := sha256.Sum256(wav)
	summary.OutputBytes, summary.OutputSHA256 = len(wav), hex.EncodeToString(outputHash[:])
	return wav, summary, nil
}

func completeWAV(raw []byte) bool {
	if len(raw) < 44 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WAVE" || int(binary.LittleEndian.Uint32(raw[4:8])) != len(raw)-8 {
		return false
	}
	format, data := false, false
	offset := 12
	for offset+8 <= len(raw) {
		size := int(binary.LittleEndian.Uint32(raw[offset+4 : offset+8]))
		if size > len(raw)-offset-8 {
			return false
		}
		switch string(raw[offset : offset+4]) {
		case "fmt ":
			format = size >= 16
		case "data":
			data = size > 0
		}
		offset += 8 + size + size%2
	}
	return offset == len(raw) && format && data
}
