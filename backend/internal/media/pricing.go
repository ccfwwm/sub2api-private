package media

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

var numeric = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

func number(s string) (decimal.Decimal, error) {
	if !numeric.MatchString(s) || len(s) > 64 {
		return decimal.Zero, fail(400, "invalid_decimal", "Non-negative decimal strings are required")
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return d, fail(400, "invalid_decimal", "Invalid decimal")
	}
	return d, nil
}
func price(s *string) (decimal.Decimal, error) {
	if s == nil {
		return decimal.Zero, fail(422, "price_not_configured", "Price is not configured")
	}
	d, e := number(*s)
	if e == nil && d.Exponent() < -12 {
		e = fail(400, "invalid_precision", "Prices support at most 12 decimals")
	}
	return d, e
}

var units = map[string]bool{"request": true, "output": true, "second": true, "minute": true, "token": true, "input_image": true, "input_video_second": true, "input_audio_second": true}
var sources = map[string]string{"task_count": "request", "successful_outputs": "output", "request_duration": "time", "fixed_duration": "time", "artifact_duration": "time", "provider_duration": "time", "provider_tokens": "token", "accepted_input_images": "input_image", "accepted_input_video_duration": "input_video_second", "accepted_input_audio_duration": "input_audio_second"}

func ValidatePriceCard(c PriceCard, publish bool) error {
	if c.ID == "" || c.Version == "" || c.ModelMatch.Value == "" || c.Scope.Reference == "" || c.Operation == "" {
		return fail(400, "invalid_price_card", "Identity, model, scope and operation are required")
	}
	if c.Currency != "USD" || (c.Purpose != "sale" && c.Purpose != "cost") {
		return fail(400, "invalid_price_card", "USD and sale/cost purpose are required")
	}
	if !contains([]string{"image", "video", "audio"}, c.MediaType) || !contains([]string{"image", "image_edit", "gemini_image", "video", "audio", "chat_music"}, c.Operation) {
		return fail(400, "invalid_price_operation", "Media type and operation are required")
	}
	if c.SuccessPolicy != "any_output" && c.SuccessPolicy != "all_outputs" {
		return fail(400, "invalid_success_policy", "Choose any_output or all_outputs")
	}
	if c.Purpose == "sale" && (!contains([]string{"group", "channel", "linked_template"}, c.Scope.Kind) || c.MultiplierPolicy != "effective_media_then_time") || c.Purpose == "cost" && (!contains([]string{"account", "account_group", "supplier_template"}, c.Scope.Kind) || c.MultiplierPolicy != "account_cost_once") {
		return fail(400, "invalid_price_scope", "Scope and multiplier must match the price purpose")
	}
	if c.ModelMatch.Kind != "exact" && c.ModelMatch.Kind != "alias" && c.ModelMatch.Kind != "prefix" {
		return fail(400, "invalid_model_match", "Explicit exact, alias or prefix required")
	}
	if c.MultiplierPolicy != "effective_media_then_time" && c.MultiplierPolicy != "account_cost_once" {
		return fail(400, "invalid_multiplier", "Unknown multiplier policy")
	}
	if publish && c.TestOnly {
		return fail(400, "test_price_not_publishable", "Test-only prices cannot be published")
	}
	if len(c.Rules) == 0 {
		return fail(400, "empty_rules", "At least one rule is required")
	}
	priorities := map[int]bool{}
	for _, r := range c.Rules {
		if priorities[r.Priority] || len(r.Charges) == 0 {
			return fail(409, "rule_conflict", "Rule priorities must be unique and charges nonempty")
		}
		priorities[r.Priority] = true
		names := map[string]bool{}
		for _, ch := range r.Charges {
			if ch.Name == "" || names[ch.Name] || !units[ch.Unit] {
				return fail(400, "invalid_charge", "Unique names and supported units required")
			}
			names[ch.Name] = true
			if ch.Price != nil {
				if _, e := price(ch.Price); e != nil {
					return e
				}
			} else if publish && ch.Method != "progressive" {
				return fail(422, "price_not_configured", "All published charge prices must be configured")
			}
			kind, ok := sources[ch.QuantitySource]
			if !ok {
				return fail(400, "invalid_quantity_source", "Unknown source")
			}
			if kind != "time" && kind != ch.Unit || kind == "time" && ch.Unit != "second" && ch.Unit != "minute" {
				return fail(400, "unit_source_conflict", "Quantity source and unit conflict")
			}
			if ch.QuantitySource == "provider_tokens" && (ch.UsageField == nil || *ch.UsageField == "") {
				return fail(400, "usage_field_required", "Token charges require an adapter-whitelisted field")
			}
			if ch.QuantitySource == "provider_tokens" && !contains([]string{"total_tokens", "input_tokens", "output_tokens", "image_input_tokens", "image_output_tokens"}, *ch.UsageField) {
				return fail(400, "invalid_usage_field", "Token quantity must use a supported field")
			}
			if ch.Fallback != nil {
				k, found := sources[*ch.Fallback]
				if !found || k != kind {
					return fail(400, "invalid_fallback", "Fallback must have the same unit domain")
				}
			}
			if ch.Trigger != "success" && ch.Trigger != "accepted" {
				return fail(400, "invalid_trigger", "success or accepted required")
			}
			if ch.Trigger == "accepted" && (ch.Unit != "request" || ch.Refundable) {
				return fail(400, "invalid_accepted_fee", "Only explicit nonrefundable request fees may use accepted")
			}
			if ch.Rounding.Mode != "none" && ch.Rounding.Mode != "ceil" && ch.Rounding.Mode != "floor" && ch.Rounding.Mode != "half_up" {
				return fail(400, "invalid_rounding", "Unknown rounding")
			}
			step, e := number(ch.Rounding.Step)
			if e != nil {
				return e
			}
			if !step.IsPositive() {
				return fail(400, "invalid_rounding", "Step must be positive")
			}
			if _, e = number(ch.Rounding.Minimum); e != nil {
				return e
			}
			if ch.Rounding.Scope != "per_output" && ch.Rounding.Scope != "aggregate" {
				return fail(400, "invalid_rounding_scope", "per_output or aggregate required")
			}
			switch ch.Method {
			case "linear":
				if len(ch.Tiers) > 0 || ch.PackageQuantity != nil {
					return fail(400, "conflicting_tiers", "Linear pricing cannot have tiers")
				}
			case "package":
				if ch.PackageQuantity == nil || len(ch.Tiers) > 0 {
					return fail(400, "invalid_package", "Package quantity required")
				}
				if _, e = number(*ch.PackageQuantity); e != nil {
					return e
				}
			case "progressive":
				if len(ch.Tiers) == 0 || ch.PackageQuantity != nil {
					return fail(400, "invalid_tiers", "Progressive tiers required")
				}
				prev := decimal.Zero
				for i, t := range ch.Tiers {
					if _, e = price(t.Price); e != nil && (publish || t.Price != nil) {
						return e
					}
					if t.UpTo == nil {
						if i != len(ch.Tiers)-1 {
							return fail(400, "invalid_tiers", "Unbounded tier must be last")
						}
					} else {
						end, e := number(*t.UpTo)
						if e != nil {
							return e
						}
						if !end.GreaterThan(prev) {
							return fail(400, "invalid_tiers", "Tier bounds must increase")
						}
						prev = end
					}
				}
			default:
				return fail(400, "invalid_method", "linear, package or progressive required")
			}
		}
	}
	for _, q := range c.Reservation.MaxQuantities {
		if _, e := number(q); e != nil {
			return e
		}
	}
	if c.Reservation.MaximumCharge != nil {
		if _, e := number(*c.Reservation.MaximumCharge); e != nil {
			return e
		}
	}
	return nil
}

