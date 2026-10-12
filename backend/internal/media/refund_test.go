package media

import (
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRefundRespectsNonrefundableSuccessAndTaskCap(t *testing.T) {
	charges := []Charge{
		{Name: "refundable", Unit: "request", Price: pointer("0.6"), QuantitySource: "task_count", Trigger: "success", Refundable: true, Method: "linear", Rounding: Rounding{Mode: "none", Step: "1", Minimum: "0", Scope: "aggregate"}},
		{Name: "nonrefundable", Unit: "request", Price: pointer("0.4"), QuantitySource: "task_count", Trigger: "success", Refundable: false, Method: "linear", Rounding: Rounding{Mode: "none", Step: "1", Minimum: "0", Scope: "aggregate"}},
	}
	task := StoredTask{Snapshot: Snapshot{SaleRule: Rule{Charges: charges}}, Identity: Identity{Multiplier: "2", TimeMultiplier: "1"}, Quantities: map[string][]string{"task_count": {"1"}}}
	for _, test := range []struct{ captured, expected string }{{"2", "1.2"}, {"1", "0.6"}, {"0", "0"}} {
		amount, err := refundableAmount(task, decimal.RequireFromString(test.captured))
		require.NoError(t, err)
		require.Equal(t, test.expected, amount.String())
	}
}

func TestConfigurationRejectsUnknownCredentialFields(t *testing.T) {
	var cap Capability
	require.Error(t, decodeConfig([]byte(`{"profile_id":"test","api_key":"must-never-be-stored"}`), &cap))
}
