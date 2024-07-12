package dispatch_test

import (
	"bytes"
	"fmt"
	"log"
	"sync"
	"testing"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/serpent"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestSMTP(t *testing.T) {
	t.Parallel()

	const (
		username = "bob"
		password = "🤫"

		hello = "localhost"

		from = "system@coder.com"
		to   = "bob@bob.com"

		subject = "This is the subject"
		body    = "This is the body"
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
	}{
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
			name:      "PLAIN auth",
			authMechs: []string{sasl.Plain},
			cfg: codersdk.NotificationsEmailConfig{
				Hello: hello,
				From:  from,

				Auth: codersdk.NotificationsEmailAuthConfig{
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
					Username: username,
					Password: password,
				},
			},
			toAddrs:          []string{to},
			expectedAuthMeth: sasl.Plain,
		},
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitSuperLong)

			backend := NewBackend(Config{
				AuthMechanisms: tc.authMechs,

				AcceptedUsername: username,
				AcceptedPassword: password,
			})

			s, listen, err := createMockServer(backend)
			require.NoError(t, err)
			t.Cleanup(func() {
				// We expect that the server has already been closed in the test
				assert.ErrorIs(t, s.Shutdown(ctx), smtp.ErrServerClosed)
			})

			errs := bytes.NewBuffer(nil)
			s.ErrorLog = log.New(errs, "", 0)
			// Enable this to debug mock SMTP server.
			// s.Debug = os.Stderr

			addr := listen.Addr().String()
			var hp serpent.HostPort
			require.NoError(t, hp.Set(addr))
			tc.cfg.Smarthost = hp

			handler := dispatch.NewSMTPHandler(tc.cfg, logger.Named("smtp"))

			// Start mock SMTP server in the background.
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				assert.NoError(t, s.Serve(listen))
			}()

			// Build a fake payload.
			payload := types.MessagePayload{
				Version:   "1.0",
				UserEmail: to,
				Labels:    make(map[string]string),
			}

			dispatchFn, err := handler.Dispatcher(payload, subject, body)
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
				require.Equal(t, tc.cfg.Auth.Identity.String(), msg.Identity)
				require.Equal(t, tc.cfg.Auth.Username.String(), msg.Username)
				require.Equal(t, tc.cfg.Auth.Password.String(), msg.Password)
				require.Contains(t, msg.Contents, subject)
				require.Contains(t, msg.Contents, body)
				require.Contains(t, msg.Contents, fmt.Sprintf("Message-Id: %s", msgID))
			} else {
				require.ErrorContains(t, err, tc.expectedErr)
			}

			require.Equal(t, tc.retryable, retryable)

			require.NoError(t, s.Close())

			wg.Wait()
		})
	}
}
