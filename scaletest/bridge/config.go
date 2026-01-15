package bridge

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
)

type RequestMode string

const (
	RequestModeBridge RequestMode = "bridge"
	RequestModeDirect RequestMode = "direct"
)

type Config struct {
	// Mode determines how requests are made.
	// "bridge": Create users in Coder and use their session tokens to make requests through AI Bridge.
	// "direct": Make requests directly to UpstreamURL without user creation.
	Mode RequestMode `json:"mode"`

	// User is the configuration for the user to create.
	// Required in bridge mode.
	User createusers.Config `json:"user"`

	// UpstreamURL is the URL to make requests to directly.
	// Only used in direct mode.
	UpstreamURL string `json:"upstream_url"`

	// Provider is the API provider to use: "openai" or "anthropic".
	Provider string `json:"provider"`

	// RequestCount is the number of requests to make per runner.
	RequestCount int `json:"request_count"`

	// Stream indicates whether to use streaming requests.
	Stream bool `json:"stream"`

	// RequestPayloadSize is the size in bytes of the request payload (user message content).
	// If 0, uses default message content.
	RequestPayloadSize int `json:"request_payload_size"`

	// NumMessages is the number of messages to include in the conversation.
	// Messages alternate between user and assistant roles, always ending with user.
	// Must be greater than 0.
	NumMessages int `json:"num_messages"`

	// HTTPTimeout is the timeout for individual HTTP requests to the upstream
	// provider. This is separate from the job timeout which controls the overall
	// test execution.
	HTTPTimeout time.Duration `json:"http_timeout"`

	Metrics *Metrics `json:"-"`

	// RequestBody is the pre-serialized JSON request body. This is generated
	// once by PrepareRequestBody and shared across all runners and requests.
	RequestBody []byte `json:"-"`
}

func (c Config) Validate() error {
	if c.Metrics == nil {
		return xerrors.New("metrics must be set")
	}

	// Validate mode
	if c.Mode != RequestModeBridge && c.Mode != RequestModeDirect {
		return xerrors.New("mode must be either 'bridge' or 'direct'")
	}

	if c.RequestCount <= 0 {
		return xerrors.New("request_count must be greater than 0")
	}

	// Validate provider
	if c.Provider != "openai" && c.Provider != "anthropic" {
		return xerrors.New("provider must be either 'openai' or 'anthropic'")
	}

	if c.Mode == RequestModeDirect {
		// In direct mode, UpstreamURL must be set.
		if c.UpstreamURL == "" {
			return xerrors.New("upstream_url must be set in direct mode")
		}
		return nil
	}

	// In bridge mode, User config is required.
	if c.User.OrganizationID == uuid.Nil {
		return xerrors.New("user organization_id must be set in bridge mode")
	}

	if err := c.User.Validate(); err != nil {
		return xerrors.Errorf("user config: %w", err)
	}

	if c.NumMessages <= 0 {
		return xerrors.New("num_messages must be greater than 0")
	}

	return nil
}

func (c Config) NewStrategy(client *codersdk.Client) requestModeStrategy {
	if c.Mode == RequestModeDirect {
		return newDirectStrategy(directStrategyConfig{
			UpstreamURL: c.UpstreamURL,
		})
	}

	return newBridgeStrategy(bridgeStrategyConfig{
		Client:   client,
		Provider: c.Provider,
		Metrics:  c.Metrics,
		User:     c.User,
	})
}

// PrepareRequestBody generates the conversation and serializes the full request
// body once. This should be called before creating Runners so that all runners
// share the same pre-generated payload.
func (c *Config) PrepareRequestBody() error {
	provider := NewProviderStrategy(c.Provider)
	model := provider.DefaultModel()

	var formattedMessages []any
	if c.RequestPayloadSize > 0 {
		var err error
		formattedMessages, err = generateConversation(provider, c.RequestPayloadSize, c.NumMessages)
		if err != nil {
			return xerrors.Errorf("generate conversation: %w", err)
		}
	} else {
		messages := []message{{
			Role:    "user",
			Content: "Hello from the bridge load generator.",
		}}
		formattedMessages = provider.formatMessages(messages)
	}

	reqBody := provider.buildRequestBody(model, formattedMessages, c.Stream)

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return xerrors.Errorf("marshal request body: %w", err)
	}

	c.RequestBody = bodyBytes
	return nil
}
