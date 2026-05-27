package chatprovider

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const googleOpenAICompatDummyThoughtSignature = "skip_thought_signature_validator"

func withOpenAICompatRequestPatches(
	client *http.Client,
	baseURL string,
	modelID string,
) *http.Client {
	if client == nil {
		client = &http.Client{}
	} else {
		clone := *client
		client = &clone
	}
	client.Transport = &openAICompatRequestPatchTransport{
		Base:    client.Transport,
		BaseURL: baseURL,
		ModelID: modelID,
	}
	return client
}

type openAICompatRequestPatchTransport struct {
	Base    http.RoundTripper
	BaseURL string
	ModelID string
}

func (t *openAICompatRequestPatchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base()
	if !shouldPatchOpenAICompatRequest(req) {
		return base.RoundTrip(req)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()

	patched := patchOpenAICompatChatCompletionsBody(body, t.BaseURL, t.ModelID)
	req.Body = io.NopCloser(bytes.NewReader(patched))
	req.ContentLength = int64(len(patched))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(patched)), nil
	}

	return base.RoundTrip(req)
}

func (t *openAICompatRequestPatchTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

func shouldPatchOpenAICompatRequest(req *http.Request) bool {
	return req != nil &&
		req.Method == http.MethodPost &&
		req.Body != nil &&
		strings.HasSuffix(req.URL.Path, "/chat/completions")
}

func patchOpenAICompatChatCompletionsBody(body []byte, baseURL string, modelID string) []byte {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	changed := rewriteOpenAICompatSingleToolChoice(payload)
	if shouldAddGoogleOpenAICompatThoughtSignatures(baseURL, modelID) {
		changed = addGoogleOpenAICompatThoughtSignatures(payload) || changed
	}
	if !changed {
		return body
	}

	patched, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return patched
}

func rewriteOpenAICompatSingleToolChoice(payload map[string]any) bool {
	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 1 {
		return false
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		return false
	}
	function, ok := tool["function"].(map[string]any)
	if !ok {
		return false
	}
	toolName, _ := function["name"].(string)
	if toolName == "" {
		return false
	}

	toolChoice, ok := payload["tool_choice"].(map[string]any)
	if !ok {
		return false
	}
	if toolType, _ := toolChoice["type"].(string); toolType != "function" {
		return false
	}
	choiceFunction, ok := toolChoice["function"].(map[string]any)
	if !ok {
		return false
	}
	choiceName, _ := choiceFunction["name"].(string)
	if choiceName != toolName {
		return false
	}

	payload["tool_choice"] = "required"
	return true
}

func shouldAddGoogleOpenAICompatThoughtSignatures(baseURL string, modelID string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.EscapedPath())
	if host == "generativelanguage.googleapis.com" && strings.Contains(path, "/openai") {
		return true
	}
	return host == "coder-aibridge" && isGeminiModelID(modelID)
}

func isGeminiModelID(modelID string) bool {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	return strings.HasPrefix(modelID, "gemini-") || strings.Contains(modelID, "/gemini-")
}

func addGoogleOpenAICompatThoughtSignatures(payload map[string]any) bool {
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

	changed := false
	for _, raw := range messages[currentTurnStart+1:] {
		message, ok := raw.(map[string]any)
		if !ok || !isOpenAICompatAssistantRole(message["role"]) {
			continue
		}
		toolCalls, ok := message["tool_calls"].([]any)
		if !ok || len(toolCalls) == 0 {
			continue
		}
		firstToolCall, ok := toolCalls[0].(map[string]any)
		if !ok {
			continue
		}
		if ensureGoogleOpenAICompatThoughtSignature(firstToolCall) {
			changed = true
		}
	}
	return changed
}

func isOpenAICompatAssistantRole(value any) bool {
	role, _ := value.(string)
	return role == "assistant" || role == "model"
}

func ensureGoogleOpenAICompatThoughtSignature(toolCall map[string]any) bool {
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
	google["thought_signature"] = googleOpenAICompatDummyThoughtSignature
	return true
}
