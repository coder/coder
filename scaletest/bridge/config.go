package bridge

import (
	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/scaletest/createusers"
)

type Config struct {
	// User is the configuration for the user to create.
	User createusers.Config `json:"user"`

	Metrics *Metrics `json:"-"`
}

func (c Config) Validate() error {
	// The runner always needs an org; ensure we propagate it into the user config.
	if c.User.OrganizationID == uuid.Nil {
		return xerrors.New("user organization_id must be set")
	}

	if err := c.User.Validate(); err != nil {
		return xerrors.Errorf("user config: %w", err)
	}

	if c.Metrics == nil {
		return xerrors.New("metrics must be set")
	}

	return nil
}
