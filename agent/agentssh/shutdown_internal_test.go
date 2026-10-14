package agentssh

import (
	"context"
	"net"
	"sync"
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
			name = "ServerShutdown"
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
				conn.shutdown.Store(true)
			}
			require.NoError(t, channel.Close())
			require.NoError(t, channel.Close())
			require.Equal(t, clientSessionID, reporter.connect.ClientSessionID)
			require.Len(t, reporter.disconnects, 1)
			wantReason := "normal close"
			if shutdown {
				wantReason = string(codersdk.DisconnectReasonServerShutdown)
			}
			require.Equal(t, wantReason, reporter.disconnects[0].Reason)
			entries := sink.Entries(func(e slog.SinkEntry) bool { return e.Message == "JetBrains channel closed" })
			require.Len(t, entries, 1)
			require.Contains(t, entries[0].Fields, slog.F("client_session_id", clientSessionID))
			if shutdown {
				require.Contains(t, entries[0].Fields, codersdk.DisconnectReasonServerShutdown.SlogField())
			}
		})
	}
}

type shutdownNewChannel struct{ ssh.NewChannel }

func (shutdownNewChannel) Accept() (ssh.Channel, <-chan *ssh.Request, error) {
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

func TestSSHShutdownWaitsForConnectionLog(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	release := make(chan struct{})
	releaseLog := sync.OnceFunc(func() { close(release) })
	defer releaseLog()
	sink := &blockedConnectionLogSink{entered: make(chan slog.SinkEntry, 1), release: release}
	server, err := NewServer(ctx, slog.Make(sink), prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, &Config{})
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
	require.Contains(t, entry.Fields, codersdk.DisconnectReasonServerShutdown.SlogField())
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
