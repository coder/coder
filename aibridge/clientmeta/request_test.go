package clientmeta_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/clientmeta"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
)

func TestHasConnectionUpgrade(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		connection string
		want       bool
	}{
		{name: "Upgrade", connection: "keep-alive, Upgrade", want: true},
		{name: "NoUpgrade", connection: "keep-alive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost", nil)
			require.NoError(t, err)
			req.Header.Set("Connection", tc.connection)
			require.Equal(t, tc.want, clientmeta.HasConnectionUpgrade(req))
		})
	}
}

func TestIsWebSocketUpgrade(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		method     string
		connection string
		upgrade    string
		want       bool
	}{
		{name: "websocket upgrade", method: http.MethodGet, connection: "keep-alive, Upgrade", upgrade: "WebSocket", want: true},
		{name: "non-GET request", method: http.MethodPost, connection: "Upgrade", upgrade: "websocket"},
		{name: "missing connection upgrade", method: http.MethodGet, connection: "keep-alive", upgrade: "websocket"},
		{name: "different upgrade protocol", method: http.MethodGet, connection: "Upgrade", upgrade: "h2c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(t.Context(), tc.method, "http://localhost", nil)
			require.NoError(t, err)
			req.Header.Set("Connection", tc.connection)
			req.Header.Set("Upgrade", tc.upgrade)
			assert.Equal(t, tc.want, clientmeta.IsWebSocketUpgrade(req))
		})
	}
}

func TestExtractAgentFirewallHeaders(t *testing.T) {
	t.Parallel()

	const validSessionID = "e5f6a7b8-1234-5678-9abc-def012345678"
	for _, tc := range []struct {
		name        string
		sessionID   *string
		sequence    *string
		wantSession *string
		wantSeq     *int32
		errContains string
	}{
		{name: "both headers present", sessionID: new(validSessionID), sequence: new("42"), wantSession: new(validSessionID), wantSeq: new(int32(42))},
		{name: "no headers present"},
		{name: "only session ID", sessionID: new(validSessionID), errContains: "without sequence number"},
		{name: "only sequence number", sequence: new("7"), errContains: "without session ID"},
		{name: "sequence zero", sessionID: new(validSessionID), sequence: new("0"), wantSession: new(validSessionID), wantSeq: new(int32(0))},
		{name: "invalid session ID", sessionID: new("not-a-uuid"), sequence: new("42"), errContains: "must be a UUID"},
		{name: "invalid sequence", sessionID: new(validSessionID), sequence: new("not-a-number"), errContains: "must be a base-10 int32"},
		{name: "negative sequence", sessionID: new(validSessionID), sequence: new("-1"), errContains: "must be non-negative"},
		{name: "sequence exceeds int32", sessionID: new(validSessionID), sequence: new("2147483648"), errContains: "must be a base-10 int32"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost", nil)
			require.NoError(t, err)
			if tc.sessionID != nil {
				req.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, *tc.sessionID)
			}
			if tc.sequence != nil {
				req.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, *tc.sequence)
			}
			sessionID, sequence, err := clientmeta.ExtractAgentFirewallHeaders(req)
			if tc.errContains != "" {
				require.ErrorContains(t, err, tc.errContains)
				if tc.sessionID != nil {
					require.NotContains(t, err.Error(), *tc.sessionID)
				}
				if tc.sequence != nil {
					require.NotContains(t, err.Error(), *tc.sequence)
				}
				require.Nil(t, sessionID)
				require.Nil(t, sequence)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantSession, sessionID)
			require.Equal(t, tc.wantSeq, sequence)
		})
	}
}
