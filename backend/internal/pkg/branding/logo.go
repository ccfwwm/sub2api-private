// Package branding keeps uploaded branding images out of page bootstrap JSON.
package branding

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

const LogoPath = "/api/v1/settings/logo/"

type Logo struct {
	Data        []byte
	ContentType string
	Asset       string
	ETag        string
}

// ParseInlineLogo accepts raster data URLs only. Other configured URLs keep
// their existing behavior, and stored settings are never rewritten.
func ParseInlineLogo(value string) (Logo, bool) {
	header, encoded, ok := strings.Cut(strings.TrimSpace(value), ",")
	if !ok || len(encoded) > 8<<20 {
		return Logo{}, false
	}
	extensions := map[string]string{
		"data:image/png;base64": ".png", "data:image/jpeg;base64": ".jpg",
		"data:image/gif;base64": ".gif", "data:image/webp;base64": ".webp",
		"data:image/x-icon;base64": ".ico", "data:image/vnd.microsoft.icon;base64": ".ico",
	}
	ext, ok := extensions[strings.ToLower(header)]
	if !ok {
		return Logo{}, false
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 {
		return Logo{}, false
	}
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") || contentType == "image/svg+xml" {
		return Logo{}, false
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	return Logo{Data: data, ContentType: contentType, Asset: hash + ext, ETag: `"` + hash + `"`}, true
}

func PublicLogoURL(value string) string {
	if logo, ok := ParseInlineLogo(value); ok {
		return LogoPath + logo.Asset
	}
	return value
}
