package media

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Submission is a receipt of the adapter's outgoing body, never the input draft.
// It is stored in the encrypted task blob, not in task SQL or application logs.
type Submission struct {
	Stage       string         `json:"stage"`
	Path        string         `json:"path"`
	RequestHash string         `json:"request_hash"`
	Request     any            `json:"request"`
	References  map[string]int `json:"references"`
	Response    any            `json:"response,omitempty"`
}

func NewSubmission(path string, raw []byte, body map[string]any, refs map[string][]any, key string) Submission {
	sum := sha256.Sum256(raw)
	counts := map[string]int{}
	for _, kind := range []string{"image", "video", "audio"} {
		counts[kind] = len(refs[kind])
	}
	return Submission{Stage: "prepared", Path: path, RequestHash: hex.EncodeToString(sum[:]), Request: SafeReviewValue(body, []string{key}, 0), References: counts}
}

var sensitiveReviewKey = regexp.MustCompile(`(?i)secret|token|password|authorization|cookie|api.?key|b64|base64|credential|private.?key`)
var inlineReviewURL = regexp.MustCompile(`https?://[^\s"<>]+`)
var inlineReviewCredential = regexp.MustCompile(`(?i)Bearer\s+\S+|\bsk-[A-Za-z0-9_-]+`)

// Extensions stay visible; sensitive/encoded fields are explicitly marked rather than hidden.
func SafeReviewValue(value any, secrets []string, depth int) any {
	if depth > 12 {
		return "[nested content omitted]"
	}
	switch v := value.(type) {
	case string:
		for _, secret := range secrets {
			if secret != "" {
				v = strings.ReplaceAll(v, secret, "[credential omitted]")
			}
		}
		if strings.HasPrefix(v, "data:") || len(v) > 4096 {
			return "[encoded or long content omitted]"
		}
		if len(v) >= 64 {
			if decoded, err := base64.StdEncoding.DecodeString(v); err == nil && !strings.HasPrefix(http.DetectContentType(decoded), "text/plain") {
				return "[encoded content omitted]"
			}
		}
		if strings.Contains(v, ":\\") || (strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "/v1/")) {
			return "[internal path omitted]"
		}
		v = inlineReviewCredential.ReplaceAllString(v, "[credential omitted]")
		return inlineReviewURL.ReplaceAllStringFunc(v, func(raw string) string {
			u, err := url.Parse(raw)
			if err != nil {
				return "[location omitted]"
			}
			u.User = nil
			u.RawQuery = ""
			u.Fragment = ""
			return u.String()
		})
	case map[string]any:
		clean := map[string]any{}
		count := 0
		for k, item := range v {
			count++
			if count > 200 {
				clean["_omitted"] = "additional fields omitted"
				break
			}
			if sensitiveReviewKey.MatchString(k) {
				clean[k] = "[redacted]"
			} else {
				clean[k] = SafeReviewValue(item, secrets, depth+1)
			}
		}
		return clean
	case []any:
		out := []any{}
		for i, item := range v {
			if i >= 100 {
				out = append(out, "[additional entries omitted]")
				break
			}
			out = append(out, SafeReviewValue(item, secrets, depth+1))
		}
		return out
	default:
		// Typed structs/maps become JSON before the recursive redaction boundary.
		switch value.(type) {
		case nil, bool, float64, json.Number, int, int64:
			return value
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		var decoded any
		if json.Unmarshal(raw, &decoded) != nil {
			return nil
		}
		return SafeReviewValue(decoded, secrets, depth+1)
	}
}

// Historical tasks have no receipt; never infer outgoing bytes from their input snapshot.
func (e *Engine) ReadSubmission(id string) *Submission {
	if e.Blobs == nil || !strings.HasPrefix(id, "mt_") || strings.ContainsAny(id, "/\\.\x00") {
		return nil
	}
	var receipt Submission
	if e.Blobs.ReadJSON(filepath.Join(e.Blobs.Root, id, "submission"), &receipt) != nil {
		return nil
	}
	return &receipt
}
