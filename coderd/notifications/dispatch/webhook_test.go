package dispatch_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/coder/serpent"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
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
		name           string
		serverURL      string
		serverDeadline time.Time
		serverFn       func(uuid.UUID, http.ResponseWriter, *http.Request)

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
			name:            "timeout",
			serverDeadline:  time.Now().Add(-time.Hour),
			expectSuccess:   false,
			expectRetryable: true,
			serverFn: func(u uuid.UUID, writer http.ResponseWriter, request *http.Request) {
				t.Fatalf("should not get here")
			},
			expectErr: "request timeout",
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

			var (
				ctx    context.Context
				cancel context.CancelFunc
			)

			if !tc.serverDeadline.IsZero() {
				ctx, cancel = context.WithDeadline(context.Background(), tc.serverDeadline)
			} else {
				ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitLong)
			}
			t.Cleanup(cancel)

			var (
				err   error
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

			vals := coderdtest.DeploymentValues(t, func(values *codersdk.DeploymentValues) {
				require.NoError(t, values.Notifications.Webhook.Endpoint.Set(endpoint.String()))
			})
			handler := dispatch.NewWebhookHandler(vals.Notifications.Webhook, logger.With(slog.F("test", tc.name)))
			deliveryFn, err := handler.Dispatcher(runtimeconfig.NewNoopManager(), msgPayload, titleTemplate, bodyTemplate)
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

func TestRuntimeEndpointChange(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	const (
		titleTemplate = "this is the title ({{.Labels.foo}})"
		bodyTemplate  = "this is the body ({{.Labels.baz}})"

		startEndpoint = "http://localhost:0"
	)

	msgPayload := types.MessagePayload{
		Version:          "1.0",
		NotificationName: "test",
		Labels: map[string]string{
			"foo": "bar",
			"baz": "quux",
		},
	}

	// Setup: start up a mock HTTP server
	received := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r
		close(received)
	}))
	t.Cleanup(server.Close)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	runtimeEndpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	_ = runtimeEndpoint

	// Initially, set the endpoint to a hostport we know to not be listening for HTTP requests.
	vals := coderdtest.DeploymentValues(t, func(values *codersdk.DeploymentValues) {
		require.NoError(t, values.Notifications.Webhook.Endpoint.Set(startEndpoint))
	})

	// Setup runtime config manager.
	mgr := runtimeconfig.NewStoreManager(dbmem.New())

	// Dispatch a notification and it will fail.
	handler := dispatch.NewWebhookHandler(vals.Notifications.Webhook, logger.With(slog.F("test", t.Name())))
	deliveryFn, err := handler.Dispatcher(mgr, msgPayload, titleTemplate, bodyTemplate)
	require.NoError(t, err)

	msgID := uuid.New()
	_, err = deliveryFn(ctx, msgID)
	require.ErrorContains(t, err, "can't assign requested address")

	// Set the runtime value to the mock HTTP server.
	require.NoError(t, vals.Notifications.Webhook.Endpoint.SetRuntimeValue(ctx, mgr, serpent.URLOf(runtimeEndpoint)))
	deliveryFn, err = handler.Dispatcher(mgr, msgPayload, titleTemplate, bodyTemplate)
	require.NoError(t, err)
	_, err = deliveryFn(ctx, msgID)
	require.NoError(t, err)
	testutil.RequireRecvCtx(ctx, t, received)
}
