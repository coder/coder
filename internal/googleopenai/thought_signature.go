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
	if IsDirectGeminiOpenAIEndpoint(baseURL) {
		return true
	}
	return IsCoderAIBridgeEndpoint(baseURL) && IsGeminiModelID(modelID)
}

// ShouldPatchGoogleUpstreamRequest reports whether an AI Bridge upstream
// OpenAI-compatible request should carry Gemini thought signatures.
func ShouldPatchGoogleUpstreamRequest(baseURL string, modelID string) bool {
	return IsDirectGeminiOpenAIEndpoint(baseURL) && IsGeminiModelID(modelID)
}

// IsDirectGeminiOpenAIEndpoint matches Gemini API's OpenAI-compatible endpoint.
// Vertex AI has different hosts and paths. Add it here only with a fixture that
// confirms it accepts the same thought-signature fallback shape.
func IsDirectGeminiOpenAIEndpoint(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.EscapedPath())
	return host == "generativelanguage.googleapis.com" && strings.Contains(path, "/openai")
}

// IsCoderAIBridgeEndpoint matches chatd's local AI Bridge base URL.
func IsCoderAIBridgeEndpoint(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return strings.ToLower(parsed.Hostname()) == "coder-aibridge"
}

// IsGeminiModelID reports whether modelID names a Gemini model.
func IsGeminiModelID(modelID string) bool {
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

// AddThoughtSignaturesToLatestTurn adds fallback thought signatures to every
// assistant tool call after the latest user message. Only the current turn needs
// patching because completed tool-call/result pairs from earlier turns are not
// validated by Google as active function calls.
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
		if !ok || !IsAssistantRole(message["role"]) {
			continue
		}
		toolCalls, ok := message["tool_calls"].([]any)
		if !ok || len(toolCalls) == 0 {
			continue
		}
		for _, rawToolCall := range toolCalls {
			toolCall, ok := rawToolCall.(map[string]any)
			if !ok {
				continue
			}
			if EnsureThoughtSignature(toolCall) {
				changed = true
			}
		}
	}
	return changed
}

// IsAssistantRole accepts both OpenAI's "assistant" and Gemini's internal
// "model" role name for assistant messages.
func IsAssistantRole(role any) bool {
	roleValue, _ := role.(string)
	return roleValue == "assistant" || roleValue == "model"
}

// EnsureThoughtSignature adds a fallback thought signature to toolCall when it
// does not already carry a Google thought signature.
func EnsureThoughtSignature(toolCall map[string]any) bool {
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
