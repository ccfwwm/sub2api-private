package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func mediaVersion(value any) string {
	b, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// mediaValuesEqual compares the JSON meaning of values rather than their Go
// representation. Account extras are loaded as typed capabilities from the
// database, while an edit form sends the same data back as generic maps.
func mediaValuesEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	decode := func(data []byte) (any, error) {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	}
	leftValue, leftErr := decode(leftJSON)
	rightValue, rightErr := decode(rightJSON)
	return leftErr == nil && rightErr == nil && reflect.DeepEqual(leftValue, rightValue)
}

func mediaCardForType(p ChannelModelPricing, model, operation, scope, reference, purpose, mediaType string) media.PriceCard {
	if mediaType == "" {
		mediaType = mediaTypeForOperation(operation)
	}
	return standardMediaCard(p, model, operation, scope, reference, purpose, mediaType)
}

func mediaTypeForOperation(operation string) string {
	switch operation {
	case "image", "image_edit", "gemini_image":
		return "image"
	case "audio", "chat_music", "speech":
		return "audio"
	default:
		return "video"
	}
}

func floatPrice(v *float64) *string {
	if v == nil {
		return nil
	}
	s := strconv.FormatFloat(*v, 'f', -1, 64)
	return &s
}

func mediaCharge(name, unit, source string, value *float64) (media.Charge, bool) {
	price := floatPrice(value)
	if price == nil {
		return media.Charge{}, false
	}
	return media.Charge{Name: name, Unit: unit, Price: price, QuantitySource: source, Trigger: "success", Refundable: true, Method: "linear", Rounding: media.Rounding{Mode: "none", Step: "1", Minimum: "0", Scope: "aggregate"}}, true
}

// standardMediaCard projects the existing Sub2API pricing fields into the
// asynchronous media engine. Administrators edit the ordinary pricing entry;
// this card is only a runtime snapshot and is never persisted separately.
func standardMediaCard(p ChannelModelPricing, model, operation, scope, reference, purpose, mediaType string) media.PriceCard {
	if mediaType == "" {
		mediaType = mediaTypeForOperation(operation)
	}
	c := media.PriceCard{ID: scope + ":" + reference + ":" + p.Platform + ":" + model + ":" + operation, Version: mediaVersion(p), Purpose: purpose, Status: "enabled", Currency: "USD", Operation: operation, MediaType: mediaType, MultiplierPolicy: "effective_media_then_time", SuccessPolicy: "any_output"}
	if purpose == "cost" {
		c.MultiplierPolicy = "account_cost_once"
	}
	c.ModelMatch.Kind, c.ModelMatch.Value = "exact", model
	c.Scope.Kind, c.Scope.Reference = scope, reference
	charges := []media.Charge{}
	tokenMode := p.BillingMode == BillingModeToken || p.BillingMode == ""
	switch p.BillingMode {
	case BillingModeCharacters:
		if ch, ok := mediaCharge("speech_characters", "input_character", "input_characters", p.PerRequestPrice); ok {
			charges = append(charges, ch)
		}
	case BillingModeVideo:
		if ch, ok := mediaCharge("generation", "second", "request_duration", p.PerRequestPrice); ok {
			charges = append(charges, ch)
		}
	case BillingModeToken, "":
		if ch, ok := mediaCharge("input_tokens", "token", "provider_tokens", p.InputPrice); ok {
			field := "input_tokens"
			ch.UsageField = &field
			charges = append(charges, ch)
		}
		if ch, ok := mediaCharge("output_tokens", "token", "provider_tokens", p.OutputPrice); ok {
			field := "output_tokens"
			ch.UsageField = &field
			charges = append(charges, ch)
		}
	default:
		if ch, ok := mediaCharge("generation", "request", "task_count", p.PerRequestPrice); ok {
			charges = append(charges, ch)
		}
	}
	if len(charges) == 0 && len(p.Intervals) == 0 {
		return c
	}
	priority := 0
	for _, iv := range p.Intervals {
		if tokenMode || p.BillingMode == BillingModeCharacters {
			continue
		}
		if iv.PerRequestPrice == nil || iv.TierLabel == "" {
			continue
		}
		unit, source := "request", "task_count"
		if p.BillingMode == BillingModeVideo {
			unit, source = "second", "request_duration"
		}
		ch, ok := mediaCharge("generation", unit, source, iv.PerRequestPrice)
		if !ok {
			continue
		}
		condition := "size_tier"
		if p.BillingMode == BillingModeVideo {
			condition = "resolution"
		}
		c.Rules = append(c.Rules, media.Rule{Priority: priority, When: map[string]any{condition: iv.TierLabel}, Charges: []media.Charge{ch}})
		priority++
	}
	if len(charges) > 0 {
		// The ordinary/default price is a fallback. Tier rules must be checked first.
		c.Rules = append(c.Rules, media.Rule{Priority: priority, When: map[string]any{}, Charges: charges})
	}
	return c
}

