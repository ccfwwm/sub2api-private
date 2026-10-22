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
	Stage               string         `json:"stage"`
	Method              string         `json:"method,omitempty"`
	Path                string         `json:"path"`
	Encoding            string         `json:"encoding,omitempty"`
	RequestHash         string         `json:"request_hash"`
	Request             any            `json:"request"`
	Files               any            `json:"files,omitempty"`
	References          map[string]int `json:"references"`
	ResponseStatusCode  int            `json:"response_status_code,omitempty"`
	ResponseContentType string         `json:"response_content_type,omitempty"`
	Response            any            `json:"response,omitempty"`
}

func NewSubmission(path string, raw []byte, body map[string]any, refs map[string][]any, key string) Submission {
	sum := sha256.Sum256(raw)
	counts := map[string]int{}
	for _, kind := range []string{"image", "video", "audio"} {
		counts[kind] = len(refs[kind])
	}
	return Submission{Stage: "prepared", Method: http.MethodPost, Path: path, Encoding: "json", RequestHash: hex.EncodeToString(sum[:]), Request: SafeReviewValue(body, []string{key}, 0), References: counts}
}

// NewAdminSubmission stores the actual serialized body in the encrypted
// receipt for administrator debugging. Public previews continue to use
// NewSubmission and never expose inline media payloads.
func NewAdminSubmission(path string, raw []byte, body map[string]any, refs map[string][]any, key string) Submission {
	receipt := NewSubmission(path, raw, body, refs, key)
	receipt.Request = SafeAdminReviewValue(body, []string{key}, 0)
	return receipt
}

func NewAdminSubmissionWithFiles(path string, raw []byte, body map[string]any, files []Upload, refs map[string][]any, key string) Submission {
	receipt := NewAdminSubmission(path, raw, body, refs, key)
	if len(files) > 0 {
		receipt.Files = SafeAdminReviewValue(files, []string{key}, 0)
	}
	return receipt
}

var sensitiveReviewKey = regexp.MustCompile(`(?i)secret|token|password|authorization|cookie|api.?key|b64|base64|credential|private.?key`)
var inlineReviewURL = regexp.MustCompile(`https?://[^\s"<>]+`)
var inlineReviewCredential = regexp.MustCompile(`(?i)Bearer\s+\S+|\bsk-[A-Za-z0-9_-]+`)
var sensitiveAdminReviewKey = regexp.MustCompile(`(?i)(?:^|[_-])(?:secret|token|password|authorization|cookie|api[_-]?key|credential|private[_-]?key)(?:$|[_-])|^(?:accessToken|refreshToken|apiKey|privateKey|clientSecret)$`)
var mediaPayloadReviewKey = regexp.MustCompile(`(?i)^(?:b64_json|base64|base64_data|data|image|images|video|videos|audio|audios|mask|masks|reference_images|reference_videos|reference_audios|image_url|video_url|audio_url|inlineData|inline_data)$`)

// Extensions stay visible; sensitive/encoded fields are explicitly marked rather than hidden.
func SafeReviewValue(value any, secrets []string, depth int) any {
	return safeReviewValue(value, secrets, depth, false)
}

// SafeAdminReviewValue preserves request and response media payloads so an
// administrator can reproduce a call, including signed media URLs. Account
// credentials remain protected; ordinary history must use SafeReviewValue.
func SafeAdminReviewValue(value any, secrets []string, depth int) any {
	return safeReviewValue(value, secrets, depth, true)
}

func safeReviewValue(value any, secrets []string, depth int, preserveMedia bool) any {
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
		if !preserveMedia && (strings.HasPrefix(v, "data:") || len(v) > 4096) {
			return "[encoded or long content omitted]"
		}
		if !preserveMedia && len(v) >= 64 {
			if decoded, err := base64.StdEncoding.DecodeString(v); err == nil && !strings.HasPrefix(http.DetectContentType(decoded), "text/plain") {
				return "[encoded content omitted]"
			}
		}
		// Preserve inline media data for administrator debugging, but never expose
		// filesystem paths from provider extensions or echoed request fields.
		if preserveMedia && len(v) >= 64 {
			if decoded, err := base64.StdEncoding.DecodeString(v); err == nil && strings.HasPrefix(http.DetectContentType(decoded), "image/") {
				return v // JPEG Base64 commonly begins with '/', which is not a local path.
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
			u.Fragment = ""
			if !preserveMedia {
				u.RawQuery = ""
			}
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
			sensitive := sensitiveReviewKey.MatchString(k)
			if preserveMedia {
				sensitive = sensitiveAdminReviewKey.MatchString(k) && !mediaPayloadReviewKey.MatchString(k)
			}
			if sensitive {
				clean[k] = "[redacted]"
			} else {
				clean[k] = safeReviewValue(item, secrets, depth+1, preserveMedia)
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
			out = append(out, safeReviewValue(item, secrets, depth+1, preserveMedia))
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
		return safeReviewValue(decoded, secrets, depth+1, preserveMedia)
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
