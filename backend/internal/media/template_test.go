package media

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func templateFixture(t *testing.T, id string) ProtocolTemplate {
	t.Helper()
	for _, v := range BuiltinTemplates() {
		if v.ID == id {
			return v
		}
	}
	t.Fatal(id)
	return ProtocolTemplate{}
}
func pngFixture(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 2, 2))))
	return b.Bytes()
}

func TestTemplateLegacySingleMaterialMappingDoesNotBecomeRename(t *testing.T) {
	cap := Capability{ID: "account-2093", Model: "public-video", UpstreamModel: "provider-video", MediaType: "video", Operations: []string{"video"}, GroupIDs: []int64{19}, AccountIDs: []int64{2093}, Enabled: true, Status: "enabled", TemplateRef: &TemplateRef{ID: "video-single-image", Version: 1}, FieldMapping: map[string]string{"image": "image.url", "seconds": "duration", "resolution": "resolution"}}
	resolved, err := ResolveTemplateBindings([]Capability{cap}, nil, func(ref TemplateRef) (ProtocolTemplate, error) { return templateFixture(t, ref.ID), nil })
	require.NoError(t, err)
	require.Len(t, resolved, 1)
	require.True(t, resolved[0].Enabled)
	require.Equal(t, "image", resolved[0].Template.Materials[0].Target)
	require.Equal(t, []ParameterRule{{Phase: "wire", Op: "rename", Source: "seconds", Target: "duration"}}, resolved[0].TemplateRules)
	request, err := BuildTemplateRequest(context.Background(), nil, resolved[0], Request{Operation: "video", Model: cap.Model, Parameters: map[string]any{"model": cap.Model, "prompt": "lake", "seconds": 5}, MaterialRefs: map[string][]any{"image": {"https://example.org/reference.png"}}})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"url": "https://example.org/reference.png"}, request.Body["image"])
	require.EqualValues(t, 5, request.Body["duration"])
	require.NotContains(t, request.Body, "seconds")
}

func TestTemplateMappingResolutionHasStableProfileAndRuleOrder(t *testing.T) {
	cap := Capability{ID: "account:42:video", Version: "saved-version", Model: "video", UpstreamModel: "provider-video", MediaType: "video", Operations: []string{"video"}, GroupIDs: []int64{19}, AccountIDs: []int64{42}, Enabled: true, Status: "enabled", TemplateRef: &TemplateRef{ID: "video-arrays", Version: 1}, FieldMapping: map[string]string{"seconds": "duration", "ratio": "aspect_ratio", "quality": "generation_quality", "resolution": "output_resolution"}}
	load := func(ref TemplateRef) (ProtocolTemplate, error) { return templateFixture(t, ref.ID), nil }
	baseline, err := ResolveTemplateBindings([]Capability{cap}, nil, load)
	require.NoError(t, err)
	for i := 0; i < 100; i++ {
		resolved, err := ResolveTemplateBindings([]Capability{cap}, nil, load)
		require.NoError(t, err)
		require.Equal(t, baseline[0].TemplateRules, resolved[0].TemplateRules)
		require.Equal(t, baseline[0].Version, resolved[0].Version)
		require.Equal(t, publicProfileID(baseline[0]), publicProfileID(resolved[0]))
	}
	changed := cap
	changed.UpstreamModel = "different-provider-video"
	resolved, err := ResolveTemplateBindings([]Capability{changed}, nil, load)
	require.NoError(t, err)
	require.NotEqual(t, publicProfileID(baseline[0]), publicProfileID(resolved[0]))
}

