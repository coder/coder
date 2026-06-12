package workspacesdk_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

// TestAgentConn_DialBoundedByRequestContext verifies that the
// transport dial behind the agent HTTP API stops when the request
// context ends. http.Transport detaches dial contexts from the
// request context so a pending dial can outlive its request and
// serve future ones, but the agent API client is request-scoped
// with keep-alives disabled, so a detached dial can never be
// reused. If the transport does not re-link cancellation, the dial
// goroutine stays blocked in AwaitReachable pinging an unreachable
// agent forever, even after the tailnet conn is closed, and leaks.
//
//nolint:paralleltest // goleak.IgnoreCurrent requires this test to run non-parallel.
func TestAgentConn_DialBoundedByRequestContext(t *testing.T) {
	// goleak.IgnoreCurrent snapshots running goroutines, so this
	// test must not run in parallel with other tests.
	logger := testutil.Logger(t)

	// Snapshot before the tailnet conn exists so everything spawned
	// below, including the transport dial goroutine, is verified.
	ignoreCurrent := goleak.IgnoreCurrent()

	tailnetConn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
		Logger:    logger.Named("client"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tailnetConn.Close()
	})

	conn := workspacesdk.NewAgentConn(tailnetConn, workspacesdk.AgentConnOptions{
		AgentID: uuid.New(),
	})

	// No agent exists, so the transport dial blocks in
	// AwaitReachable until the request context expires. The timeout
	// only needs to be long enough for the dial goroutine to start;
	// its expiry is the behavior under test.
	ctx, cancel := context.WithTimeout(context.Background(), testutil.IntervalSlow)
	defer cancel()
	_, err = conn.ListeningPorts(ctx)
	require.Error(t, err)

	// Close the conn like test teardown would. The conn's own
	// goroutines exit on close; the dial goroutine must have already
	// exited when the request context expired.
	err = tailnetConn.Close()
	require.NoError(t, err)

	goleak.VerifyNone(t, ignoreCurrent)
}
