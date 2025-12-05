package bridge

import (
	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/scaletest/createusers"
)

type Config struct {
	// User is the configuration for the user to create.
	// Required in full mode (when DirectURL is not set).
	User createusers.Config `json:"user"`

	// DirectURL is the URL to make requests to directly.
	// If set, enables direct mode and skips user creation.
	DirectURL string `json:"direct_url"`

	// DirectToken is the Bearer token for direct mode.
	// If not set in direct mode, uses the client's token.
	DirectToken string `json:"direct_token"`

	// RequestCount is the number of requests to make per runner.
	RequestCount int `json:"request_count"`

	// Model is the model to use for requests.
	Model string `json:"model"`

	// Stream indicates whether to use streaming requests.
	Stream bool `json:"stream"`

	Metrics *Metrics `json:"-"`
}

func (c Config) Validate() error {
	if c.Metrics == nil {
		return xerrors.New("metrics must be set")
	}

	// In direct mode, DirectURL must be set.
	if c.DirectURL != "" {
		if c.RequestCount <= 0 {
			return xerrors.New("request_count must be greater than 0")
		}
		if c.Model == "" {
			return xerrors.New("model must be set")
		}
		return nil
	}

	// In full mode, User config is required.
	if c.User.OrganizationID == uuid.Nil {
		return xerrors.New("user organization_id must be set")
	}

	if err := c.User.Validate(); err != nil {
		return xerrors.Errorf("user config: %w", err)
	}

	// Validate full mode has reasonable values (defaults will be set in CLI if not provided).
	if c.RequestCount < 0 {
		return xerrors.New("request_count must be non-negative")
	}

	return nil
}
