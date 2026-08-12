package notifications_test

import (
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/coder/v2/testutil"
)

// validConfig returns a Config that passes validation, so each case below can
// invalidate exactly one field and prove that field is checked.
func validConfig(t *testing.T) notifications.Config {
	t.Helper()

	serverURL, err := url.Parse("http://coder.test")
	require.NoError(t, err)

	return notifications.Config{
		PreCreatedUser: codersdk.User{
			ReducedUser: codersdk.ReducedUser{
				MinimalUser: codersdk.MinimalUser{ID: uuid.New()},
			},
		},
		SessionToken:          "test-session-token",
		URL:                   serverURL,
		DialHTTPClient:        &http.Client{},
		DialTimeout:           testutil.WaitShort,
		DialBarrier:           new(sync.WaitGroup),
		ReceivingWatchBarrier: new(sync.WaitGroup),
		Metrics:               notifications.NewMetrics(prometheus.NewRegistry()),
	}
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, validConfig(t).Validate())
	})

	t.Run("ValidWithSMTP", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig(t)
		cfg.SMTPApiURL = "http://smtp.test"
		cfg.SMTPRequestTimeout = testutil.WaitShort
		cfg.SMTPHttpClient = &http.Client{}
		require.NoError(t, cfg.Validate())
	})

	for _, tc := range []struct {
		name          string
		invalidate    func(*notifications.Config)
		errorContains string
	}{
		{
			name:          "NoPreCreatedUser",
			invalidate:    func(c *notifications.Config) { c.PreCreatedUser = codersdk.User{} },
			errorContains: "pre_created_user must be set",
		},
		{
			name:          "NoSessionToken",
			invalidate:    func(c *notifications.Config) { c.SessionToken = "" },
			errorContains: "session_token must be set",
		},
		{
			name:          "NoURL",
			invalidate:    func(c *notifications.Config) { c.URL = nil },
			errorContains: "url must be set",
		},
		{
			name:          "NoDialHTTPClient",
			invalidate:    func(c *notifications.Config) { c.DialHTTPClient = nil },
			errorContains: "dial_http_client must be set",
		},
		{
			name:          "NoDialBarrier",
			invalidate:    func(c *notifications.Config) { c.DialBarrier = nil },
			errorContains: "dial_barrier must be set",
		},
		{
			name:          "NoReceivingWatchBarrier",
			invalidate:    func(c *notifications.Config) { c.ReceivingWatchBarrier = nil },
			errorContains: "receiving_watch_barrier must be set",
		},
		{
			name:          "NoDialTimeout",
			invalidate:    func(c *notifications.Config) { c.DialTimeout = 0 },
			errorContains: "dial_timeout must be greater than 0",
		},
		{
			name:          "NegativeDialTimeout",
			invalidate:    func(c *notifications.Config) { c.DialTimeout = -time.Second },
			errorContains: "dial_timeout must be greater than 0",
		},
		{
			name:          "NoMetrics",
			invalidate:    func(c *notifications.Config) { c.Metrics = nil },
			errorContains: "metrics must be set",
		},
		{
			// SMTP fields are only required once an SMTP URL is given.
			name: "SMTPURLWithoutRequestTimeout",
			invalidate: func(c *notifications.Config) {
				c.SMTPApiURL = "http://smtp.test"
				c.SMTPHttpClient = &http.Client{}
			},
			errorContains: "smtp_request_timeout must be set if smtp_api_url is set",
		},
		{
			name: "SMTPURLWithoutHTTPClient",
			invalidate: func(c *notifications.Config) {
				c.SMTPApiURL = "http://smtp.test"
				c.SMTPRequestTimeout = testutil.WaitShort
			},
			errorContains: "smtp_http_client must be set if smtp_api_url is set",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validConfig(t)
			tc.invalidate(&cfg)
			require.ErrorContains(t, cfg.Validate(), tc.errorContains)
		})
	}
}