func MatchRule(c PriceCard, conditions map[string]any) (Rule, error) {
	rules := append([]Rule(nil), c.Rules...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
	for _, r := range rules {
		match := true
		for k, v := range r.When {
			if stringValue(conditions[k]) != stringValue(v) {
				match = false
				break
			}
		}
		if match {
			return r, nil
		}
	}
	return Rule{}, fail(422, "price_not_configured", "No price covers these conditions")
}
func ResolvePrice(cards []PriceCard, cap Capability, identity Identity, operation string, cost bool, accountID int64, at time.Time) (PriceCard, error) {
	type candidate struct {
		card                 PriceCard
		scope, model, length int
	}
	list := []candidate{}
	for _, c := range cards {
		if c.Status != "enabled" || Operation(c.Operation) != Operation(operation) || c.MediaType != cap.MediaType || c.EffectiveAt != nil && c.EffectiveAt.After(at) {
			continue
		}
		if cost && c.Purpose != "cost" || !cost && c.Purpose != "sale" {
			continue
		}
		rank := 99
		reference := c.Scope.Reference
		if !cost {
			switch c.Scope.Kind {
			case "group":
				if reference == fmt.Sprint(identity.GroupID) {
					rank = 0
				}
			case "channel":
				if cap.ChannelID > 0 && reference == fmt.Sprint(cap.ChannelID) {
					rank = 1
				}
			case "linked_template":
				for _, s := range cap.PriceTemplates {
					if s == c.ID || s == reference {
						rank = 2
					}
				}
			}
		} else {
			switch c.Scope.Kind {
			case "account":
				if reference == fmt.Sprint(accountID) {
					rank = 0
				}
			case "account_group":
				if reference == fmt.Sprintf("%d:%d", accountID, identity.GroupID) {
					rank = 1
				}
			case "supplier_template":
				for _, s := range cap.CostTemplates {
					if s == c.ID || s == reference {
						rank = 2
					}
				}
			}
		}
		if rank == 99 {
			continue
		}
		mr := 99
		switch c.ModelMatch.Kind {
		case "exact":
			if c.ModelMatch.Value == cap.Model {
				mr = 0
			}
		case "alias":
			if c.ModelMatch.Value == cap.UpstreamModel {
				mr = 1
			}
		case "prefix":
			if strings.HasPrefix(cap.Model, c.ModelMatch.Value) {
				mr = 2
			}
		}
		if mr != 99 {
			list = append(list, candidate{c, rank, mr, len(c.ModelMatch.Value)})
		}
	}
	if len(list) == 0 {
		return PriceCard{}, fail(422, "price_not_configured", "No configured media price")
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].scope != list[j].scope {
			return list[i].scope < list[j].scope
		}
		if list[i].model != list[j].model {
			return list[i].model < list[j].model
		}
		return list[i].length > list[j].length
	})
	if len(list) > 1 && list[0].scope == list[1].scope && list[0].model == list[1].model && list[0].length == list[1].length {
		return PriceCard{}, fail(409, "price_conflict", "Ambiguous price cards")
	}
	return list[0].card, nil
}
func rounded(q decimal.Decimal, r Rounding) (decimal.Decimal, error) {
	step, e := number(r.Step)
	if e != nil {
		return q, e
	}
	minimum, e := number(r.Minimum)
	if e != nil {
		return q, e
	}
	if !step.IsPositive() {
		return q, fail(400, "invalid_step", "Step must be positive")
	}
	switch r.Mode {
	case "none":
	case "ceil":
		whole, rem := q.QuoRem(step, 0)
		if !rem.IsZero() {
			whole = whole.Add(decimal.NewFromInt(1))
		}
		q = whole.Mul(step)
	case "floor":
		whole, _ := q.QuoRem(step, 0)
		q = whole.Mul(step)
	case "half_up":
		whole, rem := q.QuoRem(step, 0)
		if rem.Mul(decimal.NewFromInt(2)).GreaterThanOrEqual(step) {
			whole = whole.Add(decimal.NewFromInt(1))
		}
		q = whole.Mul(step)
	default:
		return q, fail(400, "invalid_rounding", "Unknown rounding")
	}
	return decimal.Max(q, minimum), nil
}
func chargeAmount(ch Charge, q decimal.Decimal) (decimal.Decimal, error) {
	p, e := price(ch.Price)
	if e != nil && ch.Method != "progressive" {
		return decimal.Zero, e
	}
	switch ch.Method {
	case "linear":
		return p.Mul(q), nil
	case "package":
		if ch.PackageQuantity == nil {
			return decimal.Zero, fail(400, "invalid_package", "Missing package size")
		}
		expected, e := number(*ch.PackageQuantity)
		if e != nil {
			return decimal.Zero, e
		}
		if !q.Equal(expected) {
			return decimal.Zero, fail(422, "package_mismatch", "Quantity not covered by package")
		}
		return p, nil
	case "progressive":
		amount, prev := decimal.Zero, decimal.Zero
		for _, t := range ch.Tiers {
			end := q
			if t.UpTo != nil {
				end, e = number(*t.UpTo)
				if e != nil {
					return amount, e
				}
			}
			tp, e := price(t.Price)
			if e != nil {
				return amount, e
			}
			n := decimal.Max(decimal.Zero, decimal.Min(q, end).Sub(prev))
			amount = amount.Add(n.Mul(tp))
			prev = end
			if end.GreaterThanOrEqual(q) {
				return amount, nil
			}
		}
		return amount, fail(422, "tier_not_configured", "Progressive tiers do not cover quantity")
	default:
		return decimal.Zero, fail(400, "invalid_method", "Unknown method")
	}
}

