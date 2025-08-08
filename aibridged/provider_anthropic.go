package aibridged

import (
	"encoding/json"
	"io"
	"net/http"

	"golang.org/x/xerrors"
)

var _ Provider = &AnthropicMessagesProvider{}

// AnthropicMessagesProvider allows for interactions with the Anthropic Messages API.
// See https://docs.anthropic.com/en/api/messages
type AnthropicMessagesProvider struct {
	baseURL, key string
}

func NewAnthropicMessagesProvider(baseURL, key string) *AnthropicMessagesProvider {
	return &AnthropicMessagesProvider{
		baseURL: baseURL,
		key:     key,
	}
}

func (p *AnthropicMessagesProvider) CreateSession(w http.ResponseWriter, r *http.Request, tools ToolRegistry) (Session, error) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, xerrors.Errorf("read body: %w", err)
	}

	var req BetaMessageNewParamsWrapper
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal request: %w", err)
	}

	if req.Stream {
		return NewAnthropicMessagesStreamingSession(&req, p.baseURL, p.key), nil
	}

	return NewAnthropicMessagesBlockingSession(&req, p.baseURL, p.key), nil
}

func (p *AnthropicMessagesProvider) Identifier() string {
	return "anthropic"
}
