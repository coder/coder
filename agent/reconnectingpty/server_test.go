package reconnectingpty_test

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/reconnectingpty"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestServerShutdownReason(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("test command requires a POSIX shell")
	}
	for _, tc := range []struct {
		name        string
		clientClose bool
		wantReason  codersdk.DisconnectReason
	}{
		{name: "AgentShutdown", wantReason: codersdk.DisconnectReasonServerShutdown},
		{name: "ClientCloseBeforeShutdown", clientClose: true, wantReason: codersdk.DisconnectReasonGraceful},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			gracefulCtx, gracefulCancel := context.WithCancel(ctx)
			defer gracefulCancel()
			sink := testutil.NewFakeSink(t)
			sshServer, err := agentssh.NewServer(ctx, testutil.Logger(t), prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, &agentssh.Config{})
			require.NoError(t, err)
			t.Cleanup(func() { _ = sshServer.Close() })
			reporter := &shutdownReporter{}
			server := reconnectingpty.NewServer(sink.Logger(), sshServer, reporter,
				prometheus.NewCounter(prometheus.CounterOpts{Name: "connections_total"}),
				prometheus.NewCounterVec(prometheus.CounterOpts{Name: "errors_total"}, []string{"error_type"}),
				time.Minute,
			)
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			clientSessionID := uuid.NewString()
			serveDone := make(chan error, 1)
			go func() {
				serveDone <- server.Serve(gracefulCtx, ctx, &sessionListener{Listener: ln, clientSessionID: clientSessionID})
			}()

			conn, err := net.Dial("tcp", ln.Addr().String())
			require.NoError(t, err)
			defer conn.Close()
			data, err := json.Marshal(workspacesdk.AgentReconnectingPTYInit{
				ID: uuid.New(), Height: 80, Width: 80, Command: "echo ready; exec cat", BackendType: "buffered",
			})
			require.NoError(t, err)
			data = append(make([]byte, 2), data...)
			// #nosec G115 - The init message is far below the uint16 limit.
			binary.LittleEndian.PutUint16(data, uint16(len(data)-2))
			_, err = conn.Write(data)
			require.NoError(t, err)
			tr := testutil.NewTerminalReader(t, conn)
			require.NoError(t, tr.ReadUntilString(ctx, "ready"))

			closedLogs := func() []slog.SinkEntry {
				return sink.Entries(func(e slog.SinkEntry) bool { return e.Message == "reconnecting pty closed" })
			}
			if tc.clientClose {
				require.NoError(t, conn.Close())
				require.Eventually(t, func() bool { return len(closedLogs()) == 1 }, testutil.WaitLong, testutil.IntervalFast)
			}
			gracefulCancel()
			require.NoError(t, ln.Close())
			testutil.RequireReceive(ctx, t, serveDone)

			entries := closedLogs()
			require.Len(t, entries, 1)
			fields := make(map[string]any)
			for _, field := range entries[0].Fields {
				fields[field.Name] = field.Value
			}
			require.Equal(t, clientSessionID, fields["client_session_id"])
			require.Equal(t, tc.wantReason, fields["disconnect_reason"])
			disconnects := reporter.Disconnects()
			require.Len(t, disconnects, 1)
			require.Equal(t, string(tc.wantReason), disconnects[0].Reason)
			if tc.wantReason == codersdk.DisconnectReasonServerShutdown {
				require.Equal(t, codersdk.DisconnectInitiatorAgent, fields["disconnect_initiator"])
			}
		})
	}
}

// sessionListener attaches a client session ID to every accepted connection,
// as the agent's upgraded connections do.
type sessionListener struct {
	net.Listener
	clientSessionID string
}

func (l *sessionListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return sessionConn{Conn: conn, clientSessionID: l.clientSessionID}, nil
}

type sessionConn struct {
	net.Conn
	clientSessionID string
}

func (c sessionConn) ClientSessionID() string { return c.clientSessionID }

type shutdownReporter struct {
	mu          sync.Mutex
	disconnects []proto.DisconnectEvent
}

func (r *shutdownReporter) Connect(proto.ConnectEvent) proto.DisconnectionReporter { return r }

func (r *shutdownReporter) Disconnect(event proto.DisconnectEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.disconnects = append(r.disconnects, event)
}

func (r *shutdownReporter) Disconnects() []proto.DisconnectEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]proto.DisconnectEvent(nil), r.disconnects...)
}