// Calculate is shared by quotes, the simulator and final settlement.
// Missing observed quantities are explicit: absent usage never means zero.
func Calculate(rule Rule, quantities map[string][]string, multiplier, timeMultiplier string, event string, capAmount *string) (Calculation, error) {
	result := Calculation{Currency: "USD", Items: []Line{}, Missing: []string{}}
	total := decimal.Zero
	for _, ch := range rule.Charges {
		if event == "accepted" && ch.Trigger != "accepted" || event == "success_only" && ch.Trigger != "success" || event == "failure" && ch.Trigger != "accepted" {
			continue
		}
		key := ch.QuantitySource
		if ch.UsageField != nil && ch.QuantitySource == "provider_tokens" {
			key += "." + *ch.UsageField
		}
		values, ok := quantities[key]
		if !ok && ch.Fallback != nil {
			values, ok = quantities[*ch.Fallback]
		}
		if !ok {
			result.Missing = append(result.Missing, key)
			continue
		}
		original := append([]string(nil), values...)
		nums := []decimal.Decimal{}
		for _, v := range values {
			d, e := number(v)
			if e != nil {
				return result, e
			}
			nums = append(nums, d)
		}
		if ch.Rounding.Scope == "aggregate" {
			sum := decimal.Zero
			for _, n := range nums {
				sum = sum.Add(n)
			}
			nums = []decimal.Decimal{sum}
		}
		quantity, amount := decimal.Zero, decimal.Zero
		for _, n := range nums {
			q, e := rounded(n, ch.Rounding)
			if e != nil {
				return result, e
			}
			if ch.Unit == "minute" {
				q = q.DivRound(decimal.NewFromInt(60), 64)
			}
			a, e := chargeAmount(ch, q)
			if e != nil {
				return result, e
			}
			quantity = quantity.Add(q)
			amount = amount.Add(a)
		}
		result.Items = append(result.Items, Line{Name: ch.Name, Unit: ch.Unit, Original: original, Quantity: quantity.String(), UnitPrice: stringValuePointer(ch.Price), RawAmount: amount.String(), Trigger: ch.Trigger})
		total = total.Add(amount)
	}
	m, e := number(multiplier)
	if e != nil {
		return result, e
	}
	tm, e := number(timeMultiplier)
	if e != nil {
		return result, e
	}
	total = total.Mul(m).Mul(tm)
	result.Uncapped = total.StringFixed(8)
	if capAmount != nil {
		cap, e := number(*capAmount)
		if e != nil {
			return result, e
		}
		total = decimal.Min(total, cap)
	}
	result.Total = total.Round(8).StringFixed(8)
	if _, e := money(result.Total); e != nil {
		return result, e
	}
	return result, nil
}

