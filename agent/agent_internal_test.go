package agent

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestReportConnectionEmpty tests that reportConnection() doesn't choke if given an empty IP string, which is what we
// send if we cannot get the remote address.
func TestReportConnectionEmpty(t *testing.T) {
	t.Parallel()
	connID := uuid.UUID{1}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := testutil.Context(t, testutil.WaitShort)

	uut := &agent{
		hardCtx: ctx,
		logger:  logger,
	}
	disconnected := uut.reportConnection(connID, proto.Connection_TYPE_UNSPECIFIED, "")

	require.Len(t, uut.reportConnections, 1)
	req0 := uut.reportConnections[0]
	require.Equal(t, proto.Connection_TYPE_UNSPECIFIED, req0.GetConnection().GetType())
	require.Equal(t, "", req0.GetConnection().Ip)
	require.Equal(t, connID[:], req0.GetConnection().GetId())
	require.Equal(t, proto.Connection_CONNECT, req0.GetConnection().GetAction())

	disconnected(0, "because")
	require.Len(t, uut.reportConnections, 2)
	req1 := uut.reportConnections[1]
	require.Equal(t, proto.Connection_TYPE_UNSPECIFIED, req1.GetConnection().GetType())
	require.Equal(t, "", req1.GetConnection().Ip)
	require.Equal(t, connID[:], req1.GetConnection().GetId())
	require.Equal(t, proto.Connection_DISCONNECT, req1.GetConnection().GetAction())
	require.Equal(t, "because", req1.GetConnection().GetReason())
}
