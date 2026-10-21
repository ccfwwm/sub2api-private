package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type previewTransport func(*http.Request) (*http.Response, error)

func (f previewTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func effectivePreviewFixture(t *testing.T) (*Engine, Capability, Identity) {
	t.Helper()
	template := templateFixture(t, "video-arrays")
	template.Rules = []ParameterRule{{Phase: "semantic", Op: "default", Target: "flag", Value: true}, {Phase: "wire", Op: "rename", Source: "seconds", Target: "duration"}}
	capability := Capability{ID: "preview-cap", Version: "1", Model: "public-video", UpstreamModel: "private-model", Status: "enabled", Enabled: true, MediaType: "video", Operations: []string{"video"}, AccountIDs: []int64{9}, GroupIDs: []int64{1}, Template: &template, TemplateRef: &TemplateRef{ID: template.ID, Version: 1}, TemplateRules: []ParameterRule{{Phase: "semantic", Op: "fixed", Target: "flag", Value: false}}}
	card := testCard(baseCharge("second", "request_duration", "0.1"))
	card.ModelMatch.Value = capability.Model
	blobs, err := NewBlobStore(t.TempDir(), strings.Repeat("s", 32))
	require.NoError(t, err)
	engine := &Engine{Blobs: blobs, Adapter: &Adapter{Client: &http.Client{Transport: previewTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("offline preview made a network request")
		return nil, nil
	})}}, Lookup: func(id, group int64) (Account, error) {
		require.EqualValues(t, 9, id)
		require.EqualValues(t, 1, group)
		return Account{ID: id, Allowed: true, Key: "fixture-private-secret", CostMultiplier: "1"}, nil
	}}
	engine.Config = func(context.Context, int64) ([]Capability, []PriceCard, error) {
		return []Capability{capability}, []PriceCard{card}, nil
	}
	return engine, capability, Identity{UserID: 2, KeyID: 3, GroupID: 1, Multiplier: "2", TimeMultiplier: "1"}
}

func TestEffectivePreviewMatchesSubmissionCompilerAndQuoteWithoutTaskWrites(t *testing.T) {
	engine, cap, identity := effectivePreviewFixture(t)
	rawImage := pngFixture(t)
	input := map[string]any{"model": cap.Model, "seconds": 5, "prompt": "animate", "nullable": nil, "zero": 0, "images": []any{"data:image/png;base64," + base64.StdEncoding.EncodeToString(rawImage)}}
	preview, err := engine.PreviewEffective(context.Background(), "video", input, identity, 9, false)
	require.NoError(t, err)
	require.True(t, preview.RequestComplete)
	require.False(t, preview.PaidSubmission)
	require.False(t, preview.OnlineMediaGET)
	require.Empty(t, preview.MeasurementRequired)
	require.Equal(t, "prepared", preview.Request.Stage)
	require.EqualValues(t, 9, preview.Effective["account_id"])
	normalized, ok := preview.Normalized.(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, normalized["flag"])
	req, err := engine.PrepareRequest(context.Background(), cap, "video", input, nil)
	require.NoError(t, err)
	_, _, quote, err := engine.Snapshot(context.Background(), cap, req, identity)
	require.NoError(t, err)
	pricing, ok := preview.Pricing.(map[string]any)
	require.True(t, ok)
	require.Equal(t, quote, pricing["calculation"])
	require.Equal(t, "1.00000000", pricing["maximum_charge"])
	rawWire, err := json.Marshal(req.Wire.Body)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(rawWire)), preview.Request.RequestHash)
	require.Len(t, preview.Inputs, 1)
	require.Equal(t, "image/png", preview.Inputs[0].MIME)
	require.Equal(t, len(rawImage), *preview.Inputs[0].Bytes)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(rawImage)), preview.Inputs[0].ContentHash)
	encoded, err := json.Marshal(preview)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "fixture-private-secret")
	require.NotContains(t, string(encoded), base64.StdEncoding.EncodeToString(rawImage))
	require.Contains(t, string(encoded), "account_model")
	require.Contains(t, string(encoded), "override_diff")
	// Store is nil: any attempt to reserve or create a task would fail this test.
}

func TestEffectivePreviewRemoteInputsRequireExplicitMeasurement(t *testing.T) {
	for _, encoding := range []string{"preserve", "data_url"} {
		t.Run(encoding, func(t *testing.T) {
			engine, cap, identity := effectivePreviewFixture(t)
			for i := range cap.Template.Materials {
				cap.Template.Materials[i].Encoding = encoding
			}
			input := map[string]any{"model": cap.Model, "seconds": 5, "images": []any{map[string]any{"url": "https://example.org/reference.png?token=private", "role": "first_frame"}}}
			preview, err := engine.PreviewEffective(context.Background(), "video", input, identity, 0, false)
			require.NoError(t, err)
			require.Len(t, preview.MeasurementRequired, 1)
			require.Nil(t, preview.Pricing)
			require.False(t, preview.RequestComplete)
			require.Equal(t, "not_fetched", preview.Inputs[0].Measurement)
			require.Equal(t, "first_frame", preview.Inputs[0].Role)
			require.Empty(t, preview.Inputs[0].MIME)
			require.Nil(t, preview.Inputs[0].Bytes)
			require.NotEmpty(t, preview.Inputs[0].SourceHash)
			if encoding == "preserve" {
				require.NotNil(t, preview.Request)
			} else {
				require.Nil(t, preview.Request)
			}
			body, _ := json.Marshal(preview)
			require.NotContains(t, string(body), "token=private")
		})
	}
}

