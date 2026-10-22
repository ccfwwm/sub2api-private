package media

import (
	"context"
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
