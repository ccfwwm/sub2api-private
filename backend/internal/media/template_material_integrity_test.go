package media

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTemplateMaterialWrapperCannotReplaceReference(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	cap := Capability{Model: "public", UpstreamModel: "configured", MediaType: "video", Template: &template}
	ref := map[string]any{"url": "https://example.org/reference.png", "role": "first_frame", "weight": 0.5}
	req, err := Normalize(cap, "video", map[string]any{"seconds": 5, "images": []any{ref}})
	require.NoError(t, err)
	template.Materials[0].Wrapper = map[string]any{"content": map[string]any{"$input": "value"}}
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	require.Equal(t, []any{map[string]any{"content": ref}}, wire.Body["images"])
	for _, wrapper := range []map[string]any{
		{"text": "reference omitted"},
		{"url": map[string]any{"$input": "value.url"}},
	} {
		template.Materials[0].Wrapper = wrapper
		_, err = BuildTemplateRequest(context.Background(), nil, cap, req)
		require.ErrorContains(t, err, "material_integrity_failed")
	}
}

func TestTemplateMaterialTargetsAndShapesCannotDropAttributes(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	cap := Capability{Model: "public", UpstreamModel: "configured", MediaType: "video", Template: &template}
	template.Materials[0].Shape = "content"
	req, err := Normalize(cap, "video", map[string]any{"seconds": 5, "images": []any{map[string]any{"url": "https://example.org/reference.png", "type": "custom-attribute"}}})
	require.NoError(t, err)
	_, err = BuildTemplateRequest(context.Background(), nil, cap, req)
	require.ErrorContains(t, err, "unrepresentable_reference")
	template.Materials[0].Shape = "preserve"
	template.Materials[0].Single = true
	template.Materials[0].Target = "reference"
	template.Materials[1].Single = true
	template.Materials[1].Target = "reference"
	req, err = Normalize(cap, "video", map[string]any{"seconds": 5, "images": []any{"https://example.org/reference.png"}, "videos": []any{"https://example.org/reference.mp4"}})
	require.NoError(t, err)
	_, err = BuildTemplateRequest(context.Background(), nil, cap, req)
	require.ErrorContains(t, err, "material_integrity_failed")
}

func TestTemplateMaterialStringShapeRequiresExplicitExactPropertyFilter(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	template.Materials = template.Materials[:1]
	template.Materials[0].Shape = "string"
	template.Materials[0].DropProperties = map[string][]any{"role": {"reference"}}
	cap := Capability{Model: "public", UpstreamModel: "configured", MediaType: "video", Template: &template}
	input := map[string]any{"seconds": 5, "images": []any{map[string]any{"url": "https://example.org/reference.png", "role": "reference"}}}
	req, err := Normalize(cap, "video", input)
	require.NoError(t, err)
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	require.Equal(t, []any{"https://example.org/reference.png"}, wire.Body["images"])

	// A filter is exact and explicit: it cannot silently discard a different
	// frame role or any unrelated provider extension.
	for _, ref := range []any{
		map[string]any{"url": "https://example.org/first.png", "role": "first_frame"},
		map[string]any{"url": "https://example.org/reference.png", "role": "reference", "weight": 0.7},
	} {
		req, err = Normalize(cap, "video", map[string]any{"seconds": 5, "images": []any{ref}})
		require.NoError(t, err)
		_, err = BuildTemplateRequest(context.Background(), nil, cap, req)
		require.ErrorContains(t, err, "unrepresentable_reference")
	}

	template.Materials[0].DropProperties = map[string][]any{"role": {"image"}}
	req, err = Normalize(cap, "video", input)
	require.NoError(t, err)
	_, err = BuildTemplateRequest(context.Background(), nil, cap, req)
	require.ErrorContains(t, err, "unrepresentable_reference")
}

func TestBindingMapsImageObjectArrayToProviderScalarOnlyWhenConfigured(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	template.Materials = []MaterialRule{{
		Kind: "image", Target: "Alias.images", Encoding: "preserve", Shape: "string", Single: true,
		DropProperties: map[string][]any{"weight": {0.7}},
	}}
	cap := Capability{Model: "public", UpstreamModel: "configured", MediaType: "video", Template: &template}
	request := map[string]any{"seconds": 5, "images": []any{
		map[string]any{"url": "https://example.org/reference.png", "weight": 0.7},
	}}
	req, err := Normalize(cap, "video", request)
	require.NoError(t, err)
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"images": "https://example.org/reference.png"}, wire.Body["Alias"])

	// A supplier binding that declares a scalar cannot silently flatten a
	// multi-image request. Reject it while preparing the request, before a task
	// or billing reservation is created.
	images, ok := request["images"].([]any)
	require.True(t, ok)
	request["images"] = append(images, map[string]any{"url": "https://example.org/second.png", "weight": 0.7})
	req, err = Normalize(cap, "video", request)
	require.NoError(t, err)
	_, err = BuildTemplateRequest(context.Background(), nil, cap, req)
	require.ErrorContains(t, err, "template_reference_limit")
}