func money(value string) (decimal.Decimal, error) {
	d, e := number(value)
	if e != nil {
		return d, e
	}
	if d.GreaterThan(decimal.RequireFromString("999999999999.99999999")) || d.Exponent() < -8 {
		return decimal.Zero, fail(400, "amount_out_of_range", "Amount exceeds ledger precision or range")
	}
	return d, nil
}
func stringValuePointer(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func Quote(card PriceCard, rule Rule, req Request, identity Identity) (Calculation, string, error) {
	q := map[string][]string{}
	for k, v := range req.Quantities {
		q[k] = v
	}
	q["task_count"] = []string{"1"}
	for _, ch := range rule.Charges {
		key := ch.QuantitySource
		if ch.UsageField != nil && key == "provider_tokens" {
			key += "." + *ch.UsageField
		}
		if _, found := q[key]; !found {
			max, ok := card.Reservation.MaxQuantities[key]
			if !ok && card.Reservation.MaximumCharge == nil {
				return Calculation{}, "", fail(422, "reservation_unbounded", "Unknown usage requires a finite maximum")
			}
			if ok {
				q[key] = []string{max}
			}
		}
	}
	calc, e := Calculate(rule, q, identity.Multiplier, identity.TimeMultiplier, "all", nil)
	if e != nil {
		return calc, "", e
	}
	maximum := calc.Total
	if card.Reservation.MaximumCharge != nil {
		d, e := money(*card.Reservation.MaximumCharge)
		if e != nil {
			return calc, "", e
		}
		maximum = d.Round(8).StringFixed(8)
	}
	return calc, maximum, nil
}
