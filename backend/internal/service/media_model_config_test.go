//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/stretchr/testify/require"
)

type unifiedMediaAccounts struct {
	AccountRepository
	list []Account
}

func (s unifiedMediaAccounts) ListByGroup(context.Context, int64) ([]Account, error) {
	return s.list, nil
}

type unifiedMediaGroups struct {
	GroupRepository
	group *Group
}

func (s unifiedMediaGroups) GetByID(context.Context, int64) (*Group, error) { return s.group, nil }

func mediaTestPrice(model, unit, source, value string) ChannelModelPricing {
	price := 0.0
	if _, err := fmt.Sscanf(value, "%f", &price); err != nil {
		panic(err)
	}
	mode := BillingModePerRequest
	if unit == "second" {
		mode = BillingModeVideo
	}
	if unit == "token" {
		mode = BillingModeToken
	}
	p := ChannelModelPricing{Platform: PlatformImageVideo, Models: []string{model}, BillingMode: mode}
	switch mode {
	case BillingModeToken:
		p.OutputPrice = &price
	case BillingModeVideo:
		p.PerRequestPrice = &price
	default:
		p.PerRequestPrice = &price
	}
	return p
}
func mediaTestCapability() media.Capability {
	bytes, concurrent, observation := int64(33554432), 2, 3600
	cap := media.Capability{ID: "manual", Version: "1", Model: "00sd2.5", UpstreamModel: "00sd2.5", Adapter: "v6k_video_gateway", MediaType: "video", Enabled: true, Status: "enabled", ValidationStatus: "本项目实测通过", VerificationEvidence: "unit fixture", Operations: []string{"video"}, Formats: []string{"json"}}
	cap.Duration = media.Duration{Mode: "fixed", Values: []int{30}}
	cap.Delivery.PollSeconds = 10
	cap.Limits.MaxRequestBytes, cap.Limits.MaxConcurrentTasks, cap.Limits.ObservationSeconds = &bytes, &concurrent, &observation
	return cap
}
func TestUnifiedMediaPricingAndSnapshot(t *testing.T) {
	ctx := context.Background()
	cap := mediaTestCapability()
	sale := mediaTestPrice(cap.Model, "second", "fixed_duration", "0.04")
	cost := mediaTestPrice(cap.Model, "request", "task_count", "0.6")
	group := &Group{ID: 1, Platform: PlatformImageVideo, ModelPricing: []ChannelModelPricing{mediaTestPrice(cap.Model, "request", "task_count", "0")}}
	account := &Account{ID: 2, Platform: PlatformImageVideo, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Extra: map[string]any{"media_models": []media.Capability{cap}}}
	channel := Channel{ID: 3, Status: StatusActive, GroupIDs: []int64{1}, ModelPricing: []ChannelModelPricing{sale}, ApplyPricingToAccountStats: true}
	repo := makeStandardRepo(channel, map[int64]string{1: PlatformImageVideo})
	channels := NewChannelService(repo, nil, nil, nil, nil)
	e := &media.Engine{Config: UnifiedMediaConfig(unifiedMediaAccounts{list: []Account{*account}}, unifiedMediaGroups{group: group}, channels), Lookup: func(id, group int64) (media.Account, error) { return media.Account{ID: id, Allowed: true}, nil }}
	id := media.Identity{GroupID: 1, Multiplier: "2", TimeMultiplier: "1"}
	selected, err := e.Capability(ctx, cap.Model, "video", id)
	require.NoError(t, err)
	require.NotNil(t, selected)
	req, err := media.Normalize(*selected, "video", map[string]any{"model": cap.Model, "prompt": "test"})
	require.NoError(t, err)
	snap, _, quote, err := e.Snapshot(ctx, *selected, req, id)
	require.NoError(t, err)
	require.Equal(t, "0.00000000", quote.Total, "explicit free group pricing must override channel")
	require.Nil(t, snap.Cost, "retail-to-stats option must not fabricate upstream cost")
	oldVersion := snap.Sale.Version
	group.ModelPricing = nil
	snap, _, quote, err = e.Snapshot(ctx, *selected, req, id)
	require.NoError(t, err)
	require.Equal(t, "2.40000000", quote.Total, "30 seconds must not clamp to 15; multiplier applies once")
	require.NotEqual(t, oldVersion, snap.Sale.Version)
	require.Equal(t, "30", snap.Request.Duration)
	channel.AccountStatsPricingRules = []AccountStatsPricingRule{{SortOrder: 0, AccountIDs: []int64{2}, Pricing: []ChannelModelPricing{cost}}}
	repo.listAllFn = func(context.Context) ([]Channel, error) { return []Channel{channel}, nil }
	channels.clearCache()
	snap, _, _, err = e.Snapshot(ctx, *selected, req, id)
	require.NoError(t, err)
	require.NotNil(t, snap.Cost)
	require.Equal(t, "account_cost_once", snap.Cost.MultiplierPolicy)
	channel.ModelPricing = nil
	channels.clearCache()
	_, _, _, err = e.Snapshot(ctx, *selected, req, id)
	require.ErrorContains(t, err, "price_not_configured")
}
func TestMediaExactIDsAndValidation(t *testing.T) {
	p := mediaTestPrice("doubao-seedance-2-5/65Z", "request", "task_count", "0")
	require.NoError(t, validatePricingBillingMode([]ChannelModelPricing{p}))
	require.Nil(t, exactMediaPricing([]ChannelModelPricing{p}, "doubao-seedance-2-5/65z"))
	clone := p.Clone()
	clone.PerRequestPrice = float64Ptr(2)
	require.Equal(t, 0.0, *p.PerRequestPrice)
	require.NotEqual(t, mediaVersion(p), mediaVersion(clone))
	p.PerRequestPrice = nil
	require.Error(t, validatePricingBillingMode([]ChannelModelPricing{p}))
	cap := mediaTestCapability()
	cap.ID = ""
	cap.Version = ""
	require.NoError(t, ValidateMediaAccount(PlatformImageVideo, AccountTypeAPIKey, map[string]any{"media_models": []media.Capability{cap}}))
	require.Error(t, ValidateMediaAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]any{"media_models": []media.Capability{cap}}))
	require.Error(t, ValidateMediaAccount(PlatformImageVideo, AccountTypeOAuth, nil))
}

