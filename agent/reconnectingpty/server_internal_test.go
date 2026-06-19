package reconnectingpty

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// oneByteConn is a net.Conn whose Read returns at most a single byte per call,
// simulating a network connection that delivers the init message across
// multiple segments.
type oneByteConn struct {
	data []byte
	pos  int
}

func (c *oneByteConn) Read(p []byte) (int, error) {
	if c.pos >= len(c.data) {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = c.data[c.pos]
	c.pos++
	return 1, nil
}

func (*oneByteConn) Write(p []byte) (int, error)      { return len(p), nil }
func (*oneByteConn) Close() error                     { return nil }
func (*oneByteConn) LocalAddr() net.Addr              { return testAddr{} }
func (*oneByteConn) RemoteAddr() net.Addr             { return testAddr{} }
func (*oneByteConn) SetDeadline(time.Time) error      { return nil }
func (*oneByteConn) SetReadDeadline(time.Time) error  { return nil }
func (*oneByteConn) SetWriteDeadline(time.Time) error { return nil }

type testAddr struct{}

func (testAddr) Network() string { return "test" }
func (testAddr) String() string  { return "test" }

// recordingReconnectingPTY records the parameters passed to Attach so the test
// can assert that the init message was fully decoded before the session began.
type recordingReconnectingPTY struct {
	attached     bool
	attachHeight uint16
	attachWidth  uint16
}

func (r *recordingReconnectingPTY) Attach(_ context.Context, _ string, _ net.Conn, height, width uint16, _ slog.Logger) error {
	r.attached = true
	r.attachHeight = height
	r.attachWidth = width
	return nil
}

func (*recordingReconnectingPTY) Wait()       {}
func (*recordingReconnectingPTY) Close(error) {}

// TestHandleConnInitSpansMultipleReads verifies that handleConn assembles the
// length-prefixed AgentReconnectingPTYInit even when the connection delivers it
// across multiple short reads. A single conn.Read can return fewer bytes than
// requested, truncating the init and causing json.Unmarshal to fail so the PTY
// never starts.
func TestHandleConnInitSpansMultipleReads(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	srv := NewServer(
		logger,
		nil, // commandCreator is unused on the attach-to-existing path.
		nil,
		prometheus.NewCounter(prometheus.CounterOpts{Name: "test_connections_total"}),
		prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_errors_total"}, []string{"type"}),
		0,
	)

	// Pre-register a reconnecting PTY so handleConn takes the
	// "connecting to existing reconnecting pty" path. This avoids creating a
	// command or spawning a process, keeping the test focused on reading and
	// decoding the init message.
	id := uuid.New()
	rpty := &recordingReconnectingPTY{}
	ready := make(chan ReconnectingPTY, 1)
	ready <- rpty
	srv.reconnectingPTYs.Store(id, ready)

	const wantHeight, wantWidth uint16 = 24, 80
	body, err := json.Marshal(workspacesdk.AgentReconnectingPTYInit{
		ID:      id,
		Height:  wantHeight,
		Width:   wantWidth,
		Command: "echo hello",
	})
	require.NoError(t, err)

	frame := make([]byte, 2+len(body))
	// #nosec G115 - the marshaled test init is far below the uint16 maximum.
	binary.LittleEndian.PutUint16(frame[:2], uint16(len(body)))
	copy(frame[2:], body)

	err = srv.handleConn(ctx, logger, &oneByteConn{data: frame})
	require.NoError(t, err)

	require.True(t, rpty.attached, "expected the session to start after decoding the init")
	require.Equal(t, wantHeight, rpty.attachHeight)
	require.Equal(t, wantWidth, rpty.attachWidth)
}
