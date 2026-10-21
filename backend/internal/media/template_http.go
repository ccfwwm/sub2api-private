package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

func (a *Adapter) submitTemplate(ctx context.Context, cap Capability, req Request, account Account) (UpstreamResult, error) {
	wire := req.Wire
	if wire == nil {
		return UpstreamResult{}, fail(409, "prepared_request_missing", "A template submission requires the priced frozen request")
	}
	var body io.Reader
	contentType := "application/json"
	raw, err := json.Marshal(wire.Body)
	if err != nil {
		return UpstreamResult{}, err
	}
	switch wire.Encoding {
	case "multipart":
		buffer := &bytes.Buffer{}
		writer := multipart.NewWriter(buffer)
		for field, value := range wire.Body {
			values := []any{value}
			if list, ok := value.([]any); ok {
				values = list
			}
			for _, v := range values {
				text, ok := v.(string)
				if !ok {
					encoded, e := json.Marshal(v)
					if e != nil {
						return UpstreamResult{}, e
					}
					text = string(encoded)
				}
				if e := writer.WriteField(field, text); e != nil {
					return UpstreamResult{}, e
				}
			}
		}
		for _, file := range wire.Files {
			header := textproto.MIMEHeader{}
			header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, file.Field, file.Name))
			header.Set("Content-Type", file.MIME)
			part, e := writer.CreatePart(header)
			if e != nil {
				return UpstreamResult{}, e
			}
			if _, e = part.Write(file.Data); e != nil {
				return UpstreamResult{}, e
			}
		}
		if err = writer.Close(); err != nil {
			return UpstreamResult{}, err
		}
		contentType = writer.FormDataContentType()
		raw = buffer.Bytes()
	case "form":
		values := url.Values{}
		for field, value := range wire.Body {
			if list, ok := value.([]any); ok {
				for _, item := range list {
					values.Add(field, stringValue(item))
				}
			} else {
				values.Set(field, stringValue(value))
			}
		}
		raw = []byte(values.Encode())
		contentType = "application/x-www-form-urlencoded"
	}
	if len(raw) > 256<<20 {
		return UpstreamResult{}, fail(413, "request_too_large", "Encoded request exceeds transport limit")
	}
	if cap.Limits.MaxRequestBytes != nil && int64(len(raw)) > *cap.Limits.MaxRequestBytes {
		return UpstreamResult{}, fail(413, "request_too_large", "Encoded request exceeds configured limit")
	}
	body = bytes.NewReader(raw)
	if observe := account.ObserveSubmission; observe != nil {
		receipt := NewSubmission(wire.Path, raw, wire.Body, req.MaterialRefs, account.Key)
		if err = observe(receipt); err != nil {
			return UpstreamResult{}, err
		}
		account.ObserveSubmission = func(update Submission) error {
			receipt.Stage = update.Stage
			if update.Response != nil {
				receipt.Response = update.Response
			}
			return observe(receipt)
		}
	}
	return a.call(ctx, account, cap, wire.Method, wire.Path, body, contentType, true)
}

func (a *Adapter) pollTemplate(ctx context.Context, task StoredTask, account Account) (UpstreamResult, error) {
	cap := task.Snapshot.Capability
	t := cap.Template
	if t.Query == nil {
		return UpstreamResult{Status: "unknown", Diagnostic: "No read-only query configured; do not resubmit"}, nil
	}
	vars := map[string]string{"task_id": task.UpstreamID, "model": cap.UpstreamModel, "idempotency_key": account.IdempotencyKey}
	path, err := expandTemplateEndpoint(*t.Query, vars)
	if err != nil {
		return UpstreamResult{}, err
	}
	result, err := a.call(ctx, account, cap, t.Query.Method, path, nil, "", false)
	if err == nil && result.Status == "completed" && len(result.Artifacts) == 0 && t.ArtifactList != nil {
		path, err = expandTemplateEndpoint(*t.ArtifactList, vars)
		if err != nil {
			return result, err
		}
		result, err = a.call(ctx, account, cap, t.ArtifactList.Method, path, nil, "", false)
		if err == nil && result.Status != "failed" && len(result.Artifacts) > 0 {
			var object map[string]any
			_ = json.Unmarshal(result.Response, &object)
			explicit := false
			for _, p := range t.Response.Status {
				if len(extractValues(object, p)) > 0 {
					explicit = true
				}
			}
			if !explicit {
				result.Status = "completed"
				result.Accepted = true
			}
		}
	}
	return result, err
}

// Discovery never publishes models, creates tasks or alters pricing.
func (a *Adapter) DiscoverTemplate(ctx context.Context, t ProtocolTemplate, account Account) ([]string, error) {
	if t.Discovery == nil {
		return nil, fail(422, "discovery_not_configured", "No discovery endpoint configured")
	}
	path, err := expandTemplateEndpoint(*t.Discovery, map[string]string{})
	if err != nil {
		return nil, err
	}
	endpoint, err := originURL(account, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	applyTemplateAuth(req, t.Auth, account.Key)
	resp, err := a.Client.Do(protectMediaCredentials(req, account.Key))
	if err != nil {
		return nil, fmt.Errorf("model discovery unavailable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("discovery HTTP %d", resp.StatusCode)
	}
	var object map[string]any
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&object); err != nil {
		return nil, fmt.Errorf("invalid discovery response")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, p := range t.ModelIDs {
		for _, v := range extractValues(object, p) {
			s, ok := v.(string)
			if ok && s != "" && !strings.ContainsAny(s, "\r\n") && !seen[s] {
				out = append(out, s)
				seen[s] = true
			}
		}
	}
	return out, nil
}
