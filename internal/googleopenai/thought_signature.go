// Package googleopenai contains compatibility helpers for Google's
// OpenAI-compatible Gemini APIs.
package googleopenai

import (
	"encoding/json"
	"net/url"
	"strings"
)

// DummyThoughtSignature is Google's documented last-resort bypass for callers
// that cannot preserve a real Gemini thought signature through OpenAI-compatible
// serialization. See https://ai.google.dev/gemini-api/docs/thought-signatures.
const DummyThoughtSignature = "skip_thought_signature_validator"

// ShouldPatchOpenAICompatRequest reports whether a client-side
// OpenAI-compatible request should carry Gemini thought signatures.
func ShouldPatchOpenAICompatRequest(baseURL string, modelID string) bool {
	// Direct Google endpoints are already provider-scoped. Patch them even when
	// the configured model ID is an alias without a Gemini prefix.
	if isDirectGeminiOpenAIEndpoint(baseURL) {
		return true
	}
	return isCoderAIBridgeEndpoint(baseURL) && isGeminiModelID(modelID)
}

// ShouldPatchGoogleUpstreamRequest reports whether an AI Bridge upstream
// OpenAI-compatible request should carry Gemini thought signatures.
func ShouldPatchGoogleUpstreamRequest(baseURL string) bool {
	return isDirectGeminiOpenAIEndpoint(baseURL)
}

// Vertex AI has different hosts and paths. Add it here only with a fixture that
// confirms it accepts the same thought-signature fallback shape.
func isDirectGeminiOpenAIEndpoint(baseURL string) bool {
	parsed, ok := parseBaseURL(baseURL)
	if !ok {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.EscapedPath())
	return host == "generativelanguage.googleapis.com" && strings.Contains(path, "/openai")
}

func isCoderAIBridgeEndpoint(baseURL string) bool {
	parsed, ok := parseBaseURL(baseURL)
	if !ok {
		return false
	}
	return strings.ToLower(parsed.Hostname()) == "coder-aibridge"
}

// parseBaseURL parses a provider base URL, handling bare hostnames without
// a scheme by prepending "https://".
func parseBaseURL(baseURL string) (*url.URL, bool) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, false
	}
	parsed, err := url.Parse(baseURL)
	if err == nil && parsed.Hostname() == "" && !strings.Contains(baseURL, "://") {
		parsed, err = url.Parse("https://" + baseURL)
	}
	if err != nil {
		return nil, false
	}
	return parsed, true
}

func isGeminiModelID(modelID string) bool {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	return strings.HasPrefix(modelID, "gemini-") || strings.Contains(modelID, "/gemini-")
}

// PatchThoughtSignatures adds fallback thought signatures to Gemini tool-call
// history in body. It returns changed=false when no patch is needed.
func PatchThoughtSignatures(body []byte) ([]byte, bool, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false, err
	}
	if !AddThoughtSignaturesToLatestTurn(payload) {
		return body, false, nil
	}
	patched, err := json.Marshal(payload)
	if err != nil {
		return nil, false, err
	}
	return patched, true, nil
}

// AddThoughtSignaturesToLatestTurn patches only the current turn because
// completed tool-call/result pairs from earlier turns are not validated by
// Google as active function calls.
func AddThoughtSignaturesToLatestTurn(payload map[string]any) bool {
	messages, ok := payload["messages"].([]any)
	if !ok {
		return false
	}

	currentTurnStart := -1
	for i, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := message["role"].(string); role == "user" {
			currentTurnStart = i
		}
	}
	if currentTurnStart == -1 {
		return false
	}

	changed := false
	for _, raw := range messages[currentTurnStart+1:] {
		message, ok := raw.(map[string]any)
		if !ok || !isAssistantRole(message["role"]) {
			continue
		}
		toolCalls, ok := message["tool_calls"].([]any)
		if !ok || len(toolCalls) == 0 {
			continue
		}
		// Every tool call in parallel batches needs a signature,
		// not just the first one.
		for _, rawToolCall := range toolCalls {
			toolCall, ok := rawToolCall.(map[string]any)
			if !ok {
				continue
			}
			if ensureThoughtSignature(toolCall) {
				changed = true
			}
		}
	}
	return changed
}

// Gemini can serialize assistant messages with its native "model" role.
func isAssistantRole(role any) bool {
	roleValue, _ := role.(string)
	return roleValue == "assistant" || roleValue == "model"
}

// Real provider signatures are preserved when present.
func ensureThoughtSignature(toolCall map[string]any) bool {
	extraContent, _ := toolCall["extra_content"].(map[string]any)
	google, _ := extraContent["google"].(map[string]any)
	if signature, _ := google["thought_signature"].(string); signature != "" {
		return false
	}
	if extraContent == nil {
		extraContent = map[string]any{}
		toolCall["extra_content"] = extraContent
	}
	if google == nil {
		google = map[string]any{}
		extraContent["google"] = google
	}
	google["thought_signature"] = DummyThoughtSignature
	return true
}
