package agentssh_test

import (
	"net"
	"runtime"
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
		forwardOnly bool
		remoteClose bool
		stdinEOF    bool
	}{
		{name: "AgentShutdown"},
		{name: "ServerShutdownAfterStdinEOF", stdinEOF: true},
		{name: "ForwardOnly", forwardOnly: true},
		{name: "ClientDisconnectBeforeShutdown", remoteClose: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if runtime.GOOS == "windows" && !tc.forwardOnly {
				t.Skip("test command requires a POSIX shell")
			}
			ctx := testutil.Context(t, testutil.WaitLong)
			sink := testutil.NewFakeSink(t)
			reporter := &testConnectionReporter{}
			config := &agentssh.Config{ConnectionReporter: reporter}
			wantReason := codersdk.DisconnectReasonServerShutdown
			wantInitiator := codersdk.DisconnectInitiatorAgent

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
					// Keep the command in one process so shutdown cannot race a fork.
					command = "echo ready; exec sleep 120"
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
				// The shutdown must not override a connection and session that
				// finished closing first.
				require.Eventually(t, func() bool {
					sessionLogs := sink.Entries(func(e slog.SinkEntry) bool { return e.Message == "ssh session closed" })
					return len(connectionLogs()) == 1 && len(sessionLogs) == 1
				}, testutil.WaitLong, testutil.IntervalFast)
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
			}
		})
	}
}
