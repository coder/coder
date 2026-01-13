package bridge

import (
	"golang.org/x/xerrors"

	"github.com/google/uuid"

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

	// DirectToken is the Bearer token for direct mode.
	// If not set in direct mode, uses the client's token.
	DirectToken string `json:"direct_token"`

	// Provider is the API provider to use: "openai" or "anthropic".
	Provider string `json:"provider"`

	// RequestCount is the number of requests to make per runner.
	RequestCount int `json:"request_count"`

	// Stream indicates whether to use streaming requests.
	Stream bool `json:"stream"`

	// RequestPayloadSize is the size in bytes of the request payload (user message content).
	// If 0, uses default message content.
	RequestPayloadSize int `json:"request_payload_size"`

	Metrics *Metrics `json:"-"`
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

	return nil
}

func (c Config) NewStrategy(client *codersdk.Client) requestModeStrategy {
	if c.Mode == RequestModeDirect {
		return newDirectStrategy(directStrategyConfig{
			UpstreamURL: c.UpstreamURL,
			Token:       c.DirectToken,
			ClientToken: client.SessionToken(),
		})
	}

	return newBridgeStrategy(bridgeStrategyConfig{
		Client:   client,
		Provider: c.Provider,
		Metrics:  c.Metrics,
		User:     c.User,
	})
}