// Exercises every shipped template through actual HTTP serialization, response
// parsing, polling when configured, authenticated download and byte comparison.
func TestBuiltinTemplatesCompleteHTTP(t *testing.T) {
	ctx := context.Background()
	pngBytes := pngFixture(t)
	for _, template := range BuiltinTemplates() {
		t.Run(template.ID, func(t *testing.T) {
			require.NoError(t, ValidateTemplate(template, template.Status == "enabled"))
			if template.ID == "custom-blank" {
				template.Operations = []string{"image"}
				template.MediaTypes = []string{"image"}
				template.Response = TemplateResponse{URLs: []string{"url"}}
			}
			template.Submit.Path = "/submit"
			for op, endpoint := range template.OperationSubmit {
				endpoint.Path = "/submit"
				template.OperationSubmit[op] = endpoint
			}
			if template.Query != nil {
				template.Query.Path = "/query/{task_id}"
			}
			template.ContentPath = "/content/{task_id}"
			template.Response.URLs = []string{"artifacts[].location"}
			template.Response.Base64 = nil
			template.Auth = TemplateAuth{Mode: "header", Name: "X-Account-Key"}
			template.DownloadAuth = &TemplateAuth{Mode: "query", Name: "access_key"}
			mediaType := template.MediaTypes[0]
			output := pngBytes
			mime := mediaType + "/mp4"
			if mediaType == "image" {
				mime = "image/png"
			}
			if mediaType == "audio" {
				mime = "audio/wav"
			}
			calls := []string{}
			var sent map[string]any
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method+" "+r.URL.Path)
				if strings.HasPrefix(r.URL.Path, "/content/") {
					require.Empty(t, r.Header.Get("X-Account-Key"))
					require.Equal(t, "test-secret", r.URL.Query().Get("access_key"))
					w.Header().Set("Content-Type", mime)
					_, _ = w.Write(output)
					return
				}
				require.Equal(t, "test-secret", r.Header.Get("X-Account-Key"))
				if r.URL.Path == "/submit" {
					if template.Submit.Encoding == "multipart" {
						require.NoError(t, r.ParseMultipartForm(4<<20))
						require.NotEmpty(t, r.MultipartForm.File["image"])
						file, e := r.MultipartForm.File["image"][0].Open()
						require.NoError(t, e)
						data, e := io.ReadAll(file)
						_ = file.Close()
						require.NoError(t, e)
						require.Equal(t, pngBytes, data)
					} else {
						require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
					}
					if template.Mode == "sync_binary" {
						w.Header().Set("Content-Type", mime)
						_, _ = w.Write(output)
						return
					}
					if template.Mode == "async" {
						_, _ = io.WriteString(w, `{"id":"job/a","status":"queued"}`)
						return
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"job/a","status":"completed","artifacts":[{"location":"/content/job-a"}]}`)
			}))
			defer server.Close()
			cap := CapabilityDefaults(Capability{Model: "public", UpstreamModel: "arbitrary-model-with-no-prefix", MediaType: mediaType, Template: &template})
			p := map[string]any{"model": "public", "prompt": "fixture", "input": "汉😀é", "seconds": 5, "custom_optional": map[string]any{"flag": false, "count": 0}}
			if len(template.Materials) > 0 {
				p["images"] = []any{"data:image/png;base64," + base64String(pngBytes)}
				if template.ID == "video-frame-fields" {
					p["images"] = []any{map[string]any{"url": "data:image/png;base64," + base64String(pngBytes), "role": "first_frame"}}
				}
			}
			req, err := NormalizeUploads(cap, template.Operations[0], p, nil)
			require.NoError(t, err)
			wire, err := BuildTemplateRequest(ctx, server.Client(), cap, req)
			require.NoError(t, err)
			req.Wire = &wire
			adapter := &Adapter{Client: server.Client()}
			account := Account{BaseURL: server.URL, Key: "test-secret"}
			result, err := adapter.Submit(ctx, cap, req, account)
			require.NoError(t, err)
			if template.Mode == "async" {
				require.Equal(t, "in_progress", result.Status)
				require.Empty(t, result.Artifacts)
				result, err = adapter.Poll(ctx, StoredTask{UpstreamID: result.ID, Snapshot: Snapshot{Capability: cap}}, account)
				require.NoError(t, err)
				require.Contains(t, calls, "GET /query/job/a")
			}
			require.Equal(t, "completed", result.Status)
			require.Len(t, result.Artifacts, 1)
			if template.Mode == "sync_binary" {
				data, mime, e := decodeArtifact(result.Artifacts[0].Source)
				require.NoError(t, e)
				require.Equal(t, output, data)
				require.NotEmpty(t, mime)
				var saved map[string]any
				require.NoError(t, json.Unmarshal(result.Response, &saved))
				restored := ParseTemplateResponse(saved, cap)
				require.Equal(t, result.Artifacts, restored.Artifacts)
			} else {
				blobs, e := NewBlobStore(t.TempDir(), "test-encryption-key-stable-at-least-32-bytes")
				require.NoError(t, e)
				artifact, path, hash, e := blobs.Download(ctx, server.Client(), StoredTask{ID: NewTaskID(), Snapshot: Snapshot{Capability: cap}}, result.Artifacts[0], account, 1<<20)
				require.NoError(t, e)
				require.True(t, artifact.Available)
				require.NotEmpty(t, path)
				require.Len(t, hash, 64)
			}
		})
	}
}

func TestTemplateSpeechCharactersAndNestedBillingRules(t *testing.T) {
	template := templateFixture(t, "speech-binary")
	cap := Capability{Model: "public", UpstreamModel: "private", MediaType: "audio", Template: &template}
	req, err := Normalize(cap, "speech", map[string]any{"input": "汉😀é", "voice": "user-choice", "custom": false})
	require.NoError(t, err)
	require.Equal(t, []string{"4"}, req.Quantities["input_characters"])
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	require.Equal(t, "user-choice", wire.Body["voice"])
	require.Equal(t, false, wire.Body["custom"])
	require.Error(t, ValidateParameterRules([]ParameterRule{{Phase: "wire", Op: "fixed", Target: "request.seconds", Value: 20}}))
	require.Error(t, ValidateParameterRules([]ParameterRule{{Phase: "wire", Op: "fixed", Target: "request.model", Value: "other"}}))
	template.Response.Usage = map[string]string{"input_characters": "provider.forged"}
	require.Error(t, ValidateTemplate(template, true))
}

func TestTemplateDynamicResultMIMEFilenameAndMasks(t *testing.T) {
	template := templateFixture(t, "gemini-parts")
	result := ParseTemplateResponse(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"inlineData": map[string]any{"mimeType": "image/webp", "data": "YWJj"}}}}}}}, Capability{MediaType: "image", Template: &template})
	require.Equal(t, "completed", result.Status)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, "image/webp", result.Artifacts[0].MIME)
	require.Equal(t, "data:image/webp;base64,YWJj", result.Artifacts[0].Source)
	require.Equal(t, "test.mp4", safeArtifactFilename("../../test.mp4"))
	edit := templateFixture(t, "image-edit-json")
	cap := Capability{Model: "p", UpstreamModel: "u", Template: &edit, MediaType: "image"}
	full := "data:image/png;base64," + base64String(pngFixture(t))
	req, err := Normalize(cap, "image_edit", map[string]any{"images": []any{full}, "mask": full})
	require.NoError(t, err)
	require.Len(t, req.MaterialRefs["mask"], 1)
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"url": full}, wire.Body["mask"])
}

func TestTemplateCanonicalSixBase64ShapesAndFiveReferences(t *testing.T) {
	data := pngFixture(t)
	raw := base64String(data)
	full := "data:image/png;base64," + raw
	for _, value := range []any{full, map[string]any{"url": full}, map[string]any{"b64_json": full}, raw, map[string]any{"url": raw}, map[string]any{"b64_json": raw}} {
		template := templateFixture(t, "video-arrays")
		for i := range template.Materials {
			template.Materials[i].Encoding = "data_url"
			template.Materials[i].Shape = "string"
		}
		cap := Capability{Model: "public", UpstreamModel: "other", MediaType: "video", Template: &template}
		req, err := Normalize(cap, "video", map[string]any{"seconds": 5, "images": []any{value}})
		require.NoError(t, err)
		wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
		require.NoError(t, err)
		require.Equal(t, []any{full}, wire.Body["images"])
	}
	template := templateFixture(t, "video-arrays")
	cap := Capability{Model: "public", UpstreamModel: "other", MediaType: "video", Template: &template}
	p := map[string]any{"seconds": 5, "images": []any{"https://example.org/1.png", map[string]any{"url": "https://example.org/2.png", "role": "first_frame", "weight": 0.4}, "https://example.org/3.png", "https://example.org/4.png"}, "videos": []any{"https://example.org/v.mp4"}, "custom": false}
	req, err := Normalize(cap, "video", p)
	require.NoError(t, err)
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	require.Equal(t, p["images"], wire.Body["images"])
	require.Equal(t, p["videos"], wire.Body["videos"])
	require.Equal(t, false, wire.Body["custom"])
	template.Materials[0].Shape = "string"
	_, err = BuildTemplateRequest(context.Background(), nil, cap, req)
	require.ErrorContains(t, err, "unrepresentable_reference")
}
func TestTemplateRulesPreserveFalseNullAndReferenceIntent(t *testing.T) {
	template := templateFixture(t, "video-arrays")
	template.Rules = []ParameterRule{{Phase: "semantic", Op: "default", Target: "seconds", Value: 8}, {Phase: "semantic", Op: "default", Target: "flag", Value: true}, {Phase: "semantic", Op: "default", Target: "zero", Value: 1}, {Phase: "semantic", Op: "default", Target: "nullable", Value: "default"}, {Phase: "wire", Op: "rename", Source: "seconds", Target: "duration"}}
	cap := Capability{Model: "p", UpstreamModel: "u", MediaType: "video", Template: &template}
	p := map[string]any{"seconds": 5, "flag": false, "zero": 0, "nullable": nil, "array": []any{}}
	req, err := Normalize(cap, "video", p)
	require.NoError(t, err)
	require.Equal(t, []string{"5"}, req.Quantities["request_duration"])
	wire, err := BuildTemplateRequest(context.Background(), nil, cap, req)
	require.NoError(t, err)
	require.EqualValues(t, 5, wire.Body["duration"])
	require.Equal(t, false, wire.Body["flag"])
	require.Nil(t, wire.Body["nullable"])
	require.Empty(t, wire.Body["array"])
	p["duration"] = 8
	_, err = Normalize(cap, "video", p)
	require.ErrorContains(t, err, "alias_conflict")
	delete(p, "duration")
	template.Rules = []ParameterRule{{Phase: "semantic", Op: "remove", Source: "images"}}
	p["images"] = []any{"https://example.org/1.png"}
	_, err = Normalize(cap, "video", p)
	require.ErrorContains(t, err, "material_integrity_failed")
	require.Error(t, ValidateParameterRules([]ParameterRule{{Phase: "wire", Op: "fixed", Target: "seconds", Value: 8}}))
	require.Error(t, ValidateParameterRules([]ParameterRule{{Phase: "wire", Op: "rename", Source: "a", Target: "b"}, {Phase: "wire", Op: "rename", Source: "b", Target: "a"}}))
}
func TestTemplateStatesAndSingleImageNeverDiscardInputs(t *testing.T) {
	template := templateFixture(t, "video-single-image")
	cap := Capability{Model: "anything", UpstreamModel: "renamed-without-protocol-hint", MediaType: "video", Template: &template}
	req, err := Normalize(cap, "video", map[string]any{"seconds": 5, "images": []any{"https://example.org/a.png", "https://example.org/b.png"}})
	require.NoError(t, err)
	_, err = BuildTemplateRequest(context.Background(), nil, cap, req)
	require.ErrorContains(t, err, "template_reference_limit")
	result := ParseTemplateResponse(map[string]any{"id": "x"}, cap)
	require.True(t, result.Accepted)
	require.Equal(t, "in_progress", result.Status)
	result = ParseTemplateResponse(map[string]any{"status": "failed", "url": "https://example.org/output.mp4"}, cap)
	require.Equal(t, "failed", result.Status)
	require.Empty(t, result.Artifacts)
	template.Response.States = nil
	result = ParseTemplateResponse(map[string]any{"status": "failed", "url": "https://example.org/output.mp4", "error": map[string]any{"message": "rejected"}}, cap)
	require.Equal(t, "failed", result.Status)
	require.Empty(t, result.Artifacts)
	path, err := expandTemplateEndpoint(TemplateEndpoint{Path: "/jobs/{task_id}", Query: map[string]string{"model": "{model}"}}, map[string]string{"task_id": "a/b?c", "model": "a&b"})
	require.NoError(t, err)
	require.Equal(t, "/jobs/a%2Fb%3Fc?model=a%26b", path)
}
func TestTemplateBindingPriorityAndNoNameGuess(t *testing.T) {
	cap := Capability{Model: "public", UpstreamModel: "original", Version: "1", MediaType: "video", Operations: []string{"video"}, GroupIDs: []int64{9}, AccountIDs: []int64{4}, Enabled: true, TemplateRef: &TemplateRef{ID: "video-arrays", Version: 1}}
	bindings := []ModelBinding{{ID: "default", Version: "1", Status: "enabled", GroupID: 9, Rules: []ParameterRule{{Phase: "semantic", Op: "default", Target: "seconds", Value: 8}}}, {ID: "exact", Version: "2", Status: "enabled", GroupID: 9, Model: "public", AccountID: 4, UpstreamModel: "renamed", Template: &TemplateRef{ID: "video-reference-arrays", Version: 1}}}
	load := func(ref TemplateRef) (ProtocolTemplate, error) { return templateFixture(t, ref.ID), nil }
	resolved, err := ResolveTemplateBindings([]Capability{cap}, bindings, load)
	require.NoError(t, err)
	require.Equal(t, "renamed", resolved[0].UpstreamModel)
	require.Equal(t, "video-reference-arrays", resolved[0].Template.ID)
	require.Len(t, resolved[0].BindingVersions, 2)
	changed := cap
	changed.Model = "TT-or-Grok-or-Veo"
	changed.TemplateRef = nil
	resolved, err = ResolveTemplateBindings([]Capability{changed}, nil, load)
	require.NoError(t, err)
	require.False(t, resolved[0].Enabled)
	require.Nil(t, resolved[0].Template)
	bindings = append(bindings, bindings[0])
	_, err = ResolveTemplateBindings([]Capability{cap}, bindings, load)
	require.ErrorContains(t, err, "binding_conflict")
}

func TestTemplateBindingResolutionIsolatesInvalidModelBinding(t *testing.T) {
	bad := Capability{ID: "bad-cap", Model: "bad-model", Version: "1", MediaType: "video", Operations: []string{"video"}, GroupIDs: []int64{9}, AccountIDs: []int64{4}, Enabled: true, Status: "enabled", TemplateRef: &TemplateRef{ID: "video-arrays", Version: 1}}
	good := bad
	good.ID, good.Model = "good-cap", "good-model"
	bindings := []ModelBinding{{
		ID: "broken-binding", Version: "2", Status: "enabled", GroupID: 9, Model: bad.Model, AccountID: 4,
		Rules: []ParameterRule{{Phase: "wire", Op: "rename", Source: "payload", Target: "payload.nested"}},
	}}
	load := func(ref TemplateRef) (ProtocolTemplate, error) { return templateFixture(t, ref.ID), nil }

	resolved, issues, err := ResolveTemplateBindingsIsolated([]Capability{bad, good}, bindings, load)
	require.NoError(t, err)
	require.Len(t, resolved, 2)
	require.False(t, resolved[0].Enabled)
	require.Contains(t, resolved[0].ValidationStatus, "协议绑定配置错误")
	require.True(t, resolved[1].Enabled)
	require.NotNil(t, resolved[1].Template)
	require.Len(t, issues, 1)
	require.Equal(t, int64(9), issues[0].GroupID)
	require.Equal(t, int64(4), issues[0].AccountID)
	require.Equal(t, "bad-model", issues[0].Model)
	require.Equal(t, []string{"broken-binding"}, issues[0].BindingIDs)
	require.ErrorContains(t, issues[0].Err, "parameter_rule_conflict")

	price := testCard(baseCharge("second", "request_duration", "0.04"))
	price.ModelMatch.Value = good.Model
	price.Scope.Reference = "9"
	engine := &Engine{Config: func(context.Context, int64) ([]Capability, []PriceCard, error) {
		return resolved, []PriceCard{price}, nil
	}}
	catalog, err := engine.Catalog(context.Background(), 9)
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.Equal(t, good.Model, catalog[0].Model)
}

func TestTemplateResolutionRequiresPersistedReferenceOrAdminBinding(t *testing.T) {
	load := func(ref TemplateRef) (ProtocolTemplate, error) { return templateFixture(t, ref.ID), nil }
	for _, cap := range []Capability{
		{ID: "adapter-only", Model: "grok-imagine-video-1.5", UpstreamModel: "grok-imagine-video-1.5", Adapter: "yunfei_grok", MediaType: "video", Operations: []string{"video"}, GroupIDs: []int64{9}, AccountIDs: []int64{4}, Enabled: true},
		{ID: "format-only", Model: "XXsedanco2.5S", UpstreamModel: "XXsedanco2.5S", RequestFormat: "v6k_url", MediaType: "video", Operations: []string{"video"}, GroupIDs: []int64{9}, AccountIDs: []int64{4}, Enabled: true},
	} {
		resolved, err := ResolveTemplateBindings([]Capability{cap}, nil, load)
		require.NoError(t, err)
		require.Len(t, resolved, 1)
		require.False(t, resolved[0].Enabled)
		require.Nil(t, resolved[0].Template)
		require.Equal(t, "待配置协议模板", resolved[0].ValidationStatus)
	}

	cap := Capability{ID: "explicit-binding", Model: "unrecognized-model-name", UpstreamModel: "renamed", MediaType: "video", Operations: []string{"video"}, GroupIDs: []int64{9}, AccountIDs: []int64{4}, Enabled: true}
	bindings := []ModelBinding{{ID: "admin-selected", Version: "3", Status: "enabled", GroupID: 9, Model: cap.Model, AccountID: 4, Template: &TemplateRef{ID: "video-reference-arrays", Version: 1}}}
	resolved, err := ResolveTemplateBindings([]Capability{cap}, bindings, load)
	require.NoError(t, err)
	require.True(t, resolved[0].Enabled)
	require.Equal(t, "video-reference-arrays", resolved[0].Template.ID)
}

func TestTemplateUpgradeRejectsBindingsOutsidePublishedCapability(t *testing.T) {
	template := ProtocolTemplate{MediaTypes: []string{"video"}, Operations: []string{"video"}}
	binding := ModelBinding{GroupID: 9, Model: "public-video", AccountID: 4}
	capabilities := []Capability{{
		Model: "public-video", MediaType: "video", Operations: []string{"video"},
		GroupIDs: []int64{9}, AccountIDs: []int64{4}, Enabled: true, Status: "enabled",
	}, {
		Model: "other-model", MediaType: "image", Operations: []string{"image"},
		GroupIDs: []int64{9}, AccountIDs: []int64{4}, Enabled: true, Status: "enabled",
	}}
	require.NoError(t, validateTemplateBindingCompatibility(template, binding, capabilities))

	unsupportedMedia := capabilities[0]
	unsupportedMedia.MediaType = "audio"
	require.ErrorContains(t, validateTemplateBindingCompatibility(template, binding, []Capability{unsupportedMedia}), "binding_template_media_conflict")

	unsupportedOperation := capabilities[0]
	unsupportedOperation.Operations = []string{"speech"}
	require.ErrorContains(t, validateTemplateBindingCompatibility(template, binding, []Capability{unsupportedOperation}), "binding_template_operation_conflict")

	binding.AccountID = 5
	require.ErrorContains(t, validateTemplateBindingCompatibility(template, binding, capabilities), "binding_capability_missing")
}

func TestTemplateUpgradeValidatesAccountModeCapabilities(t *testing.T) {
	template := ProtocolTemplate{MediaTypes: []string{"video"}, Operations: []string{"video"}}
	binding := ModelBinding{GroupID: 9, Model: "public-video", AccountID: 4}
	runtimeCapability := Capability{
		Model: "public-video", MediaType: "video", Operations: []string{"video"},
		GroupIDs: []int64{9}, AccountIDs: []int64{4}, Enabled: true, Status: "enabled",
	}

	store := &Store{RuntimeCapabilities: func(_ context.Context, groupID int64) ([]Capability, error) {
		require.EqualValues(t, binding.GroupID, groupID)
		return []Capability{runtimeCapability}, nil
	}}
	require.NoError(t, store.validateTemplateBindingCompatibility(context.Background(), template, binding, nil))

	for _, tc := range []struct {
		name string
		caps []Capability
	}{
		{name: "missing model"},
		{name: "disabled model", caps: []Capability{func() Capability {
			capability := runtimeCapability
			capability.Enabled = false
			return capability
		}()}},
		{name: "different group", caps: []Capability{func() Capability {
			capability := runtimeCapability
			capability.GroupIDs = []int64{10}
			return capability
		}()}},
		{name: "different model", caps: []Capability{func() Capability {
			capability := runtimeCapability
			capability.Model = "other-model"
			return capability
		}()}},
		{name: "different account", caps: []Capability{func() Capability {
			capability := runtimeCapability
			capability.AccountIDs = []int64{5}
			return capability
		}()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store.RuntimeCapabilities = func(context.Context, int64) ([]Capability, error) { return tc.caps, nil }
			require.ErrorContains(t, store.validateTemplateBindingCompatibility(context.Background(), template, binding, nil), "binding_capability_missing")
		})
	}
}
