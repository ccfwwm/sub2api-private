//go:build unit

package service

import (
	"context"
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
	p := ChannelModelPricing{Platform: PlatformImageVideo, Models: []string{model}, BillingMode: BillingModePerRequest, Media: &MediaPricing{MediaType: "video", SuccessPolicy: "any_output"}}
	p.Media.Rules = []media.Rule{{Priority: 0, When: map[string]any{}, Charges: []media.Charge{{Name: "generation", Unit: unit, Price: &value, QuantitySource: source, Trigger: "success", Refundable: true, Method: "linear", Rounding: media.Rounding{Mode: "none", Step: "1", Minimum: "0", Scope: "aggregate"}}}}}
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
	require.NoError(t, ValidateMediaPricing(p))
	require.Nil(t, exactMediaPricing([]ChannelModelPricing{p}, "doubao-seedance-2-5/65z"))
	clone := p.Clone()
	*clone.Media.Rules[0].Charges[0].Price = "2"
	require.Equal(t, "0", *p.Media.Rules[0].Charges[0].Price)
	require.NotEqual(t, mediaVersion(p), mediaVersion(clone))
	p.Media.Rules[0].Charges[0].Price = nil
	require.Error(t, ValidateMediaPricing(p))
	cap := mediaTestCapability()
	cap.ID = ""
	cap.Version = ""
	require.NoError(t, ValidateMediaAccount(PlatformImageVideo, AccountTypeAPIKey, map[string]any{"media_models": []media.Capability{cap}}))
	require.Error(t, ValidateMediaAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]any{"media_models": []media.Capability{cap}}))
	require.Error(t, ValidateMediaAccount(PlatformImageVideo, AccountTypeOAuth, nil))
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
	require.Equal(t, []int64{2}, c.AccountIDs)
	_, err = e.SelectCapability(context.Background(), a.Model, "video", map[string]any{"prompt": "test", "seconds": 15}, nil, media.Identity{GroupID: 1})
	require.Error(t, err)
	_, err = media.ResolvePrice(nil, a, media.Identity{GroupID: 1}, "video", false, 1, time.Now())
	require.Error(t, err)
}

func TestMediaCatalogIncludesAllAccountVariants(t *testing.T) {
	a := mediaTestCapability()
	a.AccountIDs, a.GroupIDs, a.Resolutions = []int64{1}, []int64{1}, []string{"720p"}
	a.Duration.Values = []int{5}
	b := a
	b.AccountIDs, b.Resolutions, b.Duration.Values = []int64{2}, []string{"1080p"}, []int{30}
	p := mediaCard(mediaTestPrice(a.Model, "second", "request_duration", "0.04"), a.Model, "video", "group", "1", "sale")
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
