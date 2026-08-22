package tailnet

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/tailnet/proto"
)

func TestConn_newTelemetryEvent_ClientSessionID(t *testing.T) {
	t.Parallel()

	store, err := newTelemetryStore()
	require.NoError(t, err)

	t.Run("Set", func(t *testing.T) {
		t.Parallel()

		const sessionID = "0123456789abcdef0123456789abcdef"
		c := &Conn{
			telemetryStore:  store,
			clientType:      proto.TelemetryEvent_CLI,
			clientSessionID: sessionID,
		}
		require.Equal(t, sessionID, c.newTelemetryEvent().ClientSessionId)
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()

		c := &Conn{
			telemetryStore: store,
			clientType:     proto.TelemetryEvent_CLI,
		}
		require.Empty(t, c.newTelemetryEvent().ClientSessionId)
	})
}
