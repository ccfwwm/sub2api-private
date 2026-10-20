package media

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMappingCreatedImageEditSupportsSingleAndMultipleUploads(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 32, 32))))
	for _, materials := range []map[string]Material{nil, {"image": {Formats: []string{"file_png"}}}} {
		original := Capability{Model: "gpt-image-2-1k", Adapter: "yunfei_gpt_images", MediaType: "image", Formats: []string{"json", "multipart"}, Operations: []string{"image", "image_edit"}, Materials: materials}
		cap := CapabilityDefaults(original)
		upload := Upload{Field: "image[]", Name: "ref.png", MIME: "image/png", Data: buf.Bytes()}
		for count := 1; count <= 2; count++ {
			uploads := []Upload{upload}
			if count == 2 {
				uploads = append(uploads, upload)
			}
			req, err := NormalizeUploads(cap, "image_edit", map[string]any{"prompt": "combine these references", "n": 1}, uploads)
			require.NoError(t, err)
			require.Equal(t, count, req.InputImages)
			require.Len(t, req.Uploads, count)
			require.NotContains(t, req.Parameters, "images")
		}
		_, err := NormalizeUploads(cap, "image_edit", map[string]any{"prompt": "edit"}, []Upload{upload, upload, upload})
		require.NoError(t, err)
		if materials != nil {
			require.Nil(t, materials["image"].MaxCount)
		}
	}
}

func TestImageEditDefaultsPreserveAdministratorRestrictions(t *testing.T) {
	zero, limit := 0, int64(4096)
	configured := Capability{Adapter: "yunfei_gpt_images", Operations: []string{"image_edit"}, Materials: map[string]Material{"image": {MaxCount: &zero, MaxFileBytes: &limit, Formats: []string{"file_png"}}}}
	cap := CapabilityDefaults(configured)
	require.Equal(t, 0, *cap.Materials["image"].MaxCount)
	require.Equal(t, limit, *cap.Materials["image"].MaxFileBytes)
	require.Equal(t, []string{"file_png"}, cap.Materials["image"].Formats)
	configured.Adapter = "yunfei_gemini_native"
	configured.Materials = nil
	require.Empty(t, CapabilityDefaults(configured).Materials)
}
