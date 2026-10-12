package media

import (
	"context"
	"time"
)

type CatalogPrice struct {
	Operation     string  `json:"operation"`
	Version       string  `json:"version"`
	Rules         []Rule  `json:"rules"`
	MaximumCharge *string `json:"maximum_charge"`
}
type CatalogEntry struct {
	Model           string         `json:"model"`
	MediaType       string         `json:"media_type"`
	Operations      []string       `json:"operations"`
	Duration        Duration       `json:"duration"`
	Resolutions     []string       `json:"resolutions"`
	Prices          []CatalogPrice `json:"prices"`
	MusicMultiplier *string        `json:"music_multiplier,omitempty"`
}

func (e *Engine) Catalog(ctx context.Context, groupID int64) ([]CatalogEntry, error) {
	caps, cards, err := e.Store.Published(ctx)
	if err != nil {
		return nil, err
	}
	list := []CatalogEntry{}
	for _, cap := range caps {
		if !cap.Enabled || cap.Status != "enabled" {
			continue
		}
		belongs := false
		for _, g := range cap.GroupIDs {
			if g == groupID {
				belongs = true
			}
		}
		if !belongs {
			continue
		}
		entry := CatalogEntry{Model: cap.Model, MediaType: cap.MediaType, Operations: []string{}, Duration: cap.Duration, Resolutions: cap.Resolutions, Prices: []CatalogPrice{}, MusicMultiplier: cap.MusicRateOverride}
		for _, op := range cap.Operations {
			price, err := ResolvePrice(cards, cap, Identity{GroupID: groupID}, op, false, 0, time.Now())
			if err == nil {
				entry.Operations = append(entry.Operations, op)
				entry.Prices = append(entry.Prices, CatalogPrice{op, price.Version, price.Rules, price.Reservation.MaximumCharge})
			}
		}
		if len(entry.Operations) > 0 {
			list = append(list, entry)
		}
	}
	return list, nil
}
