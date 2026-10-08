package agentssh_test

import (
	"net"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

func TestSSHShutdownReason(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		workspace   bool
		forwardOnly bool
		remoteClose bool
		stdinEOF    bool
	}{
		{name: "AgentShutdown"},
		{name: "WorkspaceStop", workspace: true},
		{name: "WorkspaceStopAfterStdinEOF", workspace: true, stdinEOF: true},
		{name: "ForwardOnly", workspace: true, forwardOnly: true},
		{name: "ClientDisconnectBeforeShutdown", workspace: true, remoteClose: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if runtime.GOOS == "windows" && !tc.forwardOnly {
				t.Skip("test command requires a POSIX shell")
			}
			ctx := testutil.Context(t, testutil.WaitLong)
			sink := testutil.NewFakeSink(t)
			reporter := &testConnectionReporter{}
			buildID := uuid.New().String()
			config := &agentssh.Config{ConnectionReporter: reporter}
			wantReason := codersdk.DisconnectReasonServerShutdown
			wantInitiator := codersdk.DisconnectInitiatorAgent
			if tc.workspace {
				wantReason = codersdk.DisconnectReasonWorkspaceStopped
				wantInitiator = codersdk.DisconnectInitiatorServer
				config.ShutdownCause = func() agentssh.ShutdownCause {
					return agentssh.ShutdownCause{Reason: wantReason, BuildID: buildID, BuildReason: "autostop", Transition: "stop"}
				}
			}
			s, err := agentssh.NewServer(ctx, sink.Logger(), prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, config)
			require.NoError(t, err)
			t.Cleanup(func() { _ = s.Close() })
			require.NoError(t, s.UpdateHostSigner(42))
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			clientSessionID := uuid.New().String()
			serveDone := make(chan error, 1)
			go func() { serveDone <- s.Serve(&wrappedListener{Listener: ln, clientSessionID: clientSessionID}) }()
			client := sshClient(t, ln.Addr().String())
			if tc.forwardOnly {
				destination, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				defer destination.Close()
				forward, err := client.Dial("tcp", destination.Addr().String())
				require.NoError(t, err)
				defer forward.Close()
			} else {
				session, err := client.NewSession()
				require.NoError(t, err)
				stdin, err := session.StdinPipe()
				require.NoError(t, err)
				defer stdin.Close()
				stdout := expecter.NewAttachedToSSHSession(t, session)
				command := "echo ready; cat"
				if tc.stdinEOF {
					command = "echo ready; sleep 120"
				}
				require.NoError(t, session.Start(command))
				stdout.ExpectMatch(ctx, "ready")
				if tc.stdinEOF {
					require.NoError(t, stdin.Close())
				}
			}
			connectionLogs := func() []slog.SinkEntry {
				return sink.Entries(func(e slog.SinkEntry) bool { return e.Message == "ssh connection complete" })
			}
			if tc.remoteClose {
				require.NoError(t, client.Close())
				// The known intent must not override a connection that closed first.
				require.Eventually(t, func() bool { return len(connectionLogs()) == 1 }, testutil.WaitLong, testutil.IntervalFast)
			}
			require.NoError(t, s.Close())
			require.Error(t, testutil.RequireReceive(ctx, t, serveDone))
			require.Eventually(t, func() bool { return len(connectionLogs()) == 1 }, testutil.WaitLong, testutil.IntervalFast)
			entries := connectionLogs()
			if !tc.forwardOnly {
				sessionLogs := sink.Entries(func(e slog.SinkEntry) bool { return e.Message == "ssh session closed" })
				require.Len(t, sessionLogs, 1)
				entries = append(entries, sessionLogs...)
				require.Len(t, reporter.disconnects, 1)
				if tc.remoteClose {
					require.NotEqual(t, string(wantReason), reporter.disconnects[0].Reason)
				} else {
					require.Equal(t, string(wantReason), reporter.disconnects[0].Reason)
				}
			}
			for _, entry := range entries {
				fields := make(map[string]any)
				for _, field := range entry.Fields {
					fields[field.Name] = field.Value
				}
				require.Equal(t, clientSessionID, fields["client_session_id"])
				if tc.remoteClose {
					require.NotEqual(t, wantReason, fields["disconnect_reason"])
					require.NotContains(t, fields, "build_id")
					continue
				}
				require.Equal(t, wantReason, fields["disconnect_reason"])
				require.Equal(t, wantInitiator, fields["disconnect_initiator"])
				require.Equal(t, true, fields["disconnect_expected"])
				if tc.workspace {
					require.Equal(t, buildID, fields["build_id"])
					require.Equal(t, "autostop", fields["build_reason"])
					require.Equal(t, "stop", fields["transition"])
				}
			}
		})
	}
}

