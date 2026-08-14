package notifications

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
)

type Config struct {
	// User is the configuration for the user to create. Ignored in reuse mode
	// (when SessionToken is set).
	User createusers.Config `json:"user"`

	// Roles are the roles to assign to the user. Ignored in reuse mode, where the
	// caller assigns roles before the runner starts.
	Roles []string `json:"roles"`

	// PreCreatedUser is an existing user to connect as instead of creating one.
	// It is set together with SessionToken to run in reuse mode.
	PreCreatedUser *codersdk.User `json:"-"`

	// SessionToken authenticates PreCreatedUser's websocket connection. When set,
	// the runner skips user creation and role assignment (reuse mode).
	SessionToken string `json:"-"`

	// NotificationTimeout is how long to wait for notifications after triggering.
	NotificationTimeout time.Duration `json:"notification_timeout"`

	// DialTimeout is how long to wait for websocket connection.
	DialTimeout time.Duration `json:"dial_timeout"`

	// ExpectedNotificationsIDs is the list of notification template IDs to expect.
	ExpectedNotificationsIDs map[uuid.UUID]struct{} `json:"-"`

	Metrics *Metrics `json:"-"`

	// DialBarrier ensures all runners are connected before notifications are triggered.
	DialBarrier *sync.WaitGroup `json:"-"`

	// ReceivingWatchBarrier is the barrier for receiving users. Regular users wait on this to disconnect after receiving users complete.
	ReceivingWatchBarrier *sync.WaitGroup `json:"-"`

	// SMTPApiUrl is the URL of the SMTP mock HTTP API
	SMTPApiURL string `json:"smtp_api_url"`

	// SMTPRequestTimeout is the timeout for SMTP requests.
	SMTPRequestTimeout time.Duration `json:"smtp_request_timeout"`

	// SMTPHttpClient is the HTTP client for SMTP requests.
	SMTPHttpClient *http.Client `json:"-"`
}

func (c Config) Validate() error {
	if c.SessionToken != "" {
		// Reuse mode: the caller supplies an existing user and a token to connect
		// as, so the create-user config is not used.
		if c.PreCreatedUser == nil {
			return xerrors.New("pre_created_user must be set when session_token is set")
		}
	} else {
		// The runner always needs an org; ensure we propagate it into the user config.
		if c.User.OrganizationID == uuid.Nil {
			return xerrors.New("user organization_id must be set")
		}

		if err := c.User.Validate(); err != nil {
			return xerrors.Errorf("user config: %w", err)
		}
	}

	if c.DialBarrier == nil {
		return xerrors.New("dial barrier must be set")
	}

	if c.ReceivingWatchBarrier == nil {
		return xerrors.New("receiving_watch_barrier must be set")
	}

	if c.NotificationTimeout <= 0 {
		return xerrors.New("notification_timeout must be greater than 0")
	}

	if c.SMTPApiURL != "" && c.SMTPRequestTimeout <= 0 {
		return xerrors.New("smtp_request_timeout must be set if smtp_api_url is set")
	}

	if c.SMTPApiURL != "" && c.SMTPHttpClient == nil {
		return xerrors.New("smtp_http_client must be set if smtp_api_url is set")
	}

	if c.DialTimeout <= 0 {
		return xerrors.New("dial_timeout must be greater than 0")
	}

	if c.Metrics == nil {
		return xerrors.New("metrics must be set")
	}

	return nil
}
