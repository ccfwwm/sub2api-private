package media

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func pointer[T any](value T) *T { return &value }
func baseCharge(unit, source, price string) Charge {
	return Charge{Name: "generation", Unit: unit, QuantitySource: source, Price: pointer(price), Trigger: "success", Refundable: true, Method: "linear", Rounding: Rounding{"none", "1", "0", "per_output"}}
}

func TestMediaOverrideReplacesBaseRateForSimulatorAndTasks(t *testing.T) {
	identity := Identity{Multiplier: "2", ImageOverride: pointer("0"), VideoOverride: pointer("3")}
	require.Equal(t, "0", EffectiveMultiplier(Capability{MediaType: "image"}, identity))
	require.Equal(t, "3", EffectiveMultiplier(Capability{MediaType: "video"}, identity))
	require.Equal(t, "4", EffectiveMultiplier(Capability{MediaType: "audio", MusicRateOverride: pointer("4")}, identity))
	require.Equal(t, "2", EffectiveMultiplier(Capability{MediaType: "audio"}, identity))
}
func testCard(charges ...Charge) PriceCard {
	c := PriceCard{ID: "retail", Version: "1", Purpose: "sale", Currency: "USD", MediaType: "video", Operation: "video", Status: "enabled", Rules: []Rule{{Priority: 1, When: map[string]any{}, Charges: charges}}, MultiplierPolicy: "effective_media_then_time", SuccessPolicy: "any_output"}
	c.ModelMatch.Kind = "exact"
	c.ModelMatch.Value = "00sd2.5"
	c.Scope.Kind = "group"
	c.Scope.Reference = "1"
	return c
}
func TestBillingAcceptanceVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/multimodal/billing-vectors.json")
	require.NoError(t, err)
	var spec struct {
		Cases []struct {
			ID             string           `json:"id"`
			Lines          []map[string]any `json:"lines"`
			Multiplier     string           `json:"multiplier"`
			TimeMultiplier string           `json:"time_multiplier"`
			Maximum        *string          `json:"cap"`
			Expected       *string          `json:"expected_total"`
			Error          *string          `json:"expected_error"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &spec))
	require.GreaterOrEqual(t, len(spec.Cases), 20)
	for _, v := range spec.Cases {
		t.Run(v.ID, func(t *testing.T) {
			rule := Rule{}
			quantities := map[string][]string{}
			for index, line := range v.Lines {
				unit := stringValue(line["unit"])
				source := fmt.Sprint("line", index)
				ch := baseCharge(unit, source, stringValue(line["price"]))
				if line["price"] == nil {
					ch.Price = nil
				}
				if value := stringValue(line["mode"]); value != "" {
					ch.Rounding.Mode = value
				}
				if value := stringValue(line["step"]); value != "" {
					ch.Rounding.Step = value
				}
				if value := stringValue(line["minimum"]); value != "" {
					ch.Rounding.Minimum = value
				}
				if value := stringValue(line["scope"]); value != "" {
					ch.Rounding.Scope = value
				}
				if value := stringValue(line["method"]); value != "" {
					ch.Method = value
				}
				if value := stringValue(line["package_quantity"]); value != "" {
					ch.PackageQuantity = &value
				}
				if list, ok := line["tiers"].([]any); ok {
					for _, item := range list {
						b, _ := json.Marshal(item)
						var tier Tier
						require.NoError(t, json.Unmarshal(b, &tier))
						if object, ok := item.(map[string]any); ok {
							if value, ok := object["price"].(string); ok {
								tier.Price = &value
							}
						}
						ch.Tiers = append(ch.Tiers, tier)
					}
				}
				if list, ok := line["quantities"].([]any); ok {
					for _, q := range list {
						quantities[source] = append(quantities[source], stringValue(q))
					}
				} else if line["quantity"] != nil {
					quantities[source] = []string{stringValue(line["quantity"])}
				}
				rule.Charges = append(rule.Charges, ch)
			}
			calc, err := Calculate(rule, quantities, v.Multiplier, v.TimeMultiplier, "all", v.Maximum)
			if v.Error != nil {
				require.True(t, err != nil || len(calc.Missing) > 0, "Expected rejection or explicit missing usage")
				return
			}
			require.NoError(t, err)
			require.Empty(t, calc.Missing)
			require.NotNil(t, v.Expected)
			require.Equal(t, *v.Expected, calc.Total)
		})
	}
}
func TestPreciseRoundingAndMinuteAmount(t *testing.T) {
	// The fraction is below default Decimal division precision. It must still ceil.
	ch := baseCharge("second", "duration", "1")
	ch.Rounding.Mode = "ceil"
	calc, err := Calculate(Rule{Charges: []Charge{ch}}, map[string][]string{"duration": {"1.000000000000000000000000000001"}}, "1", "1", "all", nil)
	require.NoError(t, err)
	require.Equal(t, "2.00000000", calc.Total)
	ch = baseCharge("minute", "duration", "0.6")
	calc, err = Calculate(Rule{Charges: []Charge{ch}}, map[string][]string{"duration": {"61"}}, "1", "1", "all", nil)
	require.NoError(t, err)
	require.Equal(t, "0.61000000", calc.Total)
}
func TestNoFallbackFreeAndConflictingPrices(t *testing.T) {
	card := testCard(baseCharge("request", "task_count", "0"))
	require.NoError(t, ValidatePriceCard(card, true))
	capability := Capability{Model: "00sd2.5", MediaType: "video"}
	identity := Identity{GroupID: 1, Multiplier: "1", TimeMultiplier: "1"}
	resolved, err := ResolvePrice([]PriceCard{card}, capability, identity, "video", false, 9, time.Now())
	require.NoError(t, err)
	calc, _, err := Quote(resolved, resolved.Rules[0], Request{Quantities: map[string][]string{}}, identity)
	require.NoError(t, err)
	require.Equal(t, "0.00000000", calc.Total)
	_, err = ResolvePrice(nil, capability, identity, "video", false, 9, time.Now())
	require.Error(t, err)
	_, err = ResolvePrice([]PriceCard{card, card}, capability, identity, "video", false, 9, time.Now())
	require.Error(t, err)
	_, err = ResolvePrice([]PriceCard{card}, capability, identity, "video", true, 9, time.Now())
	require.Error(t, err, "Cost must never use retail")
}
func TestDraftRejectsInvalidTierPricesAndFiniteReserve(t *testing.T) {
	ch := baseCharge("second", "provider_duration", "0")
	ch.Method = "progressive"
	ch.Tiers = []Tier{{Price: pointer("-1")}}
	card := testCard(ch)
	require.Error(t, ValidatePriceCard(card, false))
	ch.Tiers[0].Price = nil
	card.Rules[0].Charges = []Charge{ch}
	require.NoError(t, ValidatePriceCard(card, false))
	require.Error(t, ValidatePriceCard(card, true))
	card = testCard(baseCharge("second", "artifact_duration", "0.01"))
	identity := Identity{Multiplier: "1", TimeMultiplier: "1"}
	_, _, err := Quote(card, card.Rules[0], Request{Quantities: map[string][]string{}}, identity)
	require.Error(t, err)
	card.Reservation.MaximumCharge = pointer("1")
	_, max, err := Quote(card, card.Rules[0], Request{Quantities: map[string][]string{}}, identity)
	require.NoError(t, err)
	require.Equal(t, "1.00000000", max)
	_, err = money("1000000000000")
	require.Error(t, err)
	require.Equal(t, decimal.Zero, decimal.Zero)
}
