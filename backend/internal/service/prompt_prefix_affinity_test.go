package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrefixAffinitySeedUsesStableLeadingMessages(t *testing.T) {
	first := []byte(`{"tools":[{"name":"lookup"}],"messages":[{"role":"system","content":"Use the tools"},{"role":"user","content":"first"}]}`)
	next := []byte(`{"tools":[{"name":"lookup"}],"messages":[{"role":"system","content":"Use the tools"},{"role":"user","content":"second"},{"role":"developer","content":"temporary"}]}`)
	base := prefixAffinitySeed(first, "/v1/chat/completions")
	require.NotEmpty(t, base)
	require.Equal(t, base, prefixAffinitySeed(next, "/v1/chat/completions"))
	require.Empty(t, prefixAffinitySeed([]byte(`{"messages":[{"role":"user","content":"private prompt"}]}`), "/v1/chat/completions"))
	require.Empty(t, prefixAffinitySeed([]byte(`{"previous_response_id":"resp_a","messages":[{"role":"system","content":"use tools"}]}`), "/v1/chat/completions"))
}

func TestPrefixAffinityTenantIsolationAndExplicitSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	require.NoError(t, os.WriteFile(path, []byte(`[{"group_id":3,"platform":"openai","endpoint":"/v1/chat/completions","model":"test-model","upstream_model":"real-model","upstream_endpoint":"/v1/responses","mode":"enforce","percent":100,"prompt_key":true,"route":true}]`), 0o600))
	t.Setenv("SUB2API_PREFIX_AFFINITY_FILE", path)
	body := []byte(`{"messages":[{"role":"system","content":"shared instructions"},{"role":"user","content":"question"}]}`)
	base := context.Background()
	key := func(id int64) *PrefixAffinity {
		return prefixAffinityInContext(WithPrefixAffinity(base, 3, id, PlatformOpenAI, "/v1/chat/completions", "test-model", body, false))
	}
	tenantOne, tenantTwo := key(42), key(43)
	require.NotNil(t, tenantOne)
	require.NotEqual(t, tenantOne.key, tenantTwo.key)
	require.Equal(t, tenantOne.key, key(42).key)
	require.Nil(t, prefixAffinityInContext(WithPrefixAffinity(base, 3, 42, PlatformOpenAI, "/v1/chat/completions", "test-model", body, true)))
	account := &Account{Platform: PlatformOpenAI}
	ctx := WithPrefixAffinity(base, 3, 42, PlatformOpenAI, "/v1/chat/completions", "test-model", body, false)
	require.NotEqual(t, "existing", prefixAffinityPromptKey(ctx, account, "real-model", "existing"))
	require.Equal(t, "existing", prefixAffinityPromptKey(ctx, account, "other-model", "existing"))
}
