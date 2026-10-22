package media

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// CompactEvidence is a display projection only. Encrypted originals and exports
// retain the complete payload; long text prompts and signed URLs stay readable.
func CompactEvidence(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var decoded any
	if json.Unmarshal(raw, &decoded) != nil {
		return nil
	}
	return compactEvidence(decoded, "")
}

func compactEvidence(value any, field string) any {
	switch v := value.(type) {
	case string:
		if strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "http://") {
			return v
		}
		kind := ""
		if strings.HasPrefix(v, "data:") {
			kind = strings.SplitN(strings.TrimPrefix(v, "data:"), ";", 2)[0]
			kind = strings.SplitN(kind, ",", 2)[0]
		} else if field == "url" || field == "b64_json" || field == "base64" || field == "base64_data" || field == "data" || mediaPayloadReviewKey.MatchString(field) {
			if len(v) >= 16 {
				if _, err := base64.StdEncoding.DecodeString(v); err == nil {
					kind = "Base64"
				}
			}
		}
		if kind != "" {
			return fmt.Sprintf("[%s 编码内容已省略，%d 字符]", kind, len(v))
		}
		return v
	case map[string]any:
		out := map[string]any{}
		for key, item := range v {
			out[key] = compactEvidence(item, key)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = compactEvidence(item, field)
		}
		return out
	default:
		return value
	}
}
