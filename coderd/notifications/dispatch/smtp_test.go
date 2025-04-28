package dispatch_test

import (
	"bytes"
	"fmt"
	"log"
	"sync"
	"testing"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/dispatch/smtptest"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestSMTP(t *testing.T) {
	t.Parallel()

	const (
		username = "bob"
		password = "🤫"

		hello = "localhost"

		identity = "robert"
		from     = "system@coder.com"
		to       = "bob@bob.com"

		subject = "This is the subject"
		body    = "This is the body"

		caFile   = "smtptest/fixtures/ca.crt"
		certFile = "smtptest/fixtures/server.crt"
		keyFile  = "smtptest/fixtures/server.key"
	)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true, IgnoredErrorIs: []error{}}).Leveled(slog.LevelDebug)
	tests := []struct {
		name             string
		cfg              codersdk.NotificationsEmailConfig
		toAddrs          []string
		authMechs        []string
		expectedAuthMeth string
		expectedErr      string
		retryable        bool
		useTLS           bool
		failOnDataFn     func() error
	}{
		/**
		 * LOGIN auth mechanism
		 */
		{
			name:      "LOGIN auth",
			authMechs: []string{sasl.Login},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,

				Auth: codersdk.NotificationsEmailAuthConfig{
					Username: username,
					Password: password,
				},
			},
			toAddrs:          []string{to},
			expectedAuthMeth: sasl.Login,
		},
		{
			name:      "invalid LOGIN auth user",
			authMechs: []string{sasl.Login},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,

				Auth: codersdk.NotificationsEmailAuthConfig{
					Username: username + "-wrong",
					Password: password,
				},
			},
			toAddrs:          []string{to},
			expectedAuthMeth: sasl.Login,
			expectedErr:      "unknown user",
			retryable:        true,
		},
		{
			name:      "invalid LOGIN auth credentials",
			authMechs: []string{sasl.Login},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,

				Auth: codersdk.NotificationsEmailAuthConfig{
					Username: username,
					Password: password + "-wrong",
				},
			},
			toAddrs:          []string{to},
			expectedAuthMeth: sasl.Login,
			expectedErr:      "incorrect password",
			retryable:        true,
		},
		{
			name:      "password from file",
			authMechs: []string{sasl.Login},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,

				Auth: codersdk.NotificationsEmailAuthConfig{
					Username:     username,
					PasswordFile: "smtptest/fixtures/password.txt",
				},
			},
			toAddrs:          []string{to},
			expectedAuthMeth: sasl.Login,
		},
		/**
		 * PLAIN auth mechanism
		 */
		{
			name:      "PLAIN auth",
			authMechs: []string{sasl.Plain},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,

				Auth: codersdk.NotificationsEmailAuthConfig{
					Identity: identity,
					Username: username,
					Password: password,
				},
			},
			toAddrs:          []string{to},
			expectedAuthMeth: sasl.Plain,
		},
		{
			name:      "PLAIN auth without identity",
			authMechs: []string{sasl.Plain},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,

				Auth: codersdk.NotificationsEmailAuthConfig{
					Identity: "",
					Username: username,
					Password: password,
				},
			},
			toAddrs:          []string{to},
			expectedAuthMeth: sasl.Plain,
		},
		{
			name:      "PLAIN+LOGIN, choose PLAIN",
			authMechs: []string{sasl.Login, sasl.Plain},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,

				Auth: codersdk.NotificationsEmailAuthConfig{
					Identity: identity,
					Username: username,
					Password: password,
				},
			},
			toAddrs:          []string{to},
			expectedAuthMeth: sasl.Plain,
		},
		/**
		 * No auth mechanism
		 */
		{
			name:      "No auth mechanisms supported",
			authMechs: []string{},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,

				Auth: codersdk.NotificationsEmailAuthConfig{
					Username: username,
					Password: password,
				},
			},
			toAddrs:          []string{to},
			expectedAuthMeth: "",
			expectedErr:      "no authentication mechanisms supported by server",
			retryable:        false,
		},
		{
			name:      "No auth mechanisms supported, none configured",
			authMechs: []string{},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,
			},
			toAddrs:          []string{to},
			expectedAuthMeth: "",
		},
		{
			name:      "Auth mechanisms supported optionally, none configured",
			authMechs: []string{sasl.Login, sasl.Plain},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,
			},
			toAddrs:          []string{to},
			expectedAuthMeth: "",
		},
		/**
		 * TLS connections
		 */
		{
			// TLS is forced but certificate used by mock server is untrusted.
			name:        "TLS: x509 untrusted",
			useTLS:      true,
			expectedErr: "tls: failed to verify certificate",
			retryable:   true,
		},
		{
			// TLS is forced and self-signed certificate used by mock server is not verified.
			name:   "TLS: x509 untrusted ignored",
			useTLS: true,
			cfg: codersdk.NotificationsEmailConfig{
				Hello:    hello,
				From:     from,
				ForceTLS: true,
				TLS: codersdk.NotificationsEmailTLSConfig{
					InsecureSkipVerify: true,
				},
			},
			toAddrs: []string{to},
		},
		{
			// TLS is forced and STARTTLS is configured, but STARTTLS cannot be used by TLS connections.
			// STARTTLS should be disabled and connection should succeed.
			name:   "TLS: STARTTLS is ignored",
			useTLS: true,
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,
				TLS: codersdk.NotificationsEmailTLSConfig{
					InsecureSkipVerify: true,
					StartTLS:           true,
				},
			},
			toAddrs: []string{to},
		},
		{
			// Plain connection is established and upgraded via STARTTLS, but certificate is untrusted.
			name:   "TLS: STARTTLS untrusted",
			useTLS: false,
			cfg: codersdk.NotificationsEmailConfig{
				TLS: codersdk.NotificationsEmailTLSConfig{
					InsecureSkipVerify: false,
					StartTLS:           true,
				},
				ForceTLS: false,
			},
			expectedErr: "tls: failed to verify certificate",
			retryable:   true,
		},
		{
			// Plain connection is established and upgraded via STARTTLS, certificate is not verified.
			name:   "TLS: STARTTLS",
			useTLS: false,
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,
				TLS: codersdk.NotificationsEmailTLSConfig{
					InsecureSkipVerify: true,
					StartTLS:           true,
				},
				ForceTLS: false,
			},
			toAddrs: []string{to},
		},
		{
			// TLS connection using self-signed certificate.
			name:   "TLS: self-signed",
			useTLS: true,
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,
				TLS: codersdk.NotificationsEmailTLSConfig{
					CAFile:   caFile,
					CertFile: certFile,
					KeyFile:  keyFile,
				},
			},
			toAddrs: []string{to},
		},
		{
			// TLS connection using self-signed certificate & specifying the DNS name configured in the certificate.
			name:   "TLS: self-signed + SNI",
			useTLS: true,
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,
				TLS: codersdk.NotificationsEmailTLSConfig{
					ServerName: "myserver.local",
					CAFile:     caFile,
					CertFile:   certFile,
					KeyFile:    keyFile,
				},
			},
			toAddrs: []string{to},
		},
		{
			name:   "TLS: load CA",
			useTLS: true,
			cfg: codersdk.NotificationsEmailConfig{
				TLS: codersdk.NotificationsEmailTLSConfig{
					CAFile: "nope.crt",
				},
			},
			// not using full error message here since it differs on *nix and Windows:
			// *nix: no such file or directory
			// Windows: The system cannot find the file specified.
			expectedErr: "open nope.crt:",
			retryable:   true,
		},
		{
			name:   "TLS: load cert",
			useTLS: true,
			cfg: codersdk.NotificationsEmailConfig{
				TLS: codersdk.NotificationsEmailTLSConfig{
					CAFile:   caFile,
					CertFile: "smtptest/fixtures/nope.cert",
					KeyFile:  keyFile,
				},
			},
			// not using full error message here since it differs on *nix and Windows:
			// *nix: no such file or directory
			// Windows: The system cannot find the file specified.
			expectedErr: "open smtptest/fixtures/nope.cert:",
			retryable:   true,
		},
		{
			name:   "TLS: load cert key",
			useTLS: true,
			cfg: codersdk.NotificationsEmailConfig{
				TLS: codersdk.NotificationsEmailTLSConfig{
					CAFile:   caFile,
					CertFile: certFile,
					KeyFile:  "smtptest/fixtures/nope.key",
				},
			},
			// not using full error message here since it differs on *nix and Windows:
			// *nix: no such file or directory
			// Windows: The system cannot find the file specified.
			expectedErr: "open smtptest/fixtures/nope.key:",
			retryable:   true,
		},
		/**
		 * Kitchen sink
		 */
		{
			name:      "PLAIN auth and TLS",
			useTLS:    true,
			authMechs: []string{sasl.Plain},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,
				Auth: codersdk.NotificationsEmailAuthConfig{
					Identity: identity,
					Username: username,
					Password: password,
				},
				TLS: codersdk.NotificationsEmailTLSConfig{
					CAFile:   caFile,
					CertFile: certFile,
					KeyFile:  keyFile,
				},
			},
			toAddrs:          []string{to},
			expectedAuthMeth: sasl.Plain,
		},
		/**
		 * Other errors
		 */
		{
			name: "Rejected on DATA",
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,
			},
			failOnDataFn: func() error {
				return &smtp.SMTPError{Code: 501, EnhancedCode: smtp.EnhancedCode{5, 5, 4}, Message: "Rejected!"}
			},
			expectedErr: "SMTP error 501: Rejected!",
			retryable:   true,
		},
	}

	// nolint:paralleltest // Reinitialization is not required as of Go v1.22.
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)

			tc.cfg.ForceTLS = serpent.Bool(tc.useTLS)

			backend := smtptest.NewBackend(smtptest.Config{
				AuthMechanisms: tc.authMechs,

				AcceptedIdentity: tc.cfg.Auth.Identity.String(),
				AcceptedUsername: username,
				AcceptedPassword: password,

				FailOnDataFn: tc.failOnDataFn,
			})

			// Create a mock SMTP server which conditionally listens for plain or TLS connections.
			srv, listen, err := smtptest.CreateMockSMTPServer(backend, tc.useTLS)
			require.NoError(t, err)
			t.Cleanup(func() {
				// We expect that the server has already been closed in the test
				assert.ErrorIs(t, srv.Shutdown(ctx), smtp.ErrServerClosed)
			})

			errs := bytes.NewBuffer(nil)
			srv.ErrorLog = log.New(errs, "oops", 0)
			// Enable this to debug mock SMTP server.
			// srv.Debug = os.Stderr

			var hp serpent.HostPort
			require.NoError(t, hp.Set(listen.Addr().String()))
			tc.cfg.Smarthost = serpent.String(hp.String())

			handler := dispatch.NewSMTPHandler(tc.cfg, logger.Named("smtp"))

			// Start mock SMTP server in the background.
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				assert.NoError(t, srv.Serve(listen))
			}()

			// Wait for the server to become pingable.
			require.Eventually(t, func() bool {
				cl, err := smtptest.PingClient(listen, tc.useTLS, tc.cfg.TLS.StartTLS.Value())
				if err != nil {
					t.Logf("smtp not yet dialable: %s", err)
					return false
				}

				if err = cl.Noop(); err != nil {
					t.Logf("smtp not yet noopable: %s", err)
					return false
				}

				if err = cl.Close(); err != nil {
					t.Logf("smtp didn't close properly: %s", err)
					return false
				}

				return true
			}, testutil.WaitShort, testutil.IntervalFast)

			// Build a fake payload.
			payload := types.MessagePayload{
				Version:   "1.0",
				UserEmail: to,
				Labels:    make(map[string]string),
			}

			dispatchFn, err := handler.Dispatcher(payload, subject, body, helpers())
			require.NoError(t, err)

			msgID := uuid.New()
			retryable, err := dispatchFn(ctx, msgID)

			if tc.expectedErr == "" {
				require.Nil(t, err)
				require.Empty(t, errs.Bytes())

				msg := backend.LastMessage()
				require.NotNil(t, msg)
				backend.Reset()

				require.Equal(t, tc.expectedAuthMeth, msg.AuthMech)
				require.Equal(t, from, msg.From)
				require.Equal(t, tc.toAddrs, msg.To)
				if !tc.cfg.Auth.Empty() {
					require.Equal(t, tc.cfg.Auth.Identity.String(), msg.Identity)
					require.Equal(t, username, msg.Username)
					require.Equal(t, password, msg.Password)
				}
				require.Contains(t, msg.Contents, subject)
				require.Contains(t, msg.Contents, body)
				require.Contains(t, msg.Contents, fmt.Sprintf("Message-Id: %s", msgID))
			} else {
				require.ErrorContains(t, err, tc.expectedErr)
			}

			require.Equal(t, tc.retryable, retryable)

			require.NoError(t, srv.Shutdown(ctx))
			wg.Wait()
		})
	}
}
