package media

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
func ValidateCapability(c Capability, publish bool) error {
	if c.ID == "" || c.Model == "" || c.UpstreamModel == "" || c.Version == "" {
		return fail(400, "invalid_capability", "Identity and exact model are required")
	}
	if !contains([]string{"yunfei_gpt_images", "yunfei_gemini_native", "yunfei_grok", "yunfei_veo_preview", "yunfei_omni", "yunfei_lyria", "v6k_video_gateway", "ark_native"}, c.Adapter) {
		return fail(400, "invalid_adapter", "Unknown adapter")
	}
	if c.MediaType != "image" && c.MediaType != "video" && c.MediaType != "audio" {
		return fail(400, "invalid_media_type", "Unknown media type")
	}
	for _, op := range c.Operations {
		if !contains([]string{"image", "image_edit", "gemini_image", "video", "audio", "chat_music"}, Operation(op)) {
			return fail(400, "invalid_operation", "Unknown operation")
		}
	}
	for target, path := range c.UsageFields {
		if target != "provider_duration" && !contains([]string{"provider_tokens.total_tokens", "provider_tokens.input_tokens", "provider_tokens.output_tokens", "provider_tokens.image_input_tokens", "provider_tokens.image_output_tokens"}, target) {
			return fail(400, "invalid_usage_mapping", "Unsupported usage quantity")
		}
		if !regexp.MustCompile(`^(usage|usageMetadata|billing_usage)\.[A-Za-z_][A-Za-z0-9_.]*$`).MatchString(path) {
			return fail(400, "invalid_usage_mapping", "Usage must reference a known usage object")
		}
	}
	if c.MusicRateOverride != nil {
		if _, e := number(*c.MusicRateOverride); e != nil {
			return e
		}
	}
	if publish {
		if len(c.AccountIDs) == 0 || len(c.GroupIDs) == 0 || len(c.Operations) == 0 {
			return fail(400, "missing_binding", "Group, account and operation bindings required")
		}
		if c.ValidationStatus != "本项目实测通过" || c.VerificationEvidence == "" {
			return fail(422, "channel_verification_required", "Record real-channel verification before publishing")
		}
		if c.Limits.MaxRequestBytes == nil || *c.Limits.MaxRequestBytes <= 0 || *c.Limits.MaxRequestBytes > 256<<20 || c.Limits.MaxConcurrentTasks == nil || *c.Limits.MaxConcurrentTasks <= 0 || c.Limits.ObservationSeconds == nil || *c.Limits.ObservationSeconds <= 0 {
			return fail(400, "invalid_limits", "Finite request, concurrency and observation limits required")
		}
		if c.MediaType == "video" && (c.Duration.Mode == "unknown" || c.Duration.Mode == "") {
			return fail(422, "duration_not_configured", "Video duration limits must be configured")
		}
		if c.Adapter == "yunfei_gpt_images" && len(c.Sizes) == 0 {
			return fail(422, "image_sizes_required", "Configure verified pixel sizes before publishing")
		}
		if c.Delivery.PollSeconds < 1 || c.Delivery.PollSeconds > 300 {
			return fail(400, "invalid_poll_interval", "Query interval must be 1 to 300 seconds")
		}
		for kind, material := range c.Materials {
			if material.MaxCount != nil && *material.MaxCount > 0 {
				if material.MaxFileBytes == nil || *material.MaxFileBytes <= 0 || *material.MaxFileBytes > 256<<20 {
					return fail(400, "invalid_file_limit", "Enabled materials need bounded file sizes")
				}
				if kind != "image" && material.MaxDuration == nil {
					return fail(400, "input_duration_limit_required", "Input audio/video requires a duration bound")
				}
			}
		}
	}
	if c.Duration.Mode == "fixed" && (len(c.Duration.Values) != 1 || c.Duration.Values[0] <= 0) {
		return fail(400, "invalid_duration", "Fixed duration requires one positive value")
	}
	if c.Duration.Mode == "range" && (c.Duration.Min == nil || c.Duration.Max == nil || *c.Duration.Min <= 0 || *c.Duration.Max < *c.Duration.Min) {
		return fail(400, "invalid_duration", "Positive ordered duration bounds required")
	}
	if c.Duration.Mode == "enum" && len(c.Duration.Values) == 0 {
		return fail(400, "invalid_duration", "Duration values required")
	}
	return nil
}
func alias(p map[string]any, a, b string) error {
	x, xok := p[a]
	y, yok := p[b]
	if xok && yok && !reflect.DeepEqual(x, y) && stringValue(x) != stringValue(y) {
		return fail(400, "alias_conflict", a+" conflicts with "+b)
	}
	if !xok && yok {
		p[a] = y
	}
	delete(p, b)
	return nil
}

