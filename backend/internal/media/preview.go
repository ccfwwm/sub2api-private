package media

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"sort"
	"strings"
)

type PreviewMeasurement struct {
	Kind   string `json:"kind"`
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

type PreviewInput struct {
	Kind        string `json:"kind"`
	Index       int    `json:"index"`
	Role        any    `json:"role,omitempty"`
	Source      string `json:"source"`
	URL         any    `json:"url,omitempty"`
	SourceHash  string `json:"source_sha256,omitempty"`
	MIME        string `json:"mime,omitempty"`
	Bytes       *int   `json:"bytes,omitempty"`
	ContentHash string `json:"content_sha256,omitempty"`
	Measurement string `json:"measurement"`
}

// PreviewInputs describes each original reference without reading remote URLs.
func PreviewInputs(req Request) ([]PreviewInput, error) {
	out := []PreviewInput{}
	for _, kind := range []string{"image", "video", "audio", "mask"} {
		for index, ref := range req.MaterialRefs[kind] {
			raw, _, err := refString(ref)
			if err != nil {
				return nil, err
			}
			item := PreviewInput{Kind: kind, Index: index, Source: "inline", Measurement: "decoded"}
			if object, ok := ref.(map[string]any); ok {
				item.Role = SafeReviewValue(object["role"], nil, 0)
			}
			if strings.HasPrefix(raw, "https://") {
				item.Source, item.Measurement = "url", "not_fetched"
				item.URL = SafeReviewValue(raw, nil, 0)
				item.SourceHash = fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
			} else {
				mime, data, err := decodeReferenceMaterial(raw, referenceMIME(ref))
				if err != nil {
					return nil, err
				}
				size := len(data)
				item.MIME, item.Bytes, item.ContentHash = mime, &size, fmt.Sprintf("%x", sha256.Sum256(data))
			}
			out = append(out, item)
		}
	}
	return out, nil
}

type EffectivePreview struct {
	Mode                string               `json:"mode"`
	Normalized          any                  `json:"normalized"`
	Effective           map[string]any       `json:"effective"`
	RuleProvenance      any                  `json:"rule_provenance"`
	OverrideDiff        any                  `json:"override_diff"`
	Inputs              []PreviewInput       `json:"inputs"`
	Quantities          map[string][]string  `json:"quantities"`
	Pricing             any                  `json:"pricing,omitempty"`
	MeasurementRequired []PreviewMeasurement `json:"measurement_required"`
	Request             *Submission          `json:"request,omitempty"`
	RequestComplete     bool                 `json:"request_complete"`
	OnlineMediaGET      bool                 `json:"online_media_get"`
	PaidSubmission      bool                 `json:"paid_submission"`
}

type offlinePreviewTransport struct{}

func (offlinePreviewTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fail(422, "online_measurement_required", "Preview has not authorized online media GET")
}

type previewRemoteMetadata struct {
	MIME     string
	Bytes    int
	Hash     string
	Complete bool
}
type previewMeasurementTransport struct {
	base         http.RoundTripper
	observations map[string]*previewRemoteMetadata
}

func (p *previewMeasurementTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return nil, fail(422, "preview_method_forbidden", "Preview permits only explicit media GET requests")
	}
	response, err := p.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	metadata := &previewRemoteMetadata{}
	// Associate the validated final body with the original reference as well as
	// every redirect URL. The client's existing redirect policy remains in use.
	for previous := req; previous != nil; {
		p.observations[fmt.Sprintf("%x", sha256.Sum256([]byte(previous.URL.String())))] = metadata
		if previous.Response == nil {
			break
		}
		previous = previous.Response.Request
	}
	response.Body = &previewMeasuredBody{ReadCloser: response.Body, sum: sha256.New(), metadata: metadata}
	return response, nil
}

type previewMeasuredBody struct {
	io.ReadCloser
	sum      hash.Hash
	prefix   []byte
	metadata *previewRemoteMetadata
}

