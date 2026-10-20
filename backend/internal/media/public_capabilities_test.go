package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicCapabilitiesPreservesVariantsAndRedactsBindings(t *testing.T) {
	cap := Capability{ID: "small", Version: "1", Model: "00sd2.5", MediaType: "video", Enabled: true, Status: "enabled", GroupIDs: []int64{1}, Operations: []string{"video"}, Sizes: []string{"1024x1024"}, AccountIDs: []int64{987}, UpstreamModel: "PRIVATE_MODEL", CostTemplates: []string{"PRIVATE_COST"}, ArtifactHosts: []string{"PRIVATE_HOST"}}
	cap.Duration = Duration{Mode: "enum", Values: []int{5}, Scenarios: []map[string]any{{"private": "PRIVATE_SCENARIO"}}}
	cap.RequestFormat = "v6k_url"
	cap.Materials = map[string]Material{"video": {ReferenceField: "videos"}}
	large := cap
	large.ID = "large"
	large.Sizes = []string{"2048x2048"}
	large.Duration.Values = []int{10}
	disabled := cap
	disabled.Enabled = false
	wrongGroup := cap
	wrongGroup.GroupIDs = []int64{2}
	unpriced := cap
	unpriced.Model = "unpriced"
	engine := &Engine{Config: func(context.Context, int64) ([]Capability, []PriceCard, error) {
		return []Capability{cap, large, disabled, wrongGroup, unpriced}, []PriceCard{testCard(baseCharge("second", "request_seconds", "0.1"))}, nil
	}}
	entries, err := engine.PublicCapabilities(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, "v6k_url", entries[0].RequestFormat)
	require.Equal(t, "images", entries[0].ReferenceField)
	require.Equal(t, "videos", entries[0].Materials["video"].ReferenceField)
	require.Equal(t, []int{5}, entries[0].Duration.Values)
	require.Equal(t, []int{10}, entries[1].Duration.Values)
	require.Equal(t, []string{"1024x1024"}, entries[0].Sizes)
	raw, err := json.Marshal(entries)
	require.NoError(t, err)
	for _, forbidden := range []string{"PRIVATE", "account_ids", "cost_templates", "upstream_model", "artifact_hosts", "channel_id"} {
		require.False(t, strings.Contains(string(raw), forbidden), forbidden)
	}
}

func TestPublicSelectionCannotMixVariantsOrLeakSelector(t *testing.T) {
	cap := Capability{ID: "small", Version: "1", Model: "chosen", UpstreamModel: "upstream", Adapter: "yunfei_gpt_images", MediaType: "image", Enabled: true, Status: "enabled", GroupIDs: []int64{7}, Operations: []string{"image"}, Sizes: []string{"1024x1024"}}
	large := cap
	large.ID = "large"
	large.Sizes = []string{"2048x2048"}
	engine := &Engine{Config: func(context.Context, int64) ([]Capability, []PriceCard, error) {
		return []Capability{cap, large}, nil, nil
	}}
	p := map[string]any{"model": "chosen", "prompt": "tiny", "size": "2048x2048", "public_profile_id": publicProfileID(cap)}
	explicit, err := engine.SelectCapability(context.Background(), "chosen", "image", p, nil, Identity{GroupID: 7})
	require.NoError(t, err)
	require.Equal(t, cap.ID, explicit.ID)
	p["public_profile_id"] = publicProfileID(large)
	selected, err := engine.SelectCapability(context.Background(), "chosen", "image", p, nil, Identity{GroupID: 7})
	require.NoError(t, err)
	require.Equal(t, large.ID, selected.ID)
	req, err := Normalize(*selected, "image", p)
	require.NoError(t, err)
	require.NotContains(t, req.Parameters, "public_profile_id")
	p["public_profile_id"] = "mp_removed"
	_, err = engine.SelectCapability(context.Background(), "chosen", "image", p, nil, Identity{GroupID: 7})
	require.Error(t, err)
}

func TestMaskMultipartValidationAndBilling(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 2, 2))))
	data := buf.Bytes()
	limit := int64(4096)
	count := 1
	cap := Capability{Adapter: "yunfei_gpt_images", Model: "chosen", UpstreamModel: "chosen", MediaType: "image", Formats: []string{"multipart"}, Materials: map[string]Material{"image": {Formats: []string{"file_png", "data_url"}, MaxCount: &count, MaxFileBytes: &limit}}}
	uploads := []Upload{{Field: "image[]", MIME: "image/png", Name: "ref.png", Data: data}, {Field: "mask", MIME: "image/png", Name: "mask.png", Data: data}}
	req, err := NormalizeUploads(cap, "image_edit", map[string]any{"prompt": "edit"}, uploads)
	require.NoError(t, err)
	require.Equal(t, 1, req.InputImages)
	require.Len(t, req.Uploads, 2)
	require.NotContains(t, req.Parameters, "mask")
	for _, bad := range [][]Upload{append(uploads, uploads[1]), {uploads[1]}, {{Field: "mask", MIME: "image/jpeg", Data: data}, uploads[0]}} {
		_, err = NormalizeUploads(cap, "image_edit", map[string]any{"prompt": "edit"}, bad)
		require.Error(t, err)
	}
	p := map[string]any{"prompt": "edit", "image": []any{"data:image/png;base64," + base64.StdEncoding.EncodeToString(data)}, "mask": "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)}
	_, err = Normalize(cap, "image_edit", p)
	require.NoError(t, err)
	_, err = Normalize(cap, "image", p)
	require.Error(t, err)
	cap.Adapter = "yunfei_gemini_native"
	_, err = Normalize(cap, "image_edit", p)
	require.Error(t, err)
}