func TestMediaAccountUnchangedModelsDoNotRequireRepublishVerification(t *testing.T) {
	cap := mediaTestCapability()
	cap.ValidationStatus = "待渠道验证"
	cap.VerificationEvidence = ""
	cap.Enabled = true
	current := map[string]any{"media_models": []media.Capability{cap}}
	// The edit form serializes the complete Extra object before sending it back.
	currentJSON, err := json.Marshal(current)
	require.NoError(t, err)
	var persisted map[string]any
	require.NoError(t, json.Unmarshal(currentJSON, &persisted))
	requested := map[string]any{"media_models": persisted["media_models"], "custom_note": "unchanged"}
	require.NoError(t, validateMediaAccountUpdate(PlatformImageVideo, AccountTypeAPIKey, current, requested))
}

func TestMediaAccountEnableWithoutVerification(t *testing.T) {
	currentCap := mediaTestCapability()
	currentCap.ValidationStatus = "待渠道验证"
	currentCap.VerificationEvidence = ""
	currentCap.Enabled = false
	requestedCap := currentCap
	requestedCap.Enabled = true
	require.NoError(t, validateMediaAccountUpdate(
		PlatformImageVideo, AccountTypeAPIKey,
		map[string]any{"media_models": []media.Capability{currentCap}},
		map[string]any{"media_models": []media.Capability{requestedCap}},
	))
	requestedCap.Adapter = "unknown"
	require.Error(t, ValidateMediaAccount(PlatformImageVideo, AccountTypeAPIKey, map[string]any{"media_models": []media.Capability{requestedCap}}))
}

