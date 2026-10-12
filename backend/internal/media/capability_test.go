package media

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func profiles(t *testing.T) []Capability {
	t.Helper()
	raw, e := os.ReadFile("../../../docs/multimodal/model-capabilities.json")
	require.NoError(t, e)
	var spec struct{ Profiles []Capability }
	require.NoError(t, json.Unmarshal(raw, &spec))
	require.Len(t, spec.Profiles, 38)
	return spec.Profiles
}
func findProfile(t *testing.T, name string) Capability {
	for _, c := range profiles(t) {
		if c.Model == name {
			return c
		}
	}
	t.Fatal("Missing profile", name)
	return Capability{}
}
func TestEveryModelProfileLoadsWithoutOpeningUnknownCapabilities(t *testing.T) {
	for _, c := range profiles(t) {
		t.Run(c.ID, func(t *testing.T) {
			require.False(t, c.Enabled)
			require.NoError(t, ValidateCapability(c, false))
			require.Error(t, ValidateCapability(c, true))
		})
	}
}
func TestSD25FixedThirtySecondsAndOmittedResolution(t *testing.T) {
	cap := findProfile(t, "00sd2.5")
	req, e := Normalize(cap, "video", map[string]any{"model": "00sd2.5", "prompt": "A peaceful valley"})
	require.NoError(t, e)
	require.Equal(t, "30", req.Duration)
	require.Equal(t, []string{"30"}, req.Quantities["fixed_duration"])
	for _, p := range []map[string]any{{"seconds": 15}, {"seconds": 30, "duration": 15}, {"resolution": "720p"}, {"_files": []any{}}, {"metadata": map[string]any{"price": "0"}}} {
		p["prompt"] = "A peaceful valley"
		_, e = Normalize(cap, "video", p)
		require.Error(t, e)
	}
	images := []any{}
	for i := 0; i < 11; i++ {
		images = append(images, map[string]any{"url": "https://example.com/image.png"})
	}
	_, e = Normalize(cap, "video", map[string]any{"prompt": "Animate these images", "images": images})
	require.Error(t, e)
}

func TestRequiredDurationCannotUseAnOptionalDefault(t *testing.T) {
	cap := findProfile(t, "JSadanco2.0")
	cap.Duration = Duration{Mode: "enum", Values: []int{5, 10}, Default: pointer(5), Required: true}
	_, err := Normalize(cap, "video", map[string]any{"prompt": "A valley"})
	require.Error(t, err)
	req, err := Normalize(cap, "video", map[string]any{"prompt": "A valley", "duration": 10})
	require.NoError(t, err)
	require.Equal(t, "10", req.Duration)
}
func TestVeoFramesAndNativeContentAreOrderedAndMetered(t *testing.T) {
	cap := findProfile(t, "veo-3.1-generate-preview")
	cap.Materials["image"] = Material{Formats: []string{"public_url_string"}, MaxCount: pointer(2)}
	req, e := Normalize(cap, "video", map[string]any{"prompt": "Motion", "seconds": 8, "image_urls": []any{"https://example.com/first.png", "https://example.com/last.png"}})
	require.NoError(t, e)
	require.Equal(t, "frames", req.Conditions["scene"])
	require.Equal(t, 2, req.InputImages)
	cap = findProfile(t, "doubao-seedance-2-0-65Z")
	cap.Duration = Duration{Mode: "enum", Values: []int{4, 8}, Required: true}
	cap.Materials["image"] = Material{Formats: []string{"public_url_object"}, MaxCount: pointer(2)}
	p := map[string]any{"prompt": "Motion", "seconds": 8, "metadata": map[string]any{"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/first.png"}, "role": "first_frame"}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/last.png"}, "role": "last_frame"}}}}
	req, e = Normalize(cap, "video", p)
	require.NoError(t, e)
	require.Equal(t, 2, req.InputImages)
	p["images"] = []any{map[string]any{"url": "https://example.com/other.png"}}
	_, e = Normalize(cap, "video", p)
	require.Error(t, e)
}
func TestMusicHintDoesNotPretendToBeActualUsage(t *testing.T) {
	cap := findProfile(t, "lyria-3.5")
	req, e := Normalize(cap, "chat_music", map[string]any{"messages": []any{map[string]any{"role": "user", "content": "Quiet ambient piano"}}, "duration": 61, "instrumental": true, "lyrics": "ignored"})
	require.NoError(t, e)
	require.Equal(t, []string{"61"}, req.Quantities["request_duration"])
	require.NotContains(t, req.Quantities, "artifact_duration")
	require.NotContains(t, req.Parameters, "lyrics")
	_, e = Normalize(cap, "audio", map[string]any{"prompt": "Music", "duration": 0})
	require.Error(t, e)
}
