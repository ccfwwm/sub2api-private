package media

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"
)

// PublicCapability is an allowlisted projection. Never expose a Capability itself:
// its account/channel bindings, upstream names and cost templates are private.
// Entries remain separate so clients cannot combine incompatible profiles.
type PublicCapability struct {
	ProfileID          string              `json:"profile_id"`
	Version            string              `json:"version"`
	Model              string              `json:"model"`
	MediaType          string              `json:"media_type"`
	Operations         []string            `json:"operations"`
	RequestFormat      string              `json:"request_format"`
	ReferenceField     string              `json:"reference_field"`
	ReferenceShape     string              `json:"reference_shape"`
	Duration           Duration            `json:"duration"`
	Sizes              []string            `json:"sizes"`
	Resolutions        []string            `json:"resolutions"`
	AspectRatios       []string            `json:"aspect_ratios"`
	Qualities          []string            `json:"qualities"`
	Backgrounds        []string            `json:"backgrounds,omitempty"`
	OutputFormats      []string            `json:"output_formats,omitempty"`
	Formats            []string            `json:"formats"`
	Materials          map[string]Material `json:"materials"`
	PollSeconds        int                 `json:"poll_seconds"`
	MaxRequestBytes    *int64              `json:"max_request_bytes"`
	ObservationSeconds *int                `json:"observation_seconds"`
}

func (e *Engine) PublicCapabilities(ctx context.Context, groupID int64) ([]PublicCapability, error) {
	caps, cards, err := e.configuration(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := []PublicCapability{}
	for _, cap := range caps {
		ops := []string{}
		for _, op := range cap.Operations {
			if !cap.Matches(cap.Model, op, groupID) {
				continue
			}
			if _, err := ResolvePrice(cards, cap, Identity{GroupID: groupID}, op, false, 0, time.Now()); err == nil {
				ops = append(ops, op)
			}
		}
		if len(ops) == 0 {
			continue
		}
		field, shape, format := "images", "array", "unified"
		// Duration scenarios contain configuration-specific arbitrary maps. Only
		// the typed scalar bounds are public; no internal maps cross this boundary.
		duration := cap.Duration
		duration.Scenarios = nil
		materials := map[string]Material{}
		for _, kind := range []string{"image", "video", "audio"} {
			max := int64(50 << 20)
			materials[kind] = Material{ReferenceField: kind + "s", Formats: []string{"public_url_string", "public_url_object", "data_url", "data_url_in_url_object", "data_url_in_b64_json_object", "base64", "base64_in_url_object", "base64_in_b64_json_object"}, MaxFileBytes: &max}
		}
		out = append(out, PublicCapability{ProfileID: publicProfileID(cap), Version: publicProfileID(cap), Model: cap.Model, MediaType: cap.MediaType, Operations: ops, RequestFormat: format, ReferenceField: field, ReferenceShape: shape, Duration: duration, Sizes: cap.Sizes, Resolutions: cap.Resolutions, AspectRatios: cap.AspectRatios, Qualities: cap.Qualities, Formats: []string{"json", "multipart"}, Materials: materials, PollSeconds: cap.Delivery.PollSeconds, MaxRequestBytes: cap.Limits.MaxRequestBytes, ObservationSeconds: cap.Limits.ObservationSeconds})
		out[len(out)-1].Backgrounds = cap.Backgrounds
		out[len(out)-1].OutputFormats = cap.OutputFormats
	}
	return out, nil
}

func publicProfileID(cap Capability) string {
	return fmt.Sprintf("mp_%x", sha256.Sum256([]byte(cap.ID+"\x00"+cap.Version)))
}
