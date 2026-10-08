package agentssh

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestJetbrainsShutdownReason(t *testing.T) {
	t.Parallel()
	for _, shutdown := range []bool{false, true} {
		name := "Graceful"
		if shutdown {
			name = "WorkspaceShutdown"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			conn := &trackedSSHConn{}
			ctx := context.WithValue(testutil.Context(t, testutil.WaitLong), shutdownContextKey{}, conn)
			sink := testutil.NewFakeSink(t)
			clientSessionID := uuid.NewString()
			reporter := &shutdownReporter{}
			watcher := &JetbrainsChannelWatcher{
				NewChannel: shutdownNewChannel{}, ctx: ctx,
				logger:          sink.Logger().With(slog.F("client_session_id", clientSessionID)),
				clientSessionID: clientSessionID, connectionReporter: reporter,
				startSession: func(string) func() { return func() {} },
			}
			channel, _, err := watcher.Accept()
			require.NoError(t, err)
			if shutdown {
				conn.recordTermination(&ShutdownCause{Reason: codersdk.DisconnectReasonWorkspaceStopped, BuildID: uuid.NewString(), Transition: "stop"})
			}
			require.NoError(t, channel.Close())
			require.NoError(t, channel.Close())
			require.Equal(t, clientSessionID, reporter.connect.ClientSessionID)
			require.Len(t, reporter.disconnects, 1)
			wantReason := "normal close"
			if shutdown {
				wantReason = string(codersdk.DisconnectReasonWorkspaceStopped)
			}
			require.Equal(t, wantReason, reporter.disconnects[0].Reason)
			entries := sink.Entries(func(e slog.SinkEntry) bool { return e.Message == "JetBrains channel closed" })
			require.Len(t, entries, 1)
			require.Contains(t, entries[0].Fields, slog.F("client_session_id", clientSessionID))
			if shutdown {
				require.Contains(t, entries[0].Fields, codersdk.DisconnectReasonWorkspaceStopped.SlogField())
			}
		})
	}
}

func TestSSHTransportTermination(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		err          error
		wantShutdown bool
	}{
		{name: "Success", wantShutdown: true},
		{name: "EOF", err: io.EOF},
		{name: "ClosedPipe", err: io.ErrClosedPipe},
		{name: "ConnectionReset", err: &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}},
		{name: "Deadline", err: os.ErrDeadlineExceeded, wantShutdown: true},
		{name: "WrappedDeadline", err: &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}, wantShutdown: true},
		{name: "Temporary", err: &net.OpError{Op: "read", Net: "tcp", Err: syscall.EAGAIN}, wantShutdown: true},
	} {
		for _, operation := range []string{"Read", "Write"} {
			t.Run(tc.name+"/"+operation, func(t *testing.T) {
				t.Parallel()
				conn := &trackedSSHConn{Conn: shutdownErrorConn{err: tc.err}}
				if operation == "Read" {
					_, err := conn.Read(nil)
					require.Equal(t, tc.err, err)
				} else {
					_, err := conn.Write(nil)
					require.Equal(t, tc.err, err)
				}
				cause := &ShutdownCause{Reason: codersdk.DisconnectReasonWorkspaceStopped, BuildID: uuid.NewString()}
				conn.recordTermination(cause)
				if tc.wantShutdown {
					require.Equal(t, cause, conn.shutdownCause())
				} else {
					require.Nil(t, conn.shutdownCause())
				}
			})
		}
	}

	t.Run("CloseBeforeShutdown", func(t *testing.T) {
		t.Parallel()
		conn := &trackedSSHConn{Conn: shutdownErrorConn{}}
		require.NoError(t, conn.Close())
		conn.recordTermination(&ShutdownCause{Reason: codersdk.DisconnectReasonWorkspaceStopped})
		require.Nil(t, conn.shutdownCause())
	})

	t.Run("ShutdownBeforeError", func(t *testing.T) {
		t.Parallel()
		conn := &trackedSSHConn{Conn: shutdownErrorConn{err: io.EOF}}
		cause := &ShutdownCause{Reason: codersdk.DisconnectReasonWorkspaceStopped, BuildID: uuid.NewString()}
		conn.recordTermination(cause)
		_, err := conn.Read(nil)
		require.ErrorIs(t, err, io.EOF)
		require.NoError(t, conn.Close())
		conn.recordTermination(&ShutdownCause{Reason: codersdk.DisconnectReasonServerShutdown})
		require.Equal(t, cause, conn.shutdownCause())
	})
}

type shutdownErrorConn struct {
	net.Conn
	err error
}

func (c shutdownErrorConn) Read([]byte) (int, error)  { return 0, c.err }
func (c shutdownErrorConn) Write([]byte) (int, error) { return 0, c.err }
func (shutdownErrorConn) Close() error                { return nil }

