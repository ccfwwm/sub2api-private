package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMonitorCustomBodyUpdatePreservesOmittedNullAndObject(t *testing.T) {
	for _, tc := range []struct {
		body             string
		omitted, cleared bool
	}{
		{`{}`, true, false},
		{`{"custom_body":null}`, false, true},
		{`{"custom_body":{"prompt":"hello"}}`, false, false},
	} {
		var req channelMonitorUpdateRequest
		require.NoError(t, json.Unmarshal([]byte(tc.body), &req))
		got, err := parseMonitorCustomBodyUpdate(req.CustomBody)
		require.NoError(t, err)
		if tc.omitted {
			require.Nil(t, got)
			continue
		}
		require.NotNil(t, got)
		if tc.cleared {
			require.Nil(t, *got)
		} else {
			require.Equal(t, "hello", (*got)["prompt"])
		}
	}
	for _, raw := range []string{`[]`, `"value"`, `true`, `1`} {
		_, err := parseMonitorCustomBodyUpdate(json.RawMessage(raw))
		require.Error(t, err)
	}
}