func TestStandardMediaPricingProjectsExistingEntries(t *testing.T) {
	model := "00sd2.5"
	perRequest := 1.25
	imageCard := standardMediaCard(ChannelModelPricing{
		Platform:        PlatformImageVideo,
		BillingMode:     BillingModePerRequest,
		PerRequestPrice: &perRequest,
	}, model, "image", "group", "1", "sale", "image")
	require.NoError(t, media.ValidatePriceCard(imageCard, true))
	require.Equal(t, "image", imageCard.MediaType)
	require.Len(t, imageCard.Rules, 1)
	require.Equal(t, "request", imageCard.Rules[0].Charges[0].Unit)
	require.Equal(t, "task_count", imageCard.Rules[0].Charges[0].QuantitySource)

	videoPrice := 0.04
	videoTier := 0.08
	videoCard := standardMediaCard(ChannelModelPricing{
		Platform:        PlatformImageVideo,
		BillingMode:     BillingModeVideo,
		PerRequestPrice: &videoPrice,
		Intervals:       []PricingInterval{{TierLabel: "720p", PerRequestPrice: &videoTier}},
	}, model, "video", "group", "1", "sale", "video")
	require.NoError(t, media.ValidatePriceCard(videoCard, true))
	require.Len(t, videoCard.Rules, 2)
	require.Equal(t, "720p", videoCard.Rules[0].When["resolution"])
	require.Equal(t, "second", videoCard.Rules[0].Charges[0].Unit)
	require.Equal(t, "request_duration", videoCard.Rules[0].Charges[0].QuantitySource)

	tierRule, err := media.MatchRule(videoCard, map[string]any{"resolution": "720p"})
	require.NoError(t, err)
	tierQuote, err := media.Calculate(tierRule, map[string][]string{"request_duration": {"30"}}, "1", "1", "success", nil)
	require.NoError(t, err)
	require.Equal(t, "2.40000000", tierQuote.Total)

	defaultRule, err := media.MatchRule(videoCard, map[string]any{"resolution": "1080p"})
	require.NoError(t, err)
	defaultQuote, err := media.Calculate(defaultRule, map[string][]string{"request_duration": {"30"}}, "1", "1", "success", nil)
	require.NoError(t, err)
	require.Equal(t, "1.20000000", defaultQuote.Total)
}

func TestUnifiedMediaConfigUsesOrdinaryPricingWithoutMediaJSON(t *testing.T) {
	ctx := context.Background()
	cap := mediaTestCapability()
	price, free := 0.04, 0.0
	sale := ChannelModelPricing{Platform: PlatformImageVideo, Models: []string{cap.Model}, BillingMode: BillingModeVideo, PerRequestPrice: &price}
	groupPrice := ChannelModelPricing{Platform: PlatformImageVideo, Models: []string{cap.Model}, BillingMode: BillingModeVideo, PerRequestPrice: &free}
	group := &Group{ID: 1, Platform: PlatformImageVideo, ModelPricing: []ChannelModelPricing{groupPrice}}
	account := &Account{ID: 2, Platform: PlatformImageVideo, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Extra: map[string]any{"media_models": []media.Capability{cap}}}
	channel := Channel{ID: 3, Status: StatusActive, GroupIDs: []int64{1}, ModelPricing: []ChannelModelPricing{sale}}
	repo := makeStandardRepo(channel, map[int64]string{1: PlatformImageVideo})
	channels := NewChannelService(repo, nil, nil, nil, nil)
	e := &media.Engine{Config: UnifiedMediaConfig(unifiedMediaAccounts{list: []Account{*account}}, unifiedMediaGroups{group: group}, channels), Lookup: func(id, group int64) (media.Account, error) { return media.Account{ID: id, Allowed: true}, nil }}

	selected, err := e.Capability(ctx, cap.Model, "video", media.Identity{GroupID: 1})
	require.NoError(t, err)
	require.NotNil(t, selected)
	req, err := media.Normalize(*selected, "video", map[string]any{"model": cap.Model, "prompt": "test"})
	require.NoError(t, err)
	_, _, quote, err := e.Snapshot(ctx, *selected, req, media.Identity{GroupID: 1, Multiplier: "1", TimeMultiplier: "1"})
	require.NoError(t, err)
	require.Equal(t, "0.00000000", quote.Total)
}