func TestEffectivePreviewOnlineMeasurementMakesOnlyMaterialGets(t *testing.T) {
	engine, cap, identity := effectivePreviewFixture(t)
	gets := 0
	image := pngFixture(t)
	engine.Adapter.Client = &http.Client{Transport: previewTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "example.org", r.URL.Host)
		require.Equal(t, "/reference.png", r.URL.Path)
		gets++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(image)), Request: r}, nil
	})}
	preview, err := engine.PreviewEffective(context.Background(), "video", map[string]any{"model": cap.Model, "seconds": 5, "images": []any{"https://example.org/reference.png"}}, identity, 9, true)
	require.NoError(t, err)
	require.True(t, preview.RequestComplete)
	require.True(t, preview.OnlineMediaGET)
	require.False(t, preview.PaidSubmission)
	require.Equal(t, 1, gets)
	require.NotNil(t, preview.Pricing)
	require.Len(t, preview.Inputs, 1)
	require.Equal(t, "validated_remote", preview.Inputs[0].Measurement)
	require.Equal(t, "image/png", preview.Inputs[0].MIME)
	require.Equal(t, len(image), *preview.Inputs[0].Bytes)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(image)), preview.Inputs[0].ContentHash)
}

func TestEffectivePreviewRedirectKeepsSourceEvidenceAndRedirectPolicy(t *testing.T) {
	engine, cap, identity := effectivePreviewFixture(t)
	image := pngFixture(t)
	redirects, gets := 0, 0
	engine.Adapter.Client = &http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			redirects++
			require.Len(t, via, 1)
			require.Equal(t, "cdn.example.org", r.URL.Host)
			return nil
		},
		Transport: previewTransport(func(r *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodGet, r.Method)
			gets++
			if r.URL.Host == "example.org" {
				return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://cdn.example.org/reference.png"}}, Body: io.NopCloser(strings.NewReader("redirect")), Request: r}, nil
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(image)), Request: r}, nil
		}),
	}
	preview, err := engine.PreviewEffective(context.Background(), "video", map[string]any{"model": cap.Model, "seconds": 5, "images": []any{"https://example.org/reference.png"}}, identity, 9, true)
	require.NoError(t, err)
	require.Equal(t, 1, redirects)
	require.Equal(t, 2, gets)
	require.True(t, preview.RequestComplete)
	require.Len(t, preview.Inputs, 1)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte("https://example.org/reference.png"))), preview.Inputs[0].SourceHash)
	require.Equal(t, "validated_remote", preview.Inputs[0].Measurement)
	require.Equal(t, "image/png", preview.Inputs[0].MIME)
	require.Equal(t, len(image), *preview.Inputs[0].Bytes)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(image)), preview.Inputs[0].ContentHash)
}

func TestEffectivePreviewTransportRejectsGenerationMethods(t *testing.T) {
	transport := &previewMeasurementTransport{base: previewTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("generation request reached network transport")
		return nil, nil
	}), observations: map[string]*previewRemoteMetadata{}}
	request, err := http.NewRequest(http.MethodPost, "https://example.org/v1/videos", nil)
	require.NoError(t, err)
	_, err = transport.RoundTrip(request)
	require.ErrorContains(t, err, "preview_method_forbidden")
}

func TestEffectivePreviewRejectsAccountOutsideSelectedCandidates(t *testing.T) {
	engine, cap, identity := effectivePreviewFixture(t)
	_, err := engine.PreviewEffective(context.Background(), "video", map[string]any{"model": cap.Model, "seconds": 5}, identity, 999, false)
	require.ErrorContains(t, err, "preview_account_not_candidate")
}

func TestPreviewRuleProvenanceReadsExactPinnedBindingHistory(t *testing.T) {
	engine, cap, identity := effectivePreviewFixture(t)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	engine.Store = &Store{DB: db}
	cap.BindingVersions = []TemplateRef{{ID: "group-binding", Version: 4}, {ID: "account-binding", Version: 7}}
	for index, ref := range cap.BindingVersions {
		binding := ModelBinding{ID: ref.ID, GroupID: identity.GroupID, Rules: []ParameterRule{{Phase: "semantic", Op: "fixed", Target: "quality", Value: "high"}}}
		if index == 1 {
			binding.Model, binding.AccountID = cap.Model, 9
		}
		raw, _ := json.Marshal(binding)
		mock.ExpectQuery(`SELECT id,version,NULL::bigint,status,config FROM media_config_versions WHERE kind=$1 AND id=$2 AND version=$3`).WithArgs("binding", ref.ID, ref.Version).WillReturnRows(sqlmock.NewRows([]string{"id", "version", "published_version", "status", "config"}).AddRow(ref.ID, ref.Version, nil, "enabled", raw))
	}
	layers, err := engine.previewRuleProvenance(context.Background(), cap, identity.GroupID)
	require.NoError(t, err)
	require.Equal(t, "group_default", layers[2]["scope"])
	require.Equal(t, "group_model_account", layers[3]["scope"])
	require.Equal(t, cap.BindingVersions[1], layers[3]["reference"])
	require.NoError(t, mock.ExpectationsWereMet())
}