type shutdownNewChannel struct {
	ssh.NewChannel
	channel ssh.Channel
}

func (c shutdownNewChannel) Accept() (ssh.Channel, <-chan *ssh.Request, error) {
	if c.channel != nil {
		return c.channel, nil, nil
	}
	return shutdownChannel{}, nil, nil
}

type shutdownChannel struct{ ssh.Channel }

func (shutdownChannel) Close() error { return nil }

type shutdownReporter struct {
	connect     proto.ConnectEvent
	disconnects []proto.DisconnectEvent
}

func (r *shutdownReporter) Connect(event proto.ConnectEvent) proto.DisconnectionReporter {
	r.connect = event
	return r
}

func (r *shutdownReporter) Disconnect(event proto.DisconnectEvent) {
	r.disconnects = append(r.disconnects, event)
}

func TestJetbrainsChannelClosedBeforeShutdown(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"Close", "Read", "Write"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			conn := &trackedSSHConn{}
			ctx := context.WithValue(testutil.Context(t, testutil.WaitLong), shutdownContextKey{}, conn)
			reporter := &shutdownReporter{}
			ending := make(chan struct{})
			release := make(chan struct{})
			releaseClose := sync.OnceFunc(func() { close(release) })
			defer releaseClose()
			watcher := &JetbrainsChannelWatcher{
				NewChannel: shutdownNewChannel{channel: shutdownErrorChannel{}}, ctx: ctx,
				logger: testutil.Logger(t), connectionReporter: reporter,
				startSession: func(string) func() {
					return func() {
						if operation == "Close" {
							close(ending)
							<-release
						}
					}
				},
			}
			channel, _, err := watcher.Accept()
			require.NoError(t, err)
			done := make(chan error, 1)
			switch operation {
			case "Close":
				go func() { done <- channel.Close() }()
				testutil.TryReceive(ctx, t, ending)
			case "Read":
				_, err := channel.Read(nil)
				require.ErrorIs(t, err, io.EOF)
			case "Write":
				_, err := channel.Write(nil)
				require.ErrorIs(t, err, io.ErrClosedPipe)
			}
			conn.recordTermination(&ShutdownCause{Reason: codersdk.DisconnectReasonWorkspaceStopped, BuildID: uuid.NewString()})
			if operation == "Close" {
				releaseClose()
				require.NoError(t, testutil.RequireReceive(ctx, t, done))
			} else {
				require.NoError(t, channel.Close())
			}
			require.Len(t, reporter.disconnects, 1)
			require.Equal(t, "normal close", reporter.disconnects[0].Reason)
		})
	}
}

type shutdownErrorChannel struct{ ssh.Channel }

func (shutdownErrorChannel) Read([]byte) (int, error)  { return 0, io.EOF }
func (shutdownErrorChannel) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (shutdownErrorChannel) Close() error              { return nil }

func TestSSHShutdownWaitsForConnectionLog(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	release := make(chan struct{})
	releaseLog := sync.OnceFunc(func() { close(release) })
	defer releaseLog()
	sink := &blockedConnectionLogSink{entered: make(chan slog.SinkEntry, 1), release: release}
	server, err := NewServer(ctx, slog.Make(sink), prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, &Config{
		ShutdownCause: func() ShutdownCause {
			return ShutdownCause{Reason: codersdk.DisconnectReasonWorkspaceStopped, BuildID: uuid.NewString()}
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		releaseLog()
		_ = server.Close()
	})
	require.NoError(t, server.UpdateHostSigner(42))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		// #nosec G106 - The test connects to its own server.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	require.NoError(t, err)
	defer client.Close()
	destination, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer destination.Close()
	forward, err := client.Dial("tcp", destination.Addr().String())
	require.NoError(t, err)
	defer forward.Close()
	closeDone := make(chan error, 1)
	go func() { closeDone <- server.Close() }()
	entry := testutil.RequireReceive(ctx, t, sink.entered)
	require.Contains(t, entry.Fields, codersdk.DisconnectReasonWorkspaceStopped.SlogField())
	// Forwarding-only connections rely on this completion log. Keep their
	// shutdown wait-group entry until every sink has accepted the reason.
	server.mu.RLock()
	tracked := len(server.conns)
	server.mu.RUnlock()
	require.Equal(t, 1, tracked)
	releaseLog()
	require.NoError(t, testutil.RequireReceive(ctx, t, closeDone))
	require.Error(t, testutil.RequireReceive(ctx, t, serveDone))
}

type blockedConnectionLogSink struct {
	entered chan slog.SinkEntry
	release <-chan struct{}
}

func (s *blockedConnectionLogSink) LogEntry(_ context.Context, entry slog.SinkEntry) {
	if entry.Message != "ssh connection complete" {
		return
	}
	s.entered <- entry
	<-s.release
}

func (*blockedConnectionLogSink) Sync() {}