func accountMediaModels(extra map[string]any) ([]media.Capability, error) {
	if extra == nil || extra["media_models"] == nil {
		return nil, nil
	}
	b, err := json.Marshal(extra["media_models"])
	if err != nil {
		return nil, err
	}
	var caps []media.Capability
	if err = json.Unmarshal(b, &caps); err != nil {
		return nil, err
	}
	for i := range caps {
		caps[i] = media.CapabilityDefaults(caps[i])
	}
	return caps, nil
}

// NormalizeMediaAccountConfig maintains one model list while retaining the
// standard Sub2API mapping field for older admin clients.
func NormalizeMediaAccountConfig(platform string, credentials, extra map[string]any, mappingOnly bool) (map[string]any, map[string]any, error) {
	if platform != PlatformImageVideo {
		return credentials, extra, nil
	}
	credentials = cloneAnyMap(credentials)
	extra = cloneAnyMap(extra)
	caps, err := accountMediaModels(extra)
	if err != nil {
		return nil, nil, err
	}
	mapping := (&Account{Credentials: credentials}).GetModelMapping()
	_, mediaPresent := extra["media_models"]
	if mappingOnly || !mediaPresent {
		byModel := map[string]media.Capability{}
		for _, cap := range caps {
			byModel[cap.Model] = cap
		}
		keys := make([]string, 0, len(mapping))
		for model := range mapping {
			keys = append(keys, model)
		}
		sort.Strings(keys)
		caps = nil
		for _, model := range keys {
			if strings.Contains(model, "*") {
				return nil, nil, infraerrors.BadRequest("EXACT_MEDIA_MODEL_REQUIRED", "Media models require exact IDs")
			}
			cap, exists := byModel[model]
			if !exists {
				cap = media.CapabilityDefaults(media.Capability{Model: model, UpstreamModel: mapping[model], ValidationStatus: "待配置协议模板"})
				cap.Enabled = false
			}
			cap.Model, cap.UpstreamModel = model, mapping[model]
			caps = append(caps, cap)
		}
	}
	projected := map[string]any{}
	for i, cap := range caps {
		caps[i] = media.CapabilityDefaults(cap)
		projected[cap.Model] = caps[i].UpstreamModel
	}
	extra["media_models"] = caps
	credentials["model_mapping"] = projected
	return credentials, extra, nil
}

func cloneAnyMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, v := range value {
		result[key] = v
	}
	return result
}

func ValidateMediaAccount(platform, kind string, extra map[string]any) error {
	if platform == PlatformImageVideo && kind != AccountTypeAPIKey {
		return infraerrors.BadRequest("MEDIA_APIKEY_REQUIRED", "Media accounts use API Key credentials")
	}
	caps, err := accountMediaModels(extra)
	if err != nil {
		return infraerrors.BadRequest("INVALID_MEDIA_MODELS", "Media models must be an array of capabilities")
	}
	if len(caps) > 0 && platform != PlatformImageVideo {
		return infraerrors.BadRequest("MEDIA_PLATFORM_REQUIRED", "Media models require the imagevideo platform")
	}
	seen := map[string]bool{}
	for _, cap := range caps {
		if seen[cap.Model] {
			return infraerrors.BadRequest("DUPLICATE_MEDIA_MODEL", "Each model may be configured once per account")
		}
		seen[cap.Model] = true
		// Runtime bindings always come from the account and its existing groups.
		cap.AccountIDs, cap.GroupIDs = []int64{1}, []int64{1}
		cap.ID, cap.Version = "manual:"+cap.Model, mediaVersion(cap)
		cap.Status = "enabled"
		if err := media.ValidateCapability(cap, cap.Enabled); err != nil {
			return infraerrors.BadRequest("INVALID_MEDIA_CAPABILITY", err.Error())
		}
	}
	return nil
}

// validateMediaAccountUpdate keeps the account PUT path separate from a media
// capability publish. The edit form sends the complete Extra object back even
// when only concurrency, priority, proxy, or status changed. Re-validating an
// unchanged enabled capability as a publish in that case makes an unrelated
// account edit depend on a verification record. A changed media_models value
// still goes through capability validation. Verification is informational.
func validateMediaAccountUpdate(platform, kind string, currentExtra, requestedExtra map[string]any) error {
	// Always enforce the account-level platform/type invariant.
	if err := ValidateMediaAccount(platform, kind, nil); err != nil {
		return err
	}
	if requestedExtra == nil {
		return nil
	}
	requested, present := requestedExtra["media_models"]
	if !present {
		return nil
	}
	current, hadCurrent := currentExtra["media_models"]
	if hadCurrent && mediaValuesEqual(current, requested) {
		return nil
	}
	return ValidateMediaAccount(platform, kind, map[string]any{"media_models": requested})
}