func (b *previewMeasuredBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		_, _ = b.sum.Write(p[:n])
		b.metadata.Bytes += n
		if len(b.prefix) < 512 {
			b.prefix = append(b.prefix, p[:min(n, 512-len(b.prefix))]...)
		}
	}
	if err == io.EOF {
		b.metadata.Complete = true
		b.metadata.Hash = fmt.Sprintf("%x", b.sum.Sum(nil))
		b.metadata.MIME = http.DetectContentType(b.prefix)
	}
	return n, err
}

// PreviewEffective uses submission normalization, measurement and pricing, but
// never reserves funds, creates a task, or invokes an upstream generation API.
func (e *Engine) PreviewEffective(ctx context.Context, operation string, input map[string]any, identity Identity, accountID int64, measure bool) (EffectivePreview, error) {
	out := EffectivePreview{Mode: "effective", MeasurementRequired: []PreviewMeasurement{}, OnlineMediaGET: measure}
	cap, err := e.SelectCapability(ctx, stringValue(input["model"]), operation, input, nil, identity)
	if err != nil {
		return out, err
	}
	if cap == nil || cap.Template == nil {
		return out, fail(422, "media_not_configured", "Publish a matching protocol template binding")
	}
	if accountID > 0 {
		if !containsInt64(cap.AccountIDs, accountID) {
			return out, fail(409, "preview_account_not_candidate", "Requested account is not a candidate for the effective capability")
		}
		cap.AccountIDs = []int64{accountID}
	}
	var account Account
	for _, id := range cap.AccountIDs {
		a, lookupErr := e.Lookup(id, identity.GroupID)
		if lookupErr == nil && a.Allowed {
			account = a
			break
		}
	}
	if account.ID == 0 {
		return out, fail(503, "channel_unavailable", "No configured account is currently eligible")
	}
	// Pricing must resolve against the account whose request is being previewed.
	cap.AccountIDs = []int64{account.ID}
	req, err := NormalizeUploads(*cap, operation, input, nil)
	if err != nil {
		return out, err
	}
	out.Inputs, err = PreviewInputs(req)
	if err != nil {
		return out, err
	}
	if !measure {
		for _, material := range out.Inputs {
			if material.Source == "url" {
				out.MeasurementRequired = append(out.MeasurementRequired, PreviewMeasurement{Kind: material.Kind, Index: material.Index, Reason: "remote_media_validation"})
			}
		}
	}
	if len(out.MeasurementRequired) == 0 {
		preparer := &Engine{Blobs: e.Blobs}
		observations := map[string]*previewRemoteMetadata{}
		if !measure {
			// Fail closed even if a future serializer unexpectedly requests a URL.
			preparer.Adapter = &Adapter{Client: &http.Client{Transport: offlinePreviewTransport{}}}
		} else {
			if e.Adapter == nil || e.Adapter.Client == nil {
				return out, fail(503, "preview_transport_missing", "Media validation transport is unavailable")
			}
			client := *e.Adapter.Client
			transport := client.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			client.Transport = &previewMeasurementTransport{base: transport, observations: observations}
			preparer.Adapter = &Adapter{Client: &client}
		}
		req, err = preparer.PrepareRequest(ctx, *cap, operation, input, nil)
		if err != nil {
			return out, err
		}
		for index, item := range out.Inputs {
			if metadata := observations[item.SourceHash]; metadata != nil && metadata.Complete {
				size := metadata.Bytes
				out.Inputs[index].MIME, out.Inputs[index].Bytes, out.Inputs[index].ContentHash = metadata.MIME, &size, metadata.Hash
				out.Inputs[index].Measurement = "validated_remote"
			}
		}
	} else {
		wire, buildErr := BuildTemplateRequest(ctx, nil, *cap, req)
		if buildErr == nil {
			req.Wire = &wire
		} else if mediaErr, ok := buildErr.(*Error); !ok || mediaErr.Code != "online_measurement_required" {
			return out, buildErr
		}
	}
	out.Normalized = SafeReviewValue(req.Parameters, []string{account.Key}, 0)
	out.Quantities = req.Quantities
	out.Effective = map[string]any{"group_id": identity.GroupID, "profile_id": cap.ID, "configuration_version": cap.Version, "model": cap.Model, "upstream_model": cap.UpstreamModel, "account_id": account.ID, "template_ref": cap.TemplateRef, "binding_versions": cap.BindingVersions, "rules": SafeReviewValue(mergeRuleLayers(append(append([]ParameterRule{}, cap.Template.Rules...), cap.TemplateRules...)), []string{account.Key}, 0)}
	out.RuleProvenance, err = e.previewRuleProvenance(ctx, *cap, identity.GroupID)
	if err != nil {
		return out, err
	}
	out.RuleProvenance = SafeReviewValue(out.RuleProvenance, []string{account.Key}, 0)
	diff := previewDiff("semantic", input, req.Parameters)
	if req.Wire != nil {
		raw, _ := json.Marshal(req.Wire.Body)
		receipt := NewSubmission(req.Wire.Path, raw, req.Wire.Body, req.MaterialRefs, account.Key)
		out.Request = &receipt
		diff = append(diff, previewDiff("wire", req.Parameters, req.Wire.Body)...)
	}
	out.OverrideDiff = SafeReviewValue(diff, []string{account.Key}, 0)
	out.RequestComplete = len(out.MeasurementRequired) == 0 && out.Request != nil
	if len(out.MeasurementRequired) == 0 {
		snapshot, selectedAccount, calculation, snapshotErr := e.Snapshot(ctx, *cap, req, identity)
		if snapshotErr != nil {
			return out, snapshotErr
		}
		out.Effective["account_id"] = selectedAccount
		out.Pricing = map[string]any{"currency": "USD", "calculation": calculation, "maximum_charge": snapshot.Maximum, "price_version": snapshot.Sale.Version, "locks_price": false}
	}
	return out, nil
}

