package media

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTemplateValueMappingRequiresExplicitFallback(t *testing.T) {
	rule := ParameterRule{Phase: "semantic", Op: "map", Source: "layout", Values: map[string]any{"wide": "landscape", "off": false, "zero": 0, "none": nil}}
	require.NoError(t, ValidateParameterRules([]ParameterRule{rule}))
	for input, expected := range rule.Values {
		result, err := applyParameterRules(map[string]any{"layout": input}, []ParameterRule{rule}, "semantic")
		require.NoError(t, err)
		require.EqualValues(t, expected, result["layout"])
	}
	_, err := applyParameterRules(map[string]any{"layout": "unknown"}, []ParameterRule{rule}, "semantic")
	require.ErrorContains(t, err, "parameter_value_unmapped")
	rule.OnUnmapped = "preserve"
	result, err := applyParameterRules(map[string]any{"layout": "unknown"}, []ParameterRule{rule}, "semantic")
	require.NoError(t, err)
	require.Equal(t, "unknown", result["layout"])
	result, err = applyParameterRules(map[string]any{}, []ParameterRule{rule}, "semantic")
	require.NoError(t, err)
	require.NotContains(t, result, "layout")
	rule.OnUnmapped = "guess"
	require.ErrorContains(t, ValidateParameterRules([]ParameterRule{rule}), "invalid_mapping_fallback")
}

func TestTemplateParameterRuleWriteConflicts(t *testing.T) {
	for _, rules := range [][]ParameterRule{
		{{Phase: "wire", Op: "fixed", Target: "request", Value: map[string]any{}}, {Phase: "wire", Op: "fixed", Target: "request.flag", Value: false}},
		{{Phase: "wire", Op: "fixed", Target: "request.flag", Value: false}, {Phase: "wire", Op: "remove", Source: "request"}},
		{{Phase: "semantic", Op: "map", Source: "flag"}, {Phase: "semantic", Op: "default", Target: "flag", Value: true}},
		{{Phase: "wire", Op: "copy", Source: "x", Target: "parts[0].flag"}, {Phase: "wire", Op: "fixed", Target: "parts[0]", Value: false}},
		{{Phase: "wire", Op: "rename", Source: "request", Target: "request.nested"}},
		{{Phase: "wire", Op: "rename", Source: "request.nested", Target: "request"}},
	} {
		require.ErrorContains(t, ValidateParameterRules(rules), "parameter_rule_conflict")
	}
	// Semantic and wire phases may write the same field. Higher-priority
	// bindings explicitly replace a lower layer's rule at that same target.
	rules := []ParameterRule{{Phase: "semantic", Op: "fixed", Target: "flag", Value: false}, {Phase: "wire", Op: "convert", Source: "flag", Type: "string"}}
	require.NoError(t, ValidateParameterRules(rules))
	rules = mergeRuleLayers([]ParameterRule{{Phase: "semantic", Op: "default", Target: "flag", Value: false}, {Phase: "semantic", Op: "fixed", Target: "flag", Value: true}})
	require.Len(t, rules, 1)
	require.NoError(t, ValidateParameterRules(rules))
}
