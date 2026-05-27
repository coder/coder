package chatprovider

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const geminiOpenAICompatDummyThoughtSignature = "skip_thought_signature_validator"

func withGeminiOpenAICompatThoughtSignatures(
	client *http.Client,
	baseURL string,
	modelID string,
) *http.Client {
	if !shouldPatchGeminiOpenAICompatBaseURL(baseURL, modelID) {
		return client
	}
	if client == nil {
		client = &http.Client{}
	} else {
		clone := *client
		client = &clone
	}
	client.Transport = &geminiOpenAICompatThoughtSignatureTransport{Base: client.Transport}
	return client
}

func shouldPatchGeminiOpenAICompatBaseURL(baseURL string, modelID string) bool {
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

type geminiOpenAICompatThoughtSignatureTransport struct {
	Base http.RoundTripper
}

func (t *geminiOpenAICompatThoughtSignatureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !shouldPatchGeminiOpenAICompatThoughtSignatureRequest(req) {
		return t.base().RoundTrip(req)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()

	patched, changed := addGeminiOpenAICompatThoughtSignatures(body)
	if !changed {
		patched = body
	}
	req.Body = io.NopCloser(bytes.NewReader(patched))
	req.ContentLength = int64(len(patched))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(patched)), nil
	}

	return t.base().RoundTrip(req)
}

func (t *geminiOpenAICompatThoughtSignatureTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

func shouldPatchGeminiOpenAICompatThoughtSignatureRequest(req *http.Request) bool {
	return req != nil &&
		req.Method == http.MethodPost &&
		req.Body != nil &&
		strings.HasSuffix(req.URL.Path, "/chat/completions")
}

func addGeminiOpenAICompatThoughtSignatures(body []byte) ([]byte, bool) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, false
	}

	messages, ok := payload["messages"].([]any)
	if !ok {
		return body, false
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
		changed = ensureGeminiOpenAICompatThoughtSignature(firstToolCall) || changed
	}
	if !changed {
		return body, false
	}

	patched, err := json.Marshal(payload)
	if err != nil {
		return body, false
	}
	return patched, true
}

func isOpenAICompatAssistantRole(value any) bool {
	role, _ := value.(string)
	return role == "assistant" || role == "model"
}

func ensureGeminiOpenAICompatThoughtSignature(toolCall map[string]any) bool {
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
	google["thought_signature"] = geminiOpenAICompatDummyThoughtSignature
	return true
}