var integer = regexp.MustCompile(`^[1-9][0-9]*$`)

func requestSeconds(v any) (int, error) {
	s := stringValue(v)
	if !integer.MatchString(s) {
		return 0, fail(400, "invalid_seconds", "Duration must be a positive integer")
	}
	n, e := strconv.Atoi(s)
	if e != nil || n > 3600 {
		return 0, fail(400, "invalid_seconds", "Duration exceeds validator bounds")
	}
	return n, nil
}
func orderedValues(v any) ([]any, error) {
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fail(400, "invalid_references", "References must be ordered arrays")
	}
	return list, nil
}
func refString(v any) (string, string, error) {
	if s, ok := v.(string); ok {
		return s, "string", nil
	}
	if o, ok := v.(map[string]any); ok {
		s, ok := o["url"].(string)
		if !ok || s == "" {
			return "", "", fail(400, "invalid_reference", "Reference object requires url")
		}
		for k := range o {
			if k != "url" && k != "duration_seconds" {
				return "", "", fail(400, "invalid_reference", "Unknown reference object field")
			}
		}
		return s, "object", nil
	}
	return "", "", fail(400, "invalid_reference", "URL string or object required")
}
func referenceFormat(raw, form string) (string, error) {
	if strings.HasPrefix(raw, "data:") {
		pos := strings.Index(raw, ",")
		if pos < 0 || !strings.Contains(raw[:pos], ";base64") {
			return "", fail(400, "invalid_reference", "Base64 Data URL required")
		}
		if _, e := base64.StdEncoding.DecodeString(raw[pos+1:]); e != nil {
			return "", fail(400, "invalid_reference", "Invalid Base64")
		}
		if form == "object" {
			return "data_url_in_url_object", nil
		}
		return "data_url", nil
	}
	if strings.HasPrefix(raw, "https://") {
		u, e := url.Parse(raw)
		if e != nil || u.Hostname() == "" || u.User != nil {
			return "", fail(400, "invalid_reference", "Invalid public URL")
		}
		if form == "object" {
			return "public_url_object", nil
		}
		return "public_url_string", nil
	}
	if _, e := base64.StdEncoding.DecodeString(raw); e == nil && len(raw) > 0 {
		if form == "object" {
			return "base64_in_url_object", nil
		}
		return "base64", nil
	}
	return "", fail(400, "invalid_reference", "HTTPS URL, validated Data URL or Base64 required")
}
func allowedFormat(m Material, f string) bool {
	if contains(m.Formats, f) {
		return true
	}
	if f == "public_url_string" && contains(m.Formats, "url_string") {
		return true
	}
	if f == "public_url_object" && contains(m.Formats, "url_object") {
		return true
	}
	return false
}

