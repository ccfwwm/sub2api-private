package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

const monitorStreamMaxBytes = 256 << 10

func doMonitorStreamRequest(ctx context.Context, fullURL string, payload []byte, headers map[string]string, provider, apiMode string) (string, string, int, error) {
	resp, err := openMonitorResponse(ctx, monitorHTTPClient, http.MethodPost, fullURL, payload, headers)
	if err != nil {
		return "", "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, err := readMonitorResponse(resp.Body, monitorResponseMaxBytes)
		return "", string(raw), resp.StatusCode, err
	}
	if contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); contentType != "text/event-stream" {
		return "", "", resp.StatusCode, errors.New("stream probe: expected an SSE response")
	}
	text, err := readMonitorStream(resp.Body, provider, apiMode)
	return text, "", resp.StatusCode, err
}

func readMonitorStream(body io.Reader, provider, apiMode string) (string, error) {
	limited := &io.LimitedReader{R: body, N: monitorStreamMaxBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), monitorStreamMaxBytes+1)
	var parser openAICompatSSEFrameParser
	var text, finalText strings.Builder
	for scanner.Scan() {
		if limited.N == 0 {
			return "", errors.New("stream probe: response exceeds size limit")
		}
		frame, ok := parser.AddLine(scanner.Text())
		if !ok {
			continue
		}
		done, err := collectMonitorStreamFrame(frame, provider, apiMode, &text, &finalText)
		if err != nil {
			return "", err
		}
		if done {
			if text.Len() == 0 {
				return finalText.String(), nil
			}
			return text.String(), nil
		}
	}
	if limited.N == 0 {
		return "", errors.New("stream probe: response exceeds size limit")
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("stream probe: read response: %w", err)
	}
	return "", errors.New("stream probe: response ended before completion")
}

func collectMonitorStreamFrame(frame openAICompatSSEFrame, provider, apiMode string, text, finalText *strings.Builder) (bool, error) {
	data := strings.TrimSpace(frame.Data)
	if data == "[DONE]" {
		// Chat Completions compatible providers may terminate using this sentinel.
		return isOpenAICompatibleChatProvider(provider) && apiMode == MonitorAPIModeChatCompletions, nil
	}
	if !gjson.Valid(data) || !strings.HasPrefix(data, "{") {
		return false, errors.New("stream probe: invalid SSE event data")
	}
	event := gjson.Parse(data)
	kind := event.Get("type").String()
	if kind == "" {
		kind = frame.EventType
	}
	switch kind {
	case "error", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return false, fmt.Errorf("stream probe: upstream reported %s", kind)
	}
	if frame.EventType == "error" || (event.Get("error").Exists() && event.Get("error").Type != gjson.Null) {
		return false, errors.New("stream probe: upstream reported an SSE error")
	}
	switch {
	case provider == MonitorProviderOpenAI && apiMode == MonitorAPIModeResponses:
		switch kind {
		case "response.output_text.delta":
			text.WriteString(event.Get("delta").String())
		case "response.output_text.done":
			finalText.WriteString(event.Get("text").String())
		case "response.completed", "response.done":
			if status := event.Get("response.status").String(); status != "" && status != "completed" {
				return false, errors.New("stream probe: upstream did not complete the response")
			}
			if final := extractOpenAIResponsesText([]byte(event.Get("response").Raw)); strings.TrimSpace(final) != "" {
				text.Reset()
				text.WriteString(final)
			}
			return true, nil
		}
	case provider == MonitorProviderAnthropic:
		switch kind {
		case "content_block_start":
			if event.Get("content_block.type").String() == "text" {
				text.WriteString(event.Get("content_block.text").String())
			}
		case "content_block_delta":
			if event.Get("delta.type").String() == "text_delta" {
				text.WriteString(event.Get("delta.text").String())
			}
		case "message_stop":
			return true, nil
		}
	case provider == MonitorProviderGemini:
		if event.Get("promptFeedback.blockReason").String() != "" {
			return false, errors.New("stream probe: upstream blocked the prompt")
		}
		candidate := event.Get("candidates.0")
		candidate.Get("content.parts").ForEach(func(_, part gjson.Result) bool {
			if !part.Get("thought").Bool() {
				text.WriteString(part.Get("text").String())
			}
			return true
		})
		switch candidate.Get("finishReason").String() {
		case "", "FINISH_REASON_UNSPECIFIED":
		case "STOP", "MAX_TOKENS":
			return true, nil
		default:
			return false, errors.New("stream probe: upstream terminated generation")
		}
	default:
		choice := event.Get("choices.0")
		text.WriteString(choice.Get("delta.content").String())
		switch choice.Get("finish_reason").String() {
		case "":
		case "content_filter":
			return false, errors.New("stream probe: upstream filtered the response")
		default:
			return true, nil
		}
	}
	return false, nil
}
