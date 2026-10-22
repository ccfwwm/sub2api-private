package media

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactEvidencePreservesURLsTextAndOriginal(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("binary", 100)))
	url := "https://example.org/ref?signature=keep-this"
	prompt := strings.Repeat("Long natural language prompt. ", 500)
	input := map[string]any{"prompt": prompt, "images": []any{url, "data:image/png;base64," + encoded, map[string]any{"url": encoded, "role": "first_frame"}}, "inlineData": map[string]any{"mimeType": "image/png", "data": encoded}}
	result, err := json.Marshal(CompactEvidence(input))
	require.NoError(t, err)
	require.NotContains(t, string(result), encoded)
	require.Contains(t, string(result), url)
	require.Contains(t, string(result), prompt)
	require.Contains(t, string(result), "first_frame")
	require.Contains(t, string(result), "编码内容已省略")
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	require.Contains(t, string(raw), encoded)
}