func TestStandardMediaTokenPricingRequiresProviderUsage(t *testing.T) {
	input, output := 0.000001, 0.000002
	card := standardMediaCard(ChannelModelPricing{
		Platform:    PlatformImageVideo,
		BillingMode: BillingModeToken,
		InputPrice:  &input,
		OutputPrice: &output,
	}, "image-model", "image", "group", "1", "sale", "image")
	require.NoError(t, media.ValidatePriceCard(card, true))
	require.Len(t, card.Rules[0].Charges, 2)
	require.Equal(t, "input_tokens", *card.Rules[0].Charges[0].UsageField)
	require.Equal(t, "output_tokens", *card.Rules[0].Charges[1].UsageField)

	calculation, err := media.Calculate(card.Rules[0], map[string][]string{"task_count": {"1"}}, "1", "1", "success", nil)
	require.NoError(t, err)
	require.Contains(t, calculation.Missing, "provider_tokens.input_tokens")
	require.Contains(t, calculation.Missing, "provider_tokens.output_tokens")
}

func TestMediaMultiAccountRequestSelection(t *testing.T) {
	a := mediaTestCapability()
	a.AccountIDs = []int64{1}
	a.GroupIDs = []int64{1}
	a.Duration.Values = []int{5}
	b := a
	b.AccountIDs = []int64{2}
	b.Duration.Values = []int{30}
	e := &media.Engine{Config: func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
		return []media.Capability{a, b}, nil, nil
	}}
	c, err := e.SelectCapability(context.Background(), a.Model, "video", map[string]any{"prompt": "test", "seconds": 30}, nil, media.Identity{GroupID: 1})
	require.NoError(t, err)
	require.Equal(t, []int64{1}, c.AccountIDs, "catalog duration hints do not veto the first eligible account")
	req, err := media.Normalize(*c, "video", map[string]any{"prompt": "test", "seconds": 30})
	require.NoError(t, err)
	require.Equal(t, []string{"30"}, req.Quantities["request_duration"])
	c, err = e.SelectCapability(context.Background(), a.Model, "video", map[string]any{"prompt": "test", "seconds": 15}, nil, media.Identity{GroupID: 1})
	require.NoError(t, err)
	require.Equal(t, []int64{1}, c.AccountIDs)
	a.Enabled = false
	c, err = e.SelectCapability(context.Background(), a.Model, "video", map[string]any{"prompt": "test", "seconds": 30}, nil, media.Identity{GroupID: 1})
	require.NoError(t, err)
	require.Equal(t, []int64{2}, c.AccountIDs, "disabled accounts are still excluded")
	_, err = e.SelectCapability(context.Background(), a.Model, "video", map[string]any{"prompt": "test", "seconds": 0}, nil, media.Identity{GroupID: 1})
	require.ErrorContains(t, err, "invalid_seconds")
	_, err = media.ResolvePrice(nil, a, media.Identity{GroupID: 1}, "video", false, 1, time.Now())
	require.Error(t, err)
}

func TestMediaCatalogIncludesAllAccountVariants(t *testing.T) {
	a := mediaTestCapability()
	a.AccountIDs, a.GroupIDs, a.Resolutions = []int64{1}, []int64{1}, []string{"720p"}
	a.Duration.Values = []int{5}
	b := a
	b.AccountIDs, b.Resolutions, b.Duration.Values = []int64{2}, []string{"1080p"}, []int{30}
	p := mediaCardForType(mediaTestPrice(a.Model, "second", "request_duration", "0.04"), a.Model, "video", "group", "1", "sale", "video")
	e := &media.Engine{Config: func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
		return []media.Capability{a, b}, []media.PriceCard{p}, nil
	}}
	catalog, err := e.Catalog(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.Equal(t, []string{"720p", "1080p"}, catalog[0].Resolutions)
	require.Len(t, catalog[0].DurationVariants, 2)
	require.Len(t, catalog[0].Prices, 1)
}
