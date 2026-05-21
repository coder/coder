package notifications

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/scaletest/createusers"
)

type Config struct {
	// User is the configuration for the user to create.
	User createusers.Config `json:"user"`

	// Roles are the roles to assign to the user.
	Roles []string `json:"roles"`

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
