package branding

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

const testPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII="

func TestInlineLogoPreservesImageAndChangesURLWithContent(t *testing.T) {
	source := "data:image/png;base64," + testPNG
	logo, ok := ParseInlineLogo(source)
	require.True(t, ok)
	original, err := base64.StdEncoding.DecodeString(testPNG)
	require.NoError(t, err)
	require.Equal(t, original, logo.Data)
	require.Equal(t, "image/png", logo.ContentType)
	require.Equal(t, LogoPath+logo.Asset, PublicLogoURL(source))
	require.Len(t, logo.Asset, 68)
	changed := "data:image/png;base64," + base64.StdEncoding.EncodeToString(append(original, 0))
	require.NotEqual(t, PublicLogoURL(source), PublicLogoURL(changed))
}

func TestInlineLogoLeavesOtherSettingsUntouched(t *testing.T) {
	for _, source := range []string{"", "/logo.png", "https://example.com/logo.webp",
		"data:image/png;base64,invalid", "data:text/html;base64,PHNjcmlwdD4=",
		"data:image/png;base64,PHNjcmlwdD4=", "data:image/svg+xml;base64,PHN2Zz4="} {
		t.Run(source, func(t *testing.T) {
			_, ok := ParseInlineLogo(source)
			require.False(t, ok)
			require.Equal(t, source, PublicLogoURL(source))
		})
	}
}
