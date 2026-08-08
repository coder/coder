package notifications

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

type Config struct {
	// PreCreatedUser is the user the runner connects as. The caller must
	// provide an already-authenticated user before the runner starts.
	PreCreatedUser codersdk.User `json:"-"`

	// SessionToken authenticates PreCreatedUser for the websocket connection.
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
	// The runner connects as an already-created, authenticated user.
	if c.PreCreatedUser.ID == uuid.Nil {
		return xerrors.New("pre_created_user must be set")
	}

	if c.SessionToken == "" {
		return xerrors.New("session_token must be set")
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