// Normalize rejects billing-sensitive mistakes before price resolution or holds.
func Normalize(cap Capability, operation string, p map[string]any) (Request, error) {
	result := Request{Operation: Operation(operation), Model: cap.Model, Parameters: map[string]any{}, Conditions: map[string]any{}, Quantities: map[string][]string{}}
	b, _ := json.Marshal(p)
	if e := json.Unmarshal(b, &result.Parameters); e != nil {
		return result, e
	}
	p = result.Parameters
	allowed := map[string]bool{}
	for _, k := range []string{"model", "prompt", "seconds", "duration", "ratio", "aspect_ratio", "resolution", "size", "quality", "n", "response_format", "image", "images", "image_url", "image_urls", "input_reference", "videos", "audios", "reference_image_urls", "reference_video_urls", "reference_audio_urls", "generate_audio", "generateAudio", "negative_prompt", "negativePrompt", "metadata", "title", "lyrics", "instrumental", "bpm", "seed", "async", "audio_format", "contents", "generationConfig", "messages", "stream"} {
		allowed[k] = true
	}
	for k := range p {
		if !allowed[k] {
			return result, fail(400, "unknown_parameter", "Unsupported field: "+k)
		}
	}
	if stringValue(p["model"]) != "" && stringValue(p["model"]) != cap.Model {
		return result, fail(400, "model_conflict", "Request model conflicts with capability")
	}
	p["model"] = cap.UpstreamModel
	for _, ab := range [][2]string{{"seconds", "duration"}, {"ratio", "aspect_ratio"}, {"generate_audio", "generateAudio"}, {"negative_prompt", "negativePrompt"}, {"images", "reference_image_urls"}, {"videos", "reference_video_urls"}, {"audios", "reference_audio_urls"}} {
		if e := alias(p, ab[0], ab[1]); e != nil {
			return result, e
		}
	}
	if cap.MediaType == "video" {
		if _, present := p["seconds"]; !present && cap.Duration.Required && cap.Duration.Mode != "fixed" {
			return result, fail(400, "seconds_required", "This model requires an explicit duration")
		}
		seconds := 0
		if v, ok := p["seconds"]; ok {
			n, e := requestSeconds(v)
			if e != nil {
				return result, e
			}
			seconds = n
		} else if cap.Duration.Default != nil {
			seconds = *cap.Duration.Default
		} else if cap.Duration.Mode == "fixed" && len(cap.Duration.Values) == 1 {
			seconds = cap.Duration.Values[0]
		} else {
			return result, fail(400, "seconds_required", "Choose a configured duration")
		}
		switch cap.Duration.Mode {
		case "fixed", "enum":
			found := false
			for _, v := range cap.Duration.Values {
				if v == seconds {
					found = true
				}
			}
			if !found {
				return result, fail(400, "unsupported_duration", "Duration not supported by this model")
			}
		case "range":
			if cap.Duration.Min == nil || cap.Duration.Max == nil || seconds < *cap.Duration.Min || seconds > *cap.Duration.Max {
				return result, fail(400, "unsupported_duration", "Duration outside model limits")
			}
		default:
			return result, fail(422, "duration_not_configured", "Configure model duration")
		}
		p["seconds"] = seconds
		result.Duration = strconv.Itoa(seconds)
		result.Quantities["request_duration"] = []string{result.Duration}
		if cap.Duration.Mode == "fixed" {
			result.Quantities["fixed_duration"] = []string{result.Duration}
		}
		if e := alias(p, "resolution", "size"); e != nil {
			return result, e
		}
	}
	for field, choices := range map[string][]string{"resolution": cap.Resolutions, "ratio": cap.AspectRatios, "quality": cap.Qualities} {
		if v, ok := p[field]; ok {
			if field == "resolution" && cap.ResolutionPolicy == "omit" {
				return result, fail(400, "unsupported_resolution", "Omit resolution for this model")
			}
			if len(choices) == 0 || !contains(choices, stringValue(v)) {
				return result, fail(400, "unsupported_capability", "Unsupported "+field)
			}
			result.Conditions[field] = v
		}
	}
	if cap.MediaType == "image" {
		if v, ok := p["size"]; ok {
			if len(cap.Sizes) > 0 && !contains(cap.Sizes, stringValue(v)) {
				return result, fail(400, "unsupported_size", "Image pixel size is not configured")
			}
			result.Conditions["size_tier"] = v
		}
		if n, ok := p["n"]; ok && stringValue(n) != "1" {
			return result, fail(400, "unsupported_output_count", "Configure output-count capability before requesting multiple outputs")
		}
	}
	refs := map[string][]any{"image": {}, "video": {}, "audio": {}}
	if operation == "chat_music" {
		if e := musicChat(p); e != nil {
			return result, e
		}
	}
	if operation == "gemini_image" {
		images, e := geminiReferences(p, cap, result.Conditions)
		if e != nil {
			return result, e
		}
		refs["image"] = images
	}
	for kind, field := range map[string]string{"image": "images", "video": "videos", "audio": "audios"} {
		list, e := orderedValues(p[field])
		if e != nil {
			return result, e
		}
		refs[kind] = append(refs[kind], list...)
	}
	if cap.Adapter == "v6k_video_gateway" {
		if p["input_reference"] != nil {
			if len(refs["image"]) > 0 {
				return result, fail(400, "reference_conflict", "Choose images or input_reference")
			}
			v, e := orderedValues(p["input_reference"])
			if e != nil {
				v = []any{p["input_reference"]}
			}
			refs["image"] = v
		}
		if p["image"] != nil {
			if len(refs["image"]) > 0 {
				return result, fail(400, "reference_conflict", "Choose one image field")
			}
			refs["image"] = []any{p["image"]}
		}
	}
	switch cap.Adapter {
	case "yunfei_grok":
		if p["image_url"] != nil || p["image_urls"] != nil || p["images"] != nil || p["input_reference"] != nil {
			return result, fail(400, "wrong_image_field", "Grok JSON requires image.url")
		}
		if p["image"] != nil {
			refs["image"] = append(refs["image"], p["image"])
		}
	case "yunfei_gpt_images":
		if p["image"] != nil {
			if p["images"] != nil {
				return result, fail(400, "reference_conflict", "Choose one image field")
			}
			list, e := orderedValues(p["image"])
			if e != nil {
				list = []any{p["image"]}
			}
			refs["image"] = list
		}
		if p["image_url"] != nil || p["image_urls"] != nil || p["input_reference"] != nil {
			return result, fail(400, "wrong_image_field", "Image edits require image or images")
		}
		if operation == "image" && len(refs["image"]) > 0 {
			return result, fail(400, "edit_endpoint_required", "Use image edits for reference images")
		}
	case "yunfei_omni":
		if p["image_url"] != nil || p["image_urls"] != nil || p["negative_prompt"] != nil || p["generate_audio"] != nil {
			return result, fail(400, "unsupported_parameter", "Omni uses image/images and has no audio/negative control")
		}
		if p["image"] != nil {
			if p["images"] != nil {
				return result, fail(400, "reference_conflict", "Choose image or images")
			}
			refs["image"] = append(refs["image"], p["image"])
		}
	case "yunfei_veo_preview":
		if p["image"] != nil || p["images"] != nil {
			return result, fail(400, "wrong_image_field", "Veo preview uses image_url/image_urls")
		}
		if p["image_url"] != nil && p["image_urls"] != nil {
			return result, fail(400, "reference_conflict", "Choose image_url or image_urls")
		}
		if p["image_url"] != nil {
			refs["image"] = append(refs["image"], p["image_url"])
		}
		if p["image_urls"] != nil {
			list, e := orderedValues(p["image_urls"])
			if e != nil {
				return result, e
			}
			refs["image"] = list
			if !strings.HasSuffix(cap.Model, "-ref") && len(list) != 2 {
				return result, fail(400, "invalid_frames", "Exactly two ordered first/last frames required")
			}
		}
	}
	if operation == "image_edit" && len(refs["image"]) == 0 {
		return result, fail(400, "image_required", "Image editing needs a reference image")
	}
	if meta, ok := p["metadata"].(map[string]any); ok {
		for _, k := range []string{"model", "seconds", "duration", "price", "billing", "user_id", "account_id", "group_id", "owner"} {
			if meta[k] != nil {
				return result, fail(400, "protected_metadata", "Metadata cannot override identity or billing")
			}
		}
		if meta["content"] != nil {
			if cap.Adapter != "v6k_video_gateway" && cap.Adapter != "ark_native" {
				return result, fail(400, "unsupported_content", "Native content is not supported by this adapter")
			}
			for _, v := range refs {
				if len(v) > 0 {
					return result, fail(400, "reference_conflict", "Native content cannot duplicate generic references")
				}
			}
			content, e := nativeReferences(meta["content"])
			if e != nil {
				return result, e
			}
			refs = content
		}
	}
	for kind, list := range refs {
		material, ok := cap.Materials[kind]
		if len(list) > 0 && (!ok || material.MaxCount == nil || len(list) > *material.MaxCount) {
			return result, fail(400, "reference_limit", "Reference type/count is not configured or exceeded")
		}
		for _, v := range list {
			raw, form, e := refString(v)
			if e != nil {
				return result, e
			}
			f, e := referenceFormat(raw, form)
			if e != nil {
				return result, e
			}
			if !allowedFormat(material, f) {
				return result, fail(400, "unsupported_reference_format", "Unsupported "+kind+" format")
			}
			if material.MaxFileBytes == nil && strings.Contains(f, "base64") || material.MaxFileBytes == nil && strings.HasPrefix(f, "data_url") {
				return result, fail(422, "file_limit_not_configured", "Configure maximum decoded file size")
			}
			if !strings.HasPrefix(f, "public_url") {
				encoded := raw
				if pos := strings.Index(raw, ","); strings.HasPrefix(raw, "data:") && pos >= 0 {
					encoded = raw[pos+1:]
				}
				decoded, e := base64.StdEncoding.DecodeString(encoded)
				if e != nil || material.MaxFileBytes == nil || int64(len(decoded)) > *material.MaxFileBytes {
					return result, fail(413, "file_too_large", "Decoded media exceeds configured file limit")
				}
			}
		}
		if kind == "image" {
			result.InputImages = len(list)
			result.Quantities["accepted_input_images"] = []string{fmt.Sprint(len(list))}
		}
		if kind == "video" || kind == "audio" {
			if len(list) == 0 {
				result.Quantities["accepted_input_"+kind+"_duration"] = []string{"0"}
			}
		}
	}
	if cap.Model == "video-QY" && len(refs["video"]) == 0 {
		return result, fail(400, "video_reference_required", "Video editing requires a reference video")
	}
	if strings.HasSuffix(cap.Model, "-generate-preview-ref") && len(refs["image"]) > 0 && (result.Duration != "8" || stringValue(p["ratio"]) != "16:9") {
		return result, fail(400, "reference_scenario_conflict", "Reference Veo requires 8 seconds and 16:9")
	}
	if cap.Adapter == "yunfei_grok" && len(refs["image"]) > 0 && p["ratio"] != nil {
		return result, fail(400, "image_aspect_is_fixed", "Image video follows the reference aspect ratio; omit ratio")
	}
	if cap.MediaType == "audio" {
		prompt := strings.TrimSpace(stringValue(p["prompt"]))
		if utf8.RuneCountInString(prompt) == 0 || utf8.RuneCountInString(prompt) > 10000 {
			return result, fail(400, "invalid_prompt", "Music prompt length must be 1–10000")
		}
		if utf8.RuneCountInString(stringValue(p["title"])) > 160 || utf8.RuneCountInString(stringValue(p["lyrics"])) > 20000 {
			return result, fail(400, "music_text_limit", "Title or lyrics too long")
		}
		if instrumental, ok := p["instrumental"].(bool); ok && instrumental {
			delete(p, "lyrics")
		}
		for _, field := range []string{"instrumental", "async", "stream"} {
			if v, ok := p[field]; ok {
				if _, good := v.(bool); !good {
					return result, fail(400, "invalid_music_parameter", field+" must be boolean")
				}
			}
		}
		for field, bounds := range map[string][2]int{"seconds": {5, 300}, "bpm": {30, 300}, "seed": {0, 2147483647}, "n": {1, 1}} {
			if v, ok := p[field]; ok {
				n, e := strconv.ParseInt(stringValue(v), 10, 64)
				if e != nil || n < int64(bounds[0]) || n > int64(bounds[1]) {
					return result, fail(400, "invalid_music_parameter", "Invalid "+field)
				}
			}
		}
		if v, ok := p["audio_format"]; ok && !contains([]string{"mp3", "wav", "m4a"}, stringValue(v)) {
			return result, fail(400, "invalid_music_parameter", "Unsupported audio format")
		}
	} else if operation != "gemini_image" && operation != "chat_music" && strings.TrimSpace(stringValue(p["prompt"])) == "" && (cap.Adapter != "yunfei_grok" || len(refs["image"]) == 0) {
		return result, fail(400, "prompt_required", "Nonempty prompt required")
	}
	result.Conditions["scene"] = "text"
	if len(refs["image"]) == 1 {
		result.Conditions["scene"] = "image"
	}
	if len(refs["image"]) > 1 {
		result.Conditions["scene"] = "references"
	}
	if cap.Adapter == "yunfei_veo_preview" && !strings.HasSuffix(cap.Model, "-ref") && len(refs["image"]) == 2 {
		result.Conditions["scene"] = "frames"
	}
	if len(refs["video"]) > 0 {
		result.Conditions["scene"] = "edit"
	}
	result.Conditions["seconds"] = result.Duration
	if cap.MediaType == "audio" && p["seconds"] != nil {
		result.Duration = stringValue(p["seconds"])
		result.Quantities["request_duration"] = []string{result.Duration}
		p["duration"] = p["seconds"]
		delete(p, "seconds")
	}
	if v, ok := p["response_format"]; ok && !contains([]string{"url", "b64_json"}, stringValue(v)) {
		return result, fail(400, "invalid_response_format", "url or b64_json required")
	}
	result.MaterialRefs = refs
	return result, nil
}