func TestBindingMapsScreenshotImageObjectToProviderString(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	template.Materials = []MaterialRule{{
		Kind: "image", Target: "Alias.images", Encoding: "data_url", Shape: "string", Single: true,
	}}
	cap := Capability{Model: "public", UpstreamModel: "configured", MediaType: "video", Template: &template}
	png := base64String(pngFixture(t))
	request := map[string]any{"seconds": 4, "images": []any{
		map[string]any{"mime_type": "image/png", "b64_json": png},
	}}
	req, err := Normalize(cap, "video", request)
	require.NoError(t, err)
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"images": "data:image/png;base64," + png}, wire.Body["Alias"])

	// A provider binding with a scalar field cannot flatten multiple images.
	images, ok := request["images"].([]any)
	require.True(t, ok)
	request["images"] = append(images, map[string]any{"mime_type": "image/png", "b64_json": png})
	req, err = Normalize(cap, "video", request)
	require.NoError(t, err)
	_, err = BuildTemplateRequest(context.Background(), nil, cap, req)
	require.ErrorContains(t, err, "template_reference_limit")
}

func TestFoxrouterTemplateMapsBase64ImageObjectsToStringArray(t *testing.T) {
	var template ProtocolTemplate
	for _, candidate := range BuiltinTemplates() {
		if candidate.ID == "foxrouter-video-base64-image-strings" {
			template = candidate
			break
		}
	}
	require.NotEmpty(t, template.ID)
	require.NoError(t, ValidateTemplate(template, true))

	png := base64String(pngFixture(t))
	want := "data:image/png;base64," + png
	for _, count := range []int{1, 2} {
		images := make([]any, count)
		for i := range images {
			images[i] = map[string]any{"mime_type": "image/png", "b64_json": png}
		}
		cap := Capability{Model: "gemini-omni-flash", UpstreamModel: "gemini-omni-flash", MediaType: "video", Template: &template}
		req, err := Normalize(cap, "video", map[string]any{"seconds": 4, "prompt": "test", "images": images})
		require.NoError(t, err)
		wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
		require.NoError(t, err)

		raw, err := json.Marshal(wire.Body)
		require.NoError(t, err)
		// Alias in a Go decoder error names its embedded type, not a JSON
		// envelope. Decode the same flat contract the real supplier accepts.
		type Alias struct {
			Images []string `json:"images"`
		}
		upstream := struct{ *Alias }{Alias: &Alias{}}
		require.NoError(t, json.Unmarshal(raw, &upstream))
		require.NotContains(t, wire.Body, "Alias")
		require.Len(t, upstream.Images, count)
		for _, image := range upstream.Images {
			require.Equal(t, want, image)
		}
	}
}

func TestScalarDataURLMappingPreservesMIMEAndRejectsUnrepresentedAttributes(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	template.Materials = []MaterialRule{{
		Kind: "image", Target: "Alias.images", Encoding: "data_url", Shape: "string", Single: true,
	}}
	cap := Capability{Model: "public", UpstreamModel: "configured", MediaType: "video", Template: &template}
	png := base64String(pngFixture(t))
	request := map[string]any{"seconds": 4, "images": []any{
		map[string]any{"mime_type": "image/png", "b64_json": png, "role": "unexpected"},
	}}
	req, err := Normalize(cap, "video", request)
	require.NoError(t, err)
	_, err = BuildTemplateRequest(context.Background(), nil, cap, req)
	require.ErrorContains(t, err, "unrepresentable_reference")

	request["images"] = []any{map[string]any{"mime_type": "image/png", "b64_json": png}}
	req, err = Normalize(cap, "video", request)
	require.NoError(t, err)
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	alias, ok := wire.Body["Alias"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "data:image/png;base64,"+png, alias["images"])
}

func TestTemplatePreservesReferenceLocatorAndPayloadForEveryMediaKind(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	template.Materials = []MaterialRule{
		{Kind: "image", Target: "reference_images", Encoding: "preserve", Shape: "preserve"},
		{Kind: "video", Target: "reference_videos", Encoding: "preserve", Shape: "preserve"},
		{Kind: "audio", Target: "reference_audios", Encoding: "preserve", Shape: "preserve"},
	}
	png := base64String(pngFixture(t))
	input := map[string]any{
		"model": "public", "prompt": "keep source fields", "seconds": 5,
		"images": []any{
			map[string]any{"b64_json": png, "mime_type": "image/png", "role": "first_frame"},
			map[string]any{"url": "https://example.org/second.png", "role": "last_frame"},
		},
		"videos": []any{map[string]any{"b64_json": "AA==", "mime_type": "video/mp4", "source": "camera"}},
		"audios": []any{map[string]any{"b64_json": "AA==", "mime_type": "audio/mpeg", "language": "zh"}},
	}
	cap := Capability{Model: "public", UpstreamModel: "provider", MediaType: "video", Template: &template}
	req, err := Normalize(cap, "video", input)
	require.NoError(t, err)
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	require.Equal(t, input["images"], wire.Body["reference_images"])
	require.Equal(t, input["videos"], wire.Body["reference_videos"])
	require.Equal(t, input["audios"], wire.Body["reference_audios"])
	inputs, err := PreviewInputs(req)
	require.NoError(t, err)
	require.Len(t, inputs, 4)
	require.Equal(t, "video/mp4", inputs[2].MIME)
	require.Equal(t, "audio/mpeg", inputs[3].MIME)
}

