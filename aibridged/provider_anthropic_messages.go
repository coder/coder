package aibridged

import (
	"encoding/json"

	"golang.org/x/xerrors"
)

var _ Provider[BetaMessageNewParamsWrapper] = &AnthropicMessagesProvider{}

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

func (*AnthropicMessagesProvider) ParseRequest(payload []byte) (*BetaMessageNewParamsWrapper, error) {
	var in BetaMessageNewParamsWrapper
	if err := json.Unmarshal(payload, &in); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal request: %w", err)
	}

	return &in, nil
}

// NewStreamingSession creates a new session which handles streaming message completions.
func (*AnthropicMessagesProvider) NewStreamingSession(req *BetaMessageNewParamsWrapper) Session {
	return NewAnthropicMessagesStreamingSession(req)
}

// NewBlockingSession creates a new session which handles non-streaming message completions.
func (*AnthropicMessagesProvider) NewBlockingSession(req *BetaMessageNewParamsWrapper) Session {
	return NewAnthropicMessagesBlockingSession(req)
}