func nativeReferences(raw any) (map[string][]any, error) {
	refs := map[string][]any{"image": {}, "video": {}, "audio": {}}
	items, e := orderedValues(raw)
	if e != nil || len(items) == 0 {
		return refs, fail(400, "invalid_content", "Ordered native content required")
	}
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return refs, fail(400, "invalid_content", "Native content objects required")
		}
		kind := stringValue(object["type"])
		if kind == "text" {
			if strings.TrimSpace(stringValue(object["text"])) == "" {
				return refs, fail(400, "invalid_content", "Empty native prompt")
			}
			continue
		}
		mediaType := strings.TrimSuffix(kind, "_url")
		if !contains([]string{"image", "video", "audio"}, mediaType) {
			return refs, fail(400, "invalid_content", "Unsupported content type")
		}
		role := stringValue(object["role"])
		if role != "" && !contains([]string{"first_frame", "last_frame", "reference_image", "reference_video", "reference_audio"}, role) {
			return refs, fail(400, "invalid_content", "Unknown reference role")
		}
		for key := range object {
			if key != "type" && key != "role" && key != kind {
				return refs, fail(400, "invalid_content", "Unexpected native content field")
			}
		}
		refs[mediaType] = append(refs[mediaType], object[kind])
	}
	return refs, nil
}
func musicChat(p map[string]any) error {
	messages, e := orderedValues(p["messages"])
	if e != nil || len(messages) == 0 {
		return fail(400, "invalid_messages", "Music chat needs ordered messages")
	}
	prompt := []string{}
	for _, m := range messages {
		v, ok := m.(map[string]any)
		if !ok {
			return fail(400, "invalid_messages", "Message objects required")
		}
		if !contains([]string{"user", "system"}, stringValue(v["role"])) {
			return fail(400, "invalid_messages", "Music supports user and system messages")
		}
		s, ok := v["content"].(string)
		if !ok {
			return fail(400, "invalid_messages", "Music content must be text")
		}
		prompt = append(prompt, s)
	}
	p["prompt"] = strings.Join(prompt, "\n")
	return nil
}
func geminiReferences(p map[string]any, cap Capability, conditions map[string]any) ([]any, error) {
	refs := []any{}
	contents, e := orderedValues(p["contents"])
	if e != nil || len(contents) == 0 {
		return refs, fail(400, "invalid_contents", "Ordered Gemini contents required")
	}
	for _, content := range contents {
		obj, ok := content.(map[string]any)
		if !ok {
			return refs, fail(400, "invalid_contents", "Content objects required")
		}
		parts, e := orderedValues(obj["parts"])
		if e != nil {
			return refs, e
		}
		for _, part := range parts {
			v, ok := part.(map[string]any)
			if !ok || len(v) != 1 {
				return refs, fail(400, "invalid_part", "Part objects required")
			}
			if v["text"] != nil {
				if len(v) != 1 {
					return refs, fail(400, "invalid_part", "Ambiguous part")
				}
				continue
			}
			inline := v["inline_data"]
			if inline == nil {
				inline = v["inlineData"]
			}
			if data, ok := inline.(map[string]any); ok {
				if e := alias(data, "mime_type", "mimeType"); e != nil {
					return refs, e
				}
				for k := range data {
					if k != "mime_type" && k != "data" {
						return refs, fail(400, "invalid_part", "Unknown inline image field")
					}
				}
				mime := stringValue(data["mime_type"])
				if mime == "" {
					mime = stringValue(data["mimeType"])
				}
				if !contains([]string{"image/png", "image/jpeg", "image/webp"}, mime) {
					return refs, fail(400, "invalid_mime", "Unsupported Gemini image MIME")
				}
				refs = append(refs, "data:"+mime+";base64,"+stringValue(data["data"]))
				continue
			}
			file := v["file_data"]
			if file == nil {
				file = v["fileData"]
			}
			if data, ok := file.(map[string]any); ok {
				if e := alias(data, "file_uri", "fileUri"); e != nil {
					return refs, e
				}
				if e := alias(data, "mime_type", "mimeType"); e != nil {
					return refs, e
				}
				for k := range data {
					if k != "file_uri" && k != "mime_type" {
						return refs, fail(400, "invalid_part", "Unknown file image field")
					}
				}
				uri := data["file_uri"]
				if uri == nil {
					uri = data["fileUri"]
				}
				refs = append(refs, uri)
				continue
			}
			return refs, fail(400, "invalid_part", "Supported text or image part required")
		}
	}
	if gc, ok := p["generationConfig"].(map[string]any); ok {
		if image, ok := gc["imageConfig"].(map[string]any); ok {
			if size := stringValue(image["imageSize"]); size != "" {
				if !contains(cap.Resolutions, size) {
					return refs, fail(400, "unsupported_resolution", "Unsupported Gemini image size")
				}
				conditions["resolution"] = size
				conditions["size_tier"] = size
			}
			if ratio := stringValue(image["aspectRatio"]); ratio != "" && !contains(cap.AspectRatios, ratio) {
				return refs, fail(400, "unsupported_ratio", "Unsupported Gemini aspect ratio")
			}
		}
	}
	return refs, nil
}
