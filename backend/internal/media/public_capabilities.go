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
		field, shape := "images", "array"
		switch cap.Adapter {
		case "yunfei_gpt_images":
			field = "image"
		case "yunfei_grok":
			field, shape = "image", "url_object"
		case "yunfei_omni":
			field, shape = "image", "single"
		case "yunfei_veo_preview":
			field = "image_urls"
		case "yunfei_gemini_native":
			field, shape = "contents", "gemini_parts"
		case "ark_native", "v6k_video_gateway":
			field, shape = "metadata", "native_content"
		}
		format := cap.RequestFormat
		if format == "" {
			format = "legacy"
		}
		if format == "yunfei_multimodal" {
			field, shape = "reference_images", "array"
		}
		if format == "v6k_url" {
			field, shape = "images", "url_object_array"
		}
		// Duration scenarios contain configuration-specific arbitrary maps. Only
		// the typed scalar bounds are public; no internal maps cross this boundary.
		duration := cap.Duration
		duration.Scenarios = nil
		materials := map[string]Material{}
		for kind, material := range cap.Materials {
			materials[kind] = material
		}
		if cap.Adapter == "yunfei_gpt_images" && contains(ops, "image_edit") && contains(cap.Formats, "multipart") {
			one := 1
			materials["mask"] = Material{Formats: []string{"file_png"}, MaxCount: &one, MaxFileBytes: cap.Materials["image"].MaxFileBytes}
		}
		out = append(out, PublicCapability{ProfileID: publicProfileID(cap), Version: cap.Version, Model: cap.Model, MediaType: cap.MediaType, Operations: ops, RequestFormat: format, ReferenceField: field, ReferenceShape: shape, Duration: duration, Sizes: cap.Sizes, Resolutions: cap.Resolutions, AspectRatios: cap.AspectRatios, Qualities: cap.Qualities, Formats: cap.Formats, Materials: materials, PollSeconds: cap.Delivery.PollSeconds, MaxRequestBytes: cap.Limits.MaxRequestBytes, ObservationSeconds: cap.Limits.ObservationSeconds})
	}
	return out, nil
}

func publicProfileID(cap Capability) string {
	return fmt.Sprintf("mp_%x", sha256.Sum256([]byte(cap.ID+"\x00"+cap.Version)))
}
