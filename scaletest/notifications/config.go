package notifications

import (
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/scaletest/createusers"
)

type Config struct {
	// User is the configuration for the user to create.
	User createusers.Config `json:"user"`

	// IsOwner indicates if this user should be assigned Owner role.
	// If true, the user will receive notifications.
	IsOwner bool `json:"is_owner"`

	// NotificationTimeout is how long to wait for notifications after triggering.
	NotificationTimeout time.Duration `json:"notification_timeout"`

	// DialTimeout is how long to wait for websocket connection.
	DialTimeout time.Duration `json:"dial_timeout"`

	Metrics *Metrics `json:"-"`

	// DialBarrier ensures all runners are connected before notifications are triggered.
	DialBarrier *sync.WaitGroup `json:"-"`

	// OwnerDialBarrier is the barrier for owner users. Regular users wait on this to disconnect after owner users complete.
	OwnerDialBarrier *sync.WaitGroup `json:"-"`
}

func (c Config) Validate() error {
	// The runner always needs an org; ensure we propagate it into the user config.
	if c.User.OrganizationID == uuid.Nil {
		return xerrors.New("user organization_id must be set")
	}

	if err := c.User.Validate(); err != nil {
		return xerrors.Errorf("user config: %w", err)
	}

	if c.DialBarrier == nil {
		return xerrors.New("dial barrier must be set")
	}

	if !c.IsOwner && c.OwnerDialBarrier == nil {
		return xerrors.New("owner_dial_barrier must be set for regular users")
	}

	if c.NotificationTimeout <= 0 {
		return xerrors.New("notification_timeout must be greater than 0")
	}

	if c.DialTimeout <= 0 {
		return xerrors.New("dial_timeout must be greater than 0")
	}

	if c.Metrics == nil {
		return xerrors.New("metrics must be set")
	}

	return nil
}
