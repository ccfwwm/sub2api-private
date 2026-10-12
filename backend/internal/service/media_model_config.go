package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// MediaPricing extends an ordinary model pricing entry. No supplier templates,
// secrets or separately editable retail price cards are stored here.
type MediaPricing struct {
	MediaType   string       `json:"media_type"`
	Rules       []media.Rule `json:"rules"`
	Reservation struct {
		MaximumCharge *string           `json:"maximum_charge"`
		MaxQuantities map[string]string `json:"max_quantities"`
		Overrun       string            `json:"overrun"`
	} `json:"reservation"`
	SuccessPolicy string `json:"success_policy"`
}

func mediaVersion(value any) string {
	b, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func mediaCard(p ChannelModelPricing, model, operation, scope, reference, purpose string) media.PriceCard {
	c := media.PriceCard{ID: scope + ":" + reference + ":" + p.Platform + ":" + model + ":" + operation, Version: mediaVersion(p), Purpose: purpose, Status: "enabled", Currency: "USD", Operation: operation, MediaType: p.Media.MediaType, Rules: p.Media.Rules, SuccessPolicy: p.Media.SuccessPolicy, MultiplierPolicy: "effective_media_then_time"}
	if purpose == "cost" {
		c.MultiplierPolicy = "account_cost_once"
	}
	c.ModelMatch.Kind, c.ModelMatch.Value = "exact", model
	c.Scope.Kind, c.Scope.Reference = scope, reference
	c.Reservation = p.Media.Reservation
	if c.SuccessPolicy == "" {
		c.SuccessPolicy = "any_output"
	}
	return c
}

func ValidateMediaPricing(p ChannelModelPricing) error {
	if p.Media == nil {
		return nil
	}
	if p.Platform != PlatformImageVideo {
		return infraerrors.BadRequest("MEDIA_PLATFORM_REQUIRED", "Media pricing requires the imagevideo platform")
	}
	if len(p.Models) == 0 {
		return infraerrors.BadRequest("MEDIA_MODEL_REQUIRED", "Exact model IDs are required")
	}
	for _, model := range p.Models {
		if strings.TrimSpace(model) != model || model == "" || strings.Contains(model, "*") {
			return infraerrors.BadRequest("INVALID_MEDIA_MODEL", "Media model IDs must be exact")
		}
	}
	op := p.Media.MediaType
	if err := media.ValidatePriceCard(mediaCard(p, p.Models[0], op, "group", "1", "sale"), true); err != nil {
		return infraerrors.BadRequest("INVALID_MEDIA_PRICE", err.Error())
	}
	return nil
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
	return caps, nil
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

func exactMediaPricing(entries []ChannelModelPricing, model string) *ChannelModelPricing {
	for _, p := range entries {
		if p.Platform != PlatformImageVideo || p.Media == nil {
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
						if p == nil || p.Media.MediaType != cap.MediaType {
							return
						}
						card := mediaCard(*p, cap.Model, op, scope, ref, purpose)
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
