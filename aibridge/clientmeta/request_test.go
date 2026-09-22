package clientmeta_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/clientmeta"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
)

func TestIsWebSocketUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		method                string
		connection            string
		upgrade               string
		wantConnectionUpgrade bool
		wantWebSocketUpgrade  bool
	}{
		{name: "websocket upgrade", method: http.MethodGet, connection: "keep-alive, Upgrade", upgrade: "WebSocket", wantConnectionUpgrade: true, wantWebSocketUpgrade: true},
		{name: "non-GET request", method: http.MethodPost, connection: "Upgrade", upgrade: "websocket", wantConnectionUpgrade: true},
		{name: "missing connection upgrade", method: http.MethodGet, connection: "keep-alive", upgrade: "websocket"},
		{name: "different upgrade protocol", method: http.MethodGet, connection: "Upgrade", upgrade: "h2c", wantConnectionUpgrade: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), tc.method, "/", nil)
			require.NoError(t, err)
			req.Header.Set("Connection", tc.connection)
			req.Header.Set("Upgrade", tc.upgrade)

			require.Equal(t, tc.wantConnectionUpgrade, clientmeta.HasConnectionUpgrade(req))
			require.Equal(t, tc.wantWebSocketUpgrade, clientmeta.IsWebSocketUpgrade(req))
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

		errContains string
		wantSession *string
		wantSeq     *int32
	}{
		{
			name:        "both headers present",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("42"),
			wantSession: ptr(validSessionID),
			wantSeq:     new(int32(42)),
		},
		{
			name: "no headers present",
		},
		{
			name:        "only session ID returns error",
			sessionID:   ptr(validSessionID),
			errContains: "without sequence number",
		},
		{
			name:        "only sequence number returns error",
			seqNumber:   ptr("7"),
			errContains: "without session ID",
		},
		{
			name:        "sequence number zero",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("0"),
			wantSession: ptr(validSessionID),
			wantSeq:     new(int32(0)),
		},
		{
			name:        "invalid session ID returns error",
			sessionID:   ptr("not-a-uuid"),
			seqNumber:   ptr("42"),
			errContains: "must be a UUID",
		},
		{
			name:        "invalid sequence number returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("not-a-number"),
			errContains: "must be a base-10 int32",
		},
		{
			name:        "negative sequence number returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("-1"),
			errContains: "must be non-negative",
		},
		{
			name:        "sequence number exceeding int32 range returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("2147483648"), // max int32 + 1
			errContains: "must be a base-10 int32",
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

			sessionID, seqNumber, extractErr := clientmeta.ExtractAgentFirewallHeaders(req)

			if tc.errContains != "" {
				require.ErrorContains(t, extractErr, tc.errContains)
				if tc.sessionID != nil {
					require.NotContains(t, extractErr.Error(), *tc.sessionID)
				}
				if tc.seqNumber != nil {
					require.NotContains(t, extractErr.Error(), *tc.seqNumber)
				}
				require.Nil(t, sessionID)
				require.Nil(t, seqNumber)
				return
			}

			require.NoError(t, extractErr)
			require.Equal(t, tc.wantSession, sessionID)
			require.Equal(t, tc.wantSeq, seqNumber)
		})
	}
}