func (e *Engine) previewRuleProvenance(ctx context.Context, cap Capability, groupID int64) ([]map[string]any, error) {
	layers := []map[string]any{{"source": "template", "reference": cap.TemplateRef, "rules": cap.Template.Rules}}
	var raw []Capability
	var err error
	if e.Config != nil {
		raw, _, err = e.Config(ctx, groupID)
	} else if e.Store != nil {
		raw, _, err = e.Store.Published(ctx)
	}
	if err != nil {
		return nil, err
	}
	for _, base := range raw {
		if base.Model == cap.Model && (base.ID == cap.ID || strings.HasPrefix(cap.ID, base.ID+":g")) {
			layers = append(layers, map[string]any{"source": "account_model", "profile_id": base.ID, "rules": base.TemplateRules, "field_mapping": base.FieldMapping})
		}
	}
	for _, ref := range cap.BindingVersions {
		if e.Store == nil {
			return nil, fail(503, "preview_configuration_missing", "Binding history is unavailable")
		}
		record, err := e.Store.ConfigVersion(ctx, "binding", ref.ID, ref.Version)
		if err != nil {
			return nil, err
		}
		var binding ModelBinding
		if err := json.Unmarshal(record.Config, &binding); err != nil {
			return nil, err
		}
		scope := "group_default"
		if binding.Model != "" {
			scope = "group_model"
		}
		if binding.AccountID > 0 {
			scope = "group_model_account"
		}
		layers = append(layers, map[string]any{"source": "binding", "scope": scope, "reference": ref, "rules": binding.Rules, "template_ref": binding.Template, "upstream_model": binding.UpstreamModel})
	}
	return layers, nil
}

func previewDiff(phase string, before, after map[string]any) []any {
	keys := map[string]bool{}
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	out := []any{}
	for _, key := range ordered {
		old, oldPresent := before[key]
		next, nextPresent := after[key]
		oldRaw, _ := json.Marshal(old)
		nextRaw, _ := json.Marshal(next)
		if oldPresent != nextPresent || string(oldRaw) != string(nextRaw) {
			out = append(out, map[string]any{"phase": phase, "path": key, "before_present": oldPresent, "after_present": nextPresent, "before": old, "after": next})
		}
	}
	return out
}
