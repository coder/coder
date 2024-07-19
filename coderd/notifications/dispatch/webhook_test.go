package dispatch_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWebhook(t *testing.T) {
	t.Parallel()

	const (
		titleTemplate = "this is the title ({{.Labels.foo}})"
		bodyTemplate  = "this is the body ({{.Labels.baz}})"
	)

	msgPayload := types.MessagePayload{
		Version:          "1.0",
		NotificationName: "test",
		Labels: map[string]string{
			"foo": "bar",
			"baz": "quux",
		},
	}

	tests := []struct {
		name          string
		serverURL     string
		serverTimeout time.Duration
		serverFn      func(uuid.UUID, http.ResponseWriter, *http.Request)

		expectSuccess   bool
		expectRetryable bool
		expectErr       string
	}{
		{
			name: "successful",
			serverFn: func(msgID uuid.UUID, w http.ResponseWriter, r *http.Request) {
				var payload dispatch.WebhookPayload
				err := json.NewDecoder(r.Body).Decode(&payload)
				assert.NoError(t, err)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, msgID, payload.MsgID)
				assert.Equal(t, msgID.String(), r.Header.Get("X-Message-Id"))

				w.WriteHeader(http.StatusOK)
				_, err = w.Write([]byte(fmt.Sprintf("received %s", payload.MsgID)))
				assert.NoError(t, err)
			},
			expectSuccess: true,
		},
		{
			name: "invalid endpoint",
			// Build a deliberately invalid URL to fail validation.
			serverURL:       "invalid .com",
			expectSuccess:   false,
			expectErr:       "invalid URL escape",
			expectRetryable: false,
		},
		{
			name:            "timeout",
			serverTimeout:   time.Millisecond,
			expectSuccess:   false,
			expectRetryable: true,
			expectErr:       "request timeout",
		},
		{
			name: "non-200 response",
			serverFn: func(_ uuid.UUID, w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectSuccess:   false,
			expectRetryable: true,
			expectErr:       "non-2xx response (500)",
		},
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	// nolint:paralleltest // Irrelevant as of Go v1.22
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			timeout := testutil.WaitLong
			if tc.serverTimeout > 0 {
				timeout = tc.serverTimeout
			}

			var (
				err   error
				ctx   = testutil.Context(t, timeout)
				msgID = uuid.New()
			)

			var endpoint *url.URL
			if tc.serverURL != "" {
				endpoint = &url.URL{Host: tc.serverURL}
			} else {
				// Mock server to simulate webhook endpoint.
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					tc.serverFn(msgID, w, r)
				}))
				defer server.Close()

				endpoint, err = url.Parse(server.URL)
				require.NoError(t, err)
			}

			cfg := codersdk.NotificationsWebhookConfig{
				Endpoint: *serpent.URLOf(endpoint),
			}
			handler := dispatch.NewWebhookHandler(cfg, logger.With(slog.F("test", tc.name)))
			deliveryFn, err := handler.Dispatcher(msgPayload, titleTemplate, bodyTemplate)
			require.NoError(t, err)

			retryable, err := deliveryFn(ctx, msgID)
			if tc.expectSuccess {
				require.NoError(t, err)
				require.False(t, retryable)
				return
			}

			require.ErrorContains(t, err, tc.expectErr)
			require.Equal(t, tc.expectRetryable, retryable)
		})
	}
}
