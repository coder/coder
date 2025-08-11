package aibridged

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"golang.org/x/xerrors"
)

var _ Provider = &AnthropicMessagesProvider{}

// AnthropicMessagesProvider allows for interactions with the Anthropic Messages API.
// See https://docs.anthropic.com/en/api/messages
type AnthropicMessagesProvider struct {
	baseURL, key string
}

func NewAnthropicMessagesProvider(baseURL, key string) *AnthropicMessagesProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/"
	}
	if key == "" {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}

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

	switch r.URL.Path {
	case "/anthropic/v1/messages":
		var req BetaMessageNewParamsWrapper
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, xerrors.Errorf("failed to unmarshal request: %w", err)
		}

		if req.Stream {
			return NewAnthropicMessagesStreamingSession(&req, p.baseURL, p.key), nil
		}

		return NewAnthropicMessagesBlockingSession(&req, p.baseURL, p.key), nil
	}

	return nil, UnknownRoute
}

func (p *AnthropicMessagesProvider) Identifier() string {
	return ProviderAnthropic
}

func (p *AnthropicMessagesProvider) BaseURL() string {
	return p.baseURL
}

func (p *AnthropicMessagesProvider) Key() string {
	return p.key
}

func newAnthropicClient(baseURL, key string, opts ...option.RequestOption) anthropic.Client {
	opts = append(opts, option.WithAPIKey(key))
	opts = append(opts, option.WithBaseURL(baseURL))

	return anthropic.NewClient(opts...)
}