func exactMediaPricing(entries []ChannelModelPricing, model string) *ChannelModelPricing {
	for _, p := range entries {
		if p.Platform != PlatformImageVideo {
			continue
		}
		for _, m := range p.Models {
			if m == model {
				copy := p.Clone()
				return &copy
			}
		}
	}
	return nil
}

// UnifiedMediaConfig projects the existing account, group and channel settings
// into immutable task snapshots. Historical tasks keep their original snapshots.
func UnifiedMediaConfig(accounts AccountRepository, groups GroupRepository, channels *ChannelService) func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
	return func(ctx context.Context, groupID int64) ([]media.Capability, []media.PriceCard, error) {
		group, err := groups.GetByID(ctx, groupID)
		if err != nil {
			return nil, nil, err
		}
		if group.Platform != PlatformImageVideo && group.Platform != PlatformComposite {
			return nil, nil, nil
		}
		list, err := accounts.ListByGroup(ctx, groupID)
		if err != nil {
			return nil, nil, err
		}
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Priority == list[j].Priority {
				return list[i].ID < list[j].ID
			}
			return list[i].Priority < list[j].Priority
		})
		channel, err := channels.GetChannelForGroup(ctx, groupID)
		if err != nil {
			return nil, nil, err
		}
		caps := []media.Capability{}
		cards := []media.PriceCard{}
		seen := map[string]bool{}
		for _, account := range list {
			if account.Platform != PlatformImageVideo || account.Type != AccountTypeAPIKey {
				continue
			}
			models, err := accountMediaModels(account.Extra)
			if err != nil {
				return nil, nil, err
			}
			for _, cap := range models {
				cap.Status = "enabled"
				cap.ID = fmt.Sprintf("account:%d:%s", account.ID, cap.Model)
				cap.Version = mediaVersion(cap)
				cap.AccountIDs, cap.GroupIDs = []int64{account.ID}, []int64{groupID}
				cap.PriceTemplates, cap.CostTemplates = nil, nil
				cap.ChannelID = 0
				if channel != nil && channel.IsActive() {
					cap.ChannelID = channel.ID
				}
				if !account.IsSchedulable() {
					cap.Enabled = false
				}
				if group.ModelAllowlistEnabled() && !group.ModelAllowlist.Allows(cap.Model) {
					cap.Enabled = false
				}
				caps = append(caps, cap)
				for _, op := range cap.Operations {
					add := func(p *ChannelModelPricing, scope, ref, purpose string) {
						if p == nil {
							return
						}
						card := mediaCardForType(*p, cap.Model, op, scope, ref, purpose, cap.MediaType)
						if card.MediaType != cap.MediaType || len(card.Rules) == 0 {
							return
						}
						if purpose == "sale" && p.TimePricing != nil {
							card.TimeMultiplier = fmt.Sprint(p.TimePricing.MultiplierAt(time.Now()))
						}
						if !seen[card.ID] {
							seen[card.ID] = true
							cards = append(cards, card)
						}
					}
					add(exactMediaPricing(group.ModelPricing, cap.Model), "group", fmt.Sprint(groupID), "sale")
					if channel == nil || !channel.IsActive() {
						continue
					}
					add(exactMediaPricing(channel.ModelPricing, cap.Model), "channel", fmt.Sprint(channel.ID), "sale")
					// Explicit cost rules only; ApplyPricingToAccountStats never copies retail prices.
					rules := append([]AccountStatsPricingRule(nil), channel.AccountStatsPricingRules...)
					sort.SliceStable(rules, func(i, j int) bool { return rules[i].SortOrder < rules[j].SortOrder })
					for _, rule := range rules {
						matches := func(ids []int64, id int64) bool {
							if len(ids) == 0 {
								return true
							}
							for _, x := range ids {
								if x == id {
									return true
								}
							}
							return false
						}
						if matches(rule.GroupIDs, groupID) && matches(rule.AccountIDs, account.ID) {
							p := exactMediaPricing(rule.Pricing, cap.Model)
							if p != nil {
								add(p, "account_group", fmt.Sprintf("%d:%d", account.ID, groupID), "cost")
								break
							}
						}
					}
				}
			}
		}
		return caps, cards, nil
	}
}