func TestTemplateExplicitNestedPropertyValueMappingAndConversion(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	template.Materials = []MaterialRule{{Kind: "image", Target: "images", Encoding: "preserve", Shape: "preserve", PropertyRules: []ParameterRule{
		{Op: "map", Source: "metadata.role", Values: map[string]any{"reference": "input"}, OnUnmapped: "preserve"},
		{Op: "convert", Source: "metadata.weight", Type: "number"},
	}}}
	require.NoError(t, ValidateTemplate(template, false))
	ref := map[string]any{"b64_json": base64String(pngFixture(t)), "metadata": map[string]any{"role": "reference", "weight": "0.7", "keep": true}}
	cap := Capability{Model: "public", UpstreamModel: "provider", MediaType: "video", Template: &template}
	req, err := Normalize(cap, "video", map[string]any{"seconds": 5, "images": []any{ref}})
	require.NoError(t, err)
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	images, ok := wire.Body["images"].([]any)
	require.True(t, ok)
	require.Len(t, images, 1)
	got, ok := images[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, ref["b64_json"], got["b64_json"])
	require.Equal(t, map[string]any{"role": "input", "weight": float64(0.7), "keep": true}, got["metadata"])
	metadata, ok := ref["metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "0.7", metadata["weight"])
	template.Materials[0].PropertyRules = []ParameterRule{{Op: "convert", Source: "b64_json", Type: "string"}}
	require.ErrorContains(t, ValidateTemplate(template, false), "ordinary attributes")
}

func TestTemplateExplicitlyMapsAndFiltersNestedReferenceProperties(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	template.Materials = []MaterialRule{{
		Kind: "image", Target: "reference_images", Encoding: "preserve", Shape: "preserve",
		DropProperties: map[string][]any{"metadata.internal": {true}},
		MapProperties: map[string]string{
			"b64_json":             "image.data",
			"metadata.description": "metadata.caption",
			"role":                 "metadata.frame_role",
		},
	}}
	cap := Capability{Model: "public", UpstreamModel: "provider", MediaType: "video", Template: &template}
	ref := map[string]any{
		"b64_json": "data:image/png;base64,AA==", "role": "first_frame",
		"metadata": map[string]any{"description": "opening frame", "internal": true},
	}
	req := Request{Operation: "video", Model: "public", Parameters: map[string]any{"model": "public", "prompt": "frame", "seconds": 5}, MaterialRefs: map[string][]any{"image": {ref}}}
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	require.Equal(t, []any{map[string]any{
		"image":    map[string]any{"data": "data:image/png;base64,AA=="},
		"metadata": map[string]any{"caption": "opening frame", "frame_role": "first_frame"},
	}}, wire.Body["reference_images"])
	// Mapping and filtering operate on cloned metadata and never mutate a task's
	// normalized reference snapshot.
	require.Contains(t, ref, "b64_json")
	require.Contains(t, ref, "role")
	metadata, ok := ref["metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "opening frame", metadata["description"])
}

func TestTemplateMaterialPropertyMappingsAreValidated(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	template.Materials[0].MapProperties = map[string]string{"role": "metadata.role"}
	require.NoError(t, ValidateTemplate(template, false))
	template.Materials[0].MapProperties = map[string]string{"role": "b64_json"}
	require.ErrorContains(t, ValidateTemplate(template, false), "Only a top-level reference locator")
	template.Materials[0].Shape = "url"
	// Locator movement is controlled by an explicit structure, so an explicit
	// property mapping may not compete with it.
	template.Materials[0].MapProperties = map[string]string{"url": "b64_json"}
	require.ErrorContains(t, ValidateTemplate(template, false), "Map reference locators")
}

func TestTemplateMapsLocatorIntoExistingNestedObjectWithoutLosingAttributes(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	template.Materials = []MaterialRule{{Kind: "image", Target: "images", Encoding: "preserve", Shape: "preserve", MapProperties: map[string]string{"url": "image.url", "metadata.url": "metadata.source"}}}
	require.NoError(t, ValidateTemplate(template, false))
	ref := map[string]any{"url": "https://example.org/image.png", "image": map[string]any{"weight": 0.7}, "metadata": map[string]any{"url": "attribution"}}
	cap := Capability{Model: "public", UpstreamModel: "provider", MediaType: "video", Template: &template}
	req, err := Normalize(cap, "video", map[string]any{"seconds": 5, "images": []any{ref}})
	require.NoError(t, err)
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	require.Equal(t, []any{map[string]any{"image": map[string]any{"url": ref["url"], "weight": 0.7}, "metadata": map[string]any{"source": "attribution"}}}, wire.Body["images"])
	require.Equal(t, map[string]any{"weight": 0.7}, ref["image"])
}
