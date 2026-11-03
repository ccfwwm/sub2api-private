package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"regexp"
	"strings"
)

const openAIImagesAccountParamRulesKey = "openai_images_request_params"

var openAIImageParamNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\[\]-]{0,127}$`)

type openAIImagesAccountParamRules struct {
	Drop   []string          `json:"drop"`
	Rename map[string]string `json:"rename"`
}

func applyOpenAIImagesAccountParamRules(account *Account, body []byte, contentType string) ([]byte, string, error) {
	rules, err := readOpenAIImagesAccountParamRules(account)
	if err != nil || rules == nil || len(body) == 0 {
		return body, contentType, err
	}
	mediaType, params, parseErr := mime.ParseMediaType(contentType)
	if parseErr == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		return applyOpenAIImageMultipartRules(body, params["boundary"], contentType, rules)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", err
	}
	if payload == nil {
		return nil, "", fmt.Errorf("image request must be a JSON object")
	}
	for _, key := range rules.Drop {
		delete(payload, key)
	}
	// Apply each rename to the original fields exactly once, just as multipart
	// does. Chains and swaps must not depend on map iteration or field order.
	adapted := make(map[string]json.RawMessage, len(payload))
	for source, value := range payload {
		target := source
		if mapped, ok := rules.Rename[source]; ok {
			target = mapped
		}
		if _, collision := adapted[target]; collision {
			return nil, "", fmt.Errorf("image parameter rename collision: %s -> %s", source, target)
		}
		adapted[target] = value
	}
	encoded, err := json.Marshal(adapted)
	return encoded, contentType, err
}

func readOpenAIImagesAccountParamRules(account *Account) (*openAIImagesAccountParamRules, error) {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey || len(account.Extra) == 0 {
		return nil, nil
	}
	raw, ok := account.Extra[openAIImagesAccountParamRulesKey]
	if !ok || raw == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid OpenAI image parameter rules: %w", err)
	}
	var rules openAIImagesAccountParamRules
	if err := json.Unmarshal(encoded, &rules); err != nil {
		return nil, fmt.Errorf("invalid OpenAI image parameter rules: %w", err)
	}
	if len(rules.Drop) == 0 && len(rules.Rename) == 0 {
		return nil, nil
	}
	for _, key := range rules.Drop {
		if !openAIImageParamNamePattern.MatchString(key) {
			return nil, fmt.Errorf("invalid image parameter name %q", key)
		}
	}
	targets := make(map[string]string, len(rules.Rename))
	for source, target := range rules.Rename {
		if !openAIImageParamNamePattern.MatchString(source) || !openAIImageParamNamePattern.MatchString(target) {
			return nil, fmt.Errorf("invalid image parameter mapping %q -> %q", source, target)
		}
		if other, exists := targets[target]; exists && other != source {
			return nil, fmt.Errorf("image parameter rename collision: multiple fields map to %q", target)
		}
		targets[target] = source
	}
	return &rules, nil
}

func applyOpenAIImageMultipartRules(body []byte, boundary, contentType string, rules *openAIImagesAccountParamRules) ([]byte, string, error) {
	if strings.TrimSpace(boundary) == "" {
		return nil, "", fmt.Errorf("multipart boundary is required")
	}
	dropped := make(map[string]struct{}, len(rules.Drop))
	for _, name := range rules.Drop {
		dropped[name] = struct{}{}
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if err := writer.SetBoundary(boundary); err != nil {
		return nil, "", fmt.Errorf("preserve multipart boundary: %w", err)
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	seen := make(map[string]string)
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}
		name := part.FormName()
		if name == "" {
			return nil, "", fmt.Errorf("multipart image parameter is missing a field name")
		}
		data, readErr := io.ReadAll(part)
		_ = part.Close()
		if readErr != nil {
			return nil, "", readErr
		}
		if _, remove := dropped[name]; remove {
			continue
		}
		targetName := name
		if target, ok := rules.Rename[name]; ok {
			targetName = target
		}
		if source, exists := seen[targetName]; exists && source != name {
			return nil, "", fmt.Errorf("image parameter rename collision at %q", targetName)
		}
		seen[targetName] = name
		header := cloneMultipartHeader(part.Header)
		if targetName != name {
			kind, params, err := mime.ParseMediaType(header.Get("Content-Disposition"))
			if err != nil {
				return nil, "", fmt.Errorf("invalid multipart content disposition: %w", err)
			}
			params["name"] = targetName
			header.Set("Content-Disposition", mime.FormatMediaType(kind, params))
		}
		out, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := out.Write(data); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), contentType, nil
}
