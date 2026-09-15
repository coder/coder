package aibridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/mcpmock"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/testutil"
)

// InflightRequests returns the number of admitted requests still running.
func (b *RequestBridge) InflightRequests() int {
	b.inflight.mu.Lock()
	defer b.inflight.mu.Unlock()
	return b.inflight.active
}

func TestIsWebSocketUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		connection string
		upgrade    string
		want       bool
	}{
		{name: "websocket upgrade", method: http.MethodGet, connection: "keep-alive, Upgrade", upgrade: "WebSocket", want: true},
		{name: "non-GET request", method: http.MethodPost, connection: "Upgrade", upgrade: "websocket", want: false},
		{name: "missing connection upgrade", method: http.MethodGet, connection: "keep-alive", upgrade: "websocket", want: false},
		{name: "different upgrade protocol", method: http.MethodGet, connection: "Upgrade", upgrade: "h2c", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), tc.method, "/", nil)
			require.NoError(t, err)
			req.Header.Set("Connection", tc.connection)
			req.Header.Set("Upgrade", tc.upgrade)

			assert.Equal(t, tc.want, isWebSocketUpgrade(req))
		})
	}
}

func TestExtractAgentFirewallHeaders(t *testing.T) {
	t.Parallel()

	const validSessionID = "e5f6a7b8-1234-5678-9abc-def012345678"

	ptr := func(s string) *string { return &s }

	cases := []struct {
		name string
		// sessionID and seqNumber set the corresponding headers when
		// non-nil. A nil value leaves the header unset.
		sessionID *string
		seqNumber *string

		wantErr     bool
		errContains string
		wantSession *string
		wantSeq     *int32
	}{
		{
			name:        "both headers present",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("42"),
			wantSession: ptr(validSessionID),
			wantSeq:     int32Ptr(42),
		},
		{
			name: "no headers present",
		},
		{
			name:        "only session ID returns error",
			sessionID:   ptr(validSessionID),
			wantErr:     true,
			errContains: "without sequence number",
		},
		{
			name:        "only sequence number returns error",
			seqNumber:   ptr("7"),
			wantErr:     true,
			errContains: "without session ID",
		},
		{
			name:        "sequence number zero",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("0"),
			wantSession: ptr(validSessionID),
			wantSeq:     int32Ptr(0),
		},
		{
			name:        "invalid session ID returns error",
			sessionID:   ptr("not-a-uuid"),
			seqNumber:   ptr("42"),
			wantErr:     true,
			errContains: "invalid agent firewall session ID",
		},
		{
			name:        "invalid sequence number returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("not-a-number"),
			wantErr:     true,
			errContains: "invalid agent firewall sequence number",
		},
		{
			name:        "negative sequence number returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("-1"),
			wantErr:     true,
			errContains: "must be non-negative",
		},
		{
			name:        "sequence number exceeding int32 range returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("2147483648"), // max int32 + 1
			wantErr:     true,
			errContains: "invalid agent firewall sequence number",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
			require.NoError(t, err)
			if tc.sessionID != nil {
				req.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, *tc.sessionID)
			}
			if tc.seqNumber != nil {
				req.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, *tc.seqNumber)
			}

			sessionID, seqNumber, extractErr := extractAgentFirewallHeaders(req)

			if tc.wantErr {
				require.Error(t, extractErr)
				assert.Contains(t, extractErr.Error(), tc.errContains)
				assert.Nil(t, sessionID)
				assert.Nil(t, seqNumber)
				return
			}

			require.NoError(t, extractErr)
			if tc.wantSession == nil {
				assert.Nil(t, sessionID)
			} else {
				require.NotNil(t, sessionID)
				assert.Equal(t, *tc.wantSession, *sessionID)
			}
			if tc.wantSeq == nil {
				assert.Nil(t, seqNumber)
			} else {
				require.NotNil(t, seqNumber)
				assert.Equal(t, *tc.wantSeq, *seqNumber)
			}
		})
	}
}

func int32Ptr(n int32) *int32 { return &n }

// TestRequestBridgeShutdownDoesNotWaitForCanceledHandler verifies shutdown
// returns even when a canceled handler remains blocked.
func TestRequestBridgeShutdownDoesNotWaitForCanceledHandler(t *testing.T) {
	t.Parallel()
	for _, withMCP := range []bool{false, true} {
		name := "WithoutMCP"
		if withMCP {
			name = "WithMCP"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			// Set up a handler that only test cleanup can release.
			started := make(chan context.Context, 1)
			release := make(chan struct{})
			finished := make(chan struct{})
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(_ http.ResponseWriter, r *http.Request) {
				started <- r.Context()
				<-release // Deliberately ignore request cancellation.
			})
			bridge := &RequestBridge{
				inflight: NewInflightGate(),
				logger:   slogtest.Make(t, nil),
			}
			bridge.handler = bridge.inflight.Middleware(nil)(mux)
			// MCP cleanup must be attempted while the handler is still blocked.
			if withMCP {
				proxy := mcpmock.NewMockServerProxier(gomock.NewController(t))
				proxy.EXPECT().Shutdown(gomock.Any()).DoAndReturn(func(shutdownCtx context.Context) error {
					assert.ErrorIs(t, shutdownCtx.Err(), context.DeadlineExceeded)
					assert.EqualValues(t, 1, bridge.InflightRequests())
					return nil
				})
				bridge.mcpProxy = proxy
			}
			t.Cleanup(func() {
				close(release)
				testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, finished)
			})
			go func() {
				defer close(finished)
				bridge.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
			}()
			requestCtx := testutil.TryReceive(ctx, t, started)
			// Expire shutdown while the request is in flight.
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
			shutdownDone := make(chan error, 1)
			go func() { shutdownDone <- bridge.Shutdown(deadlineCtx) }()
			// Shutdown returns and cancels the context without waiting for release.
			require.ErrorIs(t, testutil.TryReceive(ctx, t, shutdownDone), context.DeadlineExceeded)
			testutil.TryReceive(ctx, t, requestCtx.Done())
			require.EqualValues(t, 1, bridge.InflightRequests(), "handler is still blocked")
			// Admission stays closed even though the old handler is still running.
			rejected := httptest.NewRecorder()
			bridge.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/", nil))
			require.Equal(t, http.StatusServiceUnavailable, rejected.Code)
			require.Equal(t, "AI Gateway is shutting down\n", rejected.Body.String())
		})
	}
}