func TestSSHShutdownAfterTransportClosed(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	sink := testutil.NewFakeSink(t)
	shutdownSampled := make(chan struct{})
	shutdownOnce := sync.OnceFunc(func() { close(shutdownSampled) })
	s, err := agentssh.NewServer(ctx, sink.Logger(), prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, &agentssh.Config{
		ShutdownCause: func() agentssh.ShutdownCause {
			shutdownOnce()
			return agentssh.ShutdownCause{Reason: codersdk.DisconnectReasonWorkspaceStopped, BuildID: uuid.NewString()}
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	require.NoError(t, s.UpdateHostSigner(42))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	closed := make(chan struct{})
	release := make(chan struct{})
	releaseClose := sync.OnceFunc(func() { close(release) })
	defer releaseClose()
	clientSessionID := uuid.NewString()
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- s.Serve(&blockingCloseListener{
			Listener: ln, clientSessionID: clientSessionID, closed: closed, release: release,
		})
	}()
	client := sshClient(t, ln.Addr().String())
	require.NoError(t, client.Close())
	// Keep the closed transport in the server's connection map until the
	// shutdown cause has been sampled, without relying on scheduling delays.
	testutil.TryReceive(ctx, t, closed)
	closeDone := make(chan error, 1)
	go func() { closeDone <- s.Close() }()
	testutil.TryReceive(ctx, t, shutdownSampled)
	releaseClose()
	require.NoError(t, testutil.RequireReceive(ctx, t, closeDone))
	require.Error(t, testutil.RequireReceive(ctx, t, serveDone))
	connectionLogs := func() []slog.SinkEntry {
		return sink.Entries(func(e slog.SinkEntry) bool { return e.Message == "ssh connection complete" })
	}
	require.Eventually(t, func() bool { return len(connectionLogs()) == 1 }, testutil.WaitLong, testutil.IntervalFast)
	entry := connectionLogs()[0]
	require.Contains(t, entry.Fields, slog.F("client_session_id", clientSessionID))
	for _, field := range entry.Fields {
		require.NotEqual(t, "build_id", field.Name)
		require.NotEqual(t, codersdk.DisconnectReasonWorkspaceStopped.SlogField(), field)
	}
}

type blockingCloseListener struct {
	net.Listener
	clientSessionID string
	closed          chan struct{}
	release         <-chan struct{}
}

func (ln *blockingCloseListener) Accept() (net.Conn, error) {
	conn, err := ln.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &blockingCloseConn{
		wrappedConn: wrappedConn{Conn: conn, clientSessionID: ln.clientSessionID},
		closed:      ln.closed,
		release:     ln.release,
	}, nil
}

type blockingCloseConn struct {
	wrappedConn
	once    sync.Once
	closed  chan struct{}
	release <-chan struct{}
}

func (c *blockingCloseConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { close(c.closed) })
	<-c.release
	return err
}

func TestSSHShutdownAfterSessionChannelClosed(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("test command requires a POSIX shell")
	}
	ctx := testutil.Context(t, testutil.WaitLong)
	sink := testutil.NewFakeSink(t)
	reporter := &testConnectionReporter{}
	s, err := agentssh.NewServer(ctx, sink.Logger(), prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, &agentssh.Config{
		ConnectionReporter: reporter,
		ShutdownCause: func() agentssh.ShutdownCause {
			return agentssh.ShutdownCause{Reason: codersdk.DisconnectReasonWorkspaceStopped, BuildID: uuid.NewString()}
		},
	})
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.UpdateHostSigner(42))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serveDone := make(chan error, 1)
	go func() { serveDone <- s.Serve(&wrappedListener{Listener: ln, clientSessionID: uuid.NewString()}) }()
	client := sshClient(t, ln.Addr().String())
	defer client.Close()
	session, err := client.NewSession()
	require.NoError(t, err)
	stdout := expecter.NewAttachedToSSHSession(t, session)
	require.NoError(t, session.Start("echo ready; sleep 120"))
	stdout.ExpectMatch(ctx, "ready")
	require.NoError(t, session.Close())
	require.Error(t, session.Wait())
	// The underlying SSH connection is still healthy after the channel closed.
	_, _, err = client.SendRequest("keepalive@openssh.com", true, nil)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	require.Error(t, testutil.RequireReceive(ctx, t, serveDone))
	require.Len(t, reporter.disconnects, 1)
	require.NotEqual(t, string(codersdk.DisconnectReasonWorkspaceStopped), reporter.disconnects[0].Reason,
		"a session whose channel closed before shutdown must retain its earlier reason")
}
