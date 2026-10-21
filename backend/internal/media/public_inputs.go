package media

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

func publicDiagnostic(t StoredTask) string {
	text := t.Diagnostic
	if model := t.Snapshot.Capability.UpstreamModel; model != "" && model != t.Snapshot.Capability.Model {
		text = strings.ReplaceAll(text, model, t.Snapshot.Capability.Model)
	}
	return inlineReviewURL.ReplaceAllString(text, "[upstream-url]")
}

func safeArtifactFilename(name string) string {
	parts := strings.Split(strings.ReplaceAll(name, "\\", "/"), "/")
	name = parts[len(parts)-1]
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "." || name == ".." || len(name) > 240 {
		return ""
	}
	return name
}

type PublicInputSummary struct {
	Stage      string         `json:"stage"`
	References map[string]int `json:"references"`
	Hash       string         `json:"input_hash"`
}

func (e *Engine) PublicInputs(t StoredTask) *PublicInputSummary {
	if e.Blobs == nil {
		return nil
	}
	var req Request
	if e.Blobs.ReadJSON(stringValue(t.Snapshot.Request.Parameters["request_blob"]), &req) != nil {
		return nil
	}
	result := InputSummary(req)
	if receipt := e.ReadSubmission(t.ID); receipt != nil {
		result.Stage = receipt.Stage
	} else {
		result.Stage = "unrecorded"
	}
	return result
}
func InputSummary(req Request) *PublicInputSummary {
	counts := map[string]int{}
	for _, kind := range []string{"image", "video", "audio"} {
		counts[kind] = len(req.MaterialRefs[kind])
	}
	// Hash decoded bytes for encoded sources, so equivalent Base64 wrappers
	// describe the same input without exposing URLs or encoded media.
	canonical := map[string][]any{}
	for kind, refs := range req.MaterialRefs {
		for _, ref := range refs {
			raw, _, err := refString(ref)
			if err != nil {
				continue
			}
			item := map[string]any{}
			if obj, ok := ref.(map[string]any); ok {
				for k, v := range obj {
					if k != "url" && k != "b64_json" {
						item[k] = v
					}
				}
			}
			if strings.HasPrefix(raw, "https://") {
				item["source_hash"] = configurationDigest(raw)
			} else if mime, data, e := decodeMaterial(raw); e == nil {
				item["mime"] = mime
				item["content_hash"] = fmt.Sprintf("%x", sha256.Sum256(data))
			}
			canonical[kind] = append(canonical[kind], item)
		}
	}
	return &PublicInputSummary{Stage: "prepared", References: counts, Hash: configurationDigest(canonical)}
}
