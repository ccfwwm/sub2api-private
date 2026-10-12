package media

import (
	"context"
	"reflect"
	"time"
)

type CatalogPrice struct {
	Operation      string  `json:"operation"`
	Version        string  `json:"version"`
	Rules          []Rule  `json:"rules"`
	MaximumCharge  *string `json:"maximum_charge"`
	TimeMultiplier string  `json:"time_multiplier,omitempty"`
}
type CatalogEntry struct {
	Model            string         `json:"model"`
	MediaType        string         `json:"media_type"`
	Operations       []string       `json:"operations"`
	Duration         Duration       `json:"duration"`
	DurationVariants []Duration     `json:"duration_variants,omitempty"`
	Resolutions      []string       `json:"resolutions"`
	Prices           []CatalogPrice `json:"prices"`
	MusicMultiplier  *string        `json:"music_multiplier,omitempty"`
}

func (e *Engine) Catalog(ctx context.Context, groupID int64) ([]CatalogEntry, error) {
	caps, cards, err := e.configuration(ctx, groupID)
	if err != nil {
		return nil, err
	}
	list := []CatalogEntry{}
	seen := map[string]int{}
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
				entry.Prices = append(entry.Prices, CatalogPrice{Operation: op, Version: price.Version, Rules: price.Rules, MaximumCharge: price.Reservation.MaximumCharge, TimeMultiplier: price.TimeMultiplier})
			}
		}
		if len(entry.Operations) == 0 {
			continue
		}
		if index, exists := seen[entry.Model]; exists {
			current := &list[index]
			for i, op := range entry.Operations {
				if !contains(current.Operations, op) {
					current.Operations = append(current.Operations, op)
					current.Prices = append(current.Prices, entry.Prices[i])
				}
			}
			for _, resolution := range entry.Resolutions {
				if !contains(current.Resolutions, resolution) {
					current.Resolutions = append(current.Resolutions, resolution)
				}
			}
			if !reflect.DeepEqual(current.Duration, entry.Duration) {
				if len(current.DurationVariants) == 0 {
					current.DurationVariants = []Duration{current.Duration}
				}
				found := false
				for _, duration := range current.DurationVariants {
					found = found || reflect.DeepEqual(duration, entry.Duration)
				}
				if !found {
					current.DurationVariants = append(current.DurationVariants, entry.Duration)
				}
			}
		} else {
			seen[entry.Model] = len(list)
			list = append(list, entry)
		}
	}
	return list, nil
}
