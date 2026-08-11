package notifications

import (
	"net/http"
	"net/url"
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

	// URL is the deployment address the runner dials.
	URL *url.URL `json:"-"`

	// DialHTTPClient performs the websocket handshake request. It carries the
	// caller's TLS and proxy configuration, without which the websocket library
	// falls back to http.DefaultClient and ignores both.
	//
	// One client is shared by every runner. A websocket handshake returns 101 and
	// hands the TCP connection to the caller, so it never re-enters the idle pool
	// and every dial gets its own connection regardless of how many clients exist.
	// It must not be a client whose transport caps MaxConnsPerHost, which would
	// throttle the dials this test exists to make.
	DialHTTPClient *http.Client `json:"-"`

	// DialTimeout is how long to wait for websocket connection.
	DialTimeout time.Duration `json:"dial_timeout"`

	// ExpectedNotificationIDs is the set of notification template IDs to expect.
	ExpectedNotificationIDs map[uuid.UUID]struct{} `json:"-"`

	Metrics *Metrics `json:"-"`

	// DialBarrier ensures all runners are connected before notifications are triggered.
	DialBarrier *sync.WaitGroup `json:"-"`

	// ReceivingWatchBarrier is the barrier for receiving users. Regular users wait on this to disconnect after receiving users complete.
	ReceivingWatchBarrier *sync.WaitGroup `json:"-"`

	// SMTPApiURL is the URL of the SMTP mock HTTP API.
	SMTPApiURL string `json:"smtp_api_url"`

	// SMTPRequestTimeout is the timeout for SMTP requests.
	SMTPRequestTimeout time.Duration `json:"smtp_request_timeout"`

	// SMTPHttpClient is the HTTP client for SMTP requests.
	SMTPHttpClient *http.Client `json:"-"`
}

func (c Config) Validate() error {
	if c.PreCreatedUser.ID == uuid.Nil {
		return xerrors.New("pre_created_user must be set")
	}

	if c.SessionToken == "" {
		return xerrors.New("session_token must be set")
	}

	if c.URL == nil {
		return xerrors.New("url must be set")
	}

	if c.DialHTTPClient == nil {
		return xerrors.New("dial_http_client must be set")
	}

	if c.DialBarrier == nil {
		return xerrors.New("dial_barrier must be set")
	}

	if c.ReceivingWatchBarrier == nil {
		return xerrors.New("receiving_watch_barrier must be set")
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
