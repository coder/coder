package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAgent_TCPUpgrade_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		port    int
		portStr string
		upgrade string
		error   string
		status  int
	}{
		{
			name:   "MissingPort",
			status: http.StatusNotFound,
		},
		{
			name:    "InvalidPort",
			portStr: "invalid",
			status:  http.StatusNotFound,
			error:   "Invalid port",
		},
		{
			name:   "UnsupportedPort",
			port:   9999,
			status: http.StatusNotFound,
			error:  "Unsupported port",
		},
		{
			name:   "MissingUpgrade",
			port:   workspacesdk.AgentStandardSSHPort,
			status: http.StatusBadRequest,
			error:  "Missing upgrade header",
		},
		{
			name:    "InvalidUpgrade",
			upgrade: "invalid",
			port:    workspacesdk.AgentStandardSSHPort,
			status:  http.StatusBadRequest,
			error:   "Invalid upgrade header",
		},
	}

	//nolint:dogsled
	agentConn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
	client := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext:       agentConn.DialContext,
		},
	}
	t.Cleanup(client.CloseIdleConnections)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			port := tc.portStr
			if port == "" && tc.port != 0 {
				port = strconv.Itoa(tc.port)
			}
			url := fmt.Sprintf("http://127.0.0.1:%d/api/v0/tcp/%s",
				workspacesdk.AgentHTTPAPIServerPort, port)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			require.NoError(t, err)
			if tc.upgrade != "" {
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", tc.upgrade)
			}

			require.True(t, agentConn.AwaitReachable(ctx))
			res, err := client.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, tc.status, res.StatusCode)
			if tc.error != "" {
				var apiErr codersdk.Response
				require.NoError(t, json.NewDecoder(res.Body).Decode(&apiErr))
				require.Contains(t, apiErr.Message, tc.error)
			}
		})
	}
}

func TestAgent_SSHUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// Optional client session ID to pass in or "invalid".
		id string
		// Expected disconnect reason (defaults to graceful).
		reason codersdk.DisconnectReason
		// Expected bytes for the agent to receive from the client.
		rxBytes int64
		// Expected exit code.
		code int
		// Expected session error, if any.
		error string
		// Optional string to write in addition to "echo test\n".
		write string
		// Closes the agent instead of waiting for the session to exit.
		shutdown bool
	}{
		{
			name:    "EmptyClientSessionID",
			write:   "exit 0\n",
			rxBytes: 17,
		},
		{
			name:    "InvalidClientSessionID",
			id:      "invalid",
			write:   "exit 0\n",
			rxBytes: 17,
		},
		{
			name:    "ValidClientSessionID",
			id:      "0123456789abcdef0123456789abcdef",
			write:   "exit 0\n",
			rxBytes: 17,
		},
		{
			name:     "AgentShutdown",
			id:       "0123456789abcdef0123456789abcdef",
			reason:   codersdk.DisconnectReasonServerShutdown,
			write:    "echo test\r",
			shutdown: true,
			rxBytes:  10,
			// The agent closes the shell's stdin before it signals the shell, so the
			// shell can exit 0 first.
			code: wantIgnoreCode,
		},
		{
			name:    "UnexpectedDisconnect",
			reason:  "process exited with error status: 1",
			error:   "process exited with error status: 1",
			write:   "exit 1\n",
			rxBytes: wantSomeBytes,
			code:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			sink := testutil.NewFakeSink(t)
			logger := slog.Make(sink).Leveled(slog.LevelInfo).FlightRecorder(1)
			logger.Debug(ctx, "making sure there is at least one debug log")
			conn, client, stats, _, agnt := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, o *agent.Options) {
				o.Logger = logger
			})
			require.True(t, conn.AwaitReachable(ctx))

			if tc.id != "" {
				conn.SetExtraHeaders(http.Header{
					"baggage": []string{tracing.SessionIDBaggageKey + "=" + tc.id},
				})
			}

			sshClient, err := conn.SSHClient(ctx)
			require.NoError(t, err)
			defer sshClient.Close()
			session, err := sshClient.NewSession()
			require.NoError(t, err)
			defer session.Close()
			stdin, err := session.StdinPipe()
			require.NoError(t, err)
			output := make(chan struct{})
			session.Stdout = &firstWriteNotifier{notify: output}
			err = session.Shell()
			require.NoError(t, err)

			// Generate SSH traffic so the connstats window sees the session and to
			// get a non-zero value for transferred bytes.
			_, err = stdin.Write([]byte("echo test\n"))
			require.NoError(t, err)
			// Output means the agent has sent bytes, so a shutdown cannot report
			// a session that transmitted nothing.
			testutil.TryReceive(ctx, t, output)

			assertSSHStats(t, stats)

			if tc.write != "" {
				_, err = stdin.Write([]byte(tc.write))
				require.NoError(t, err, "writing %q to stdin", tc.write)
			}

			if tc.shutdown {
				// The client's end of the tunnel may not notice the shutdown, so only
				// the agent's report is checked.
				require.NoError(t, agnt.Close())
			} else {
				_ = stdin.Close()
				err = session.Wait()
				if tc.error == "" {
					require.NoError(t, err, "waiting for session to exit")
				} else {
					require.Error(t, err, tc.error)
				}
			}

			reason := codersdk.DisconnectReasonGraceful
			if tc.reason != "" {
				reason = tc.reason
			}
			expectID := tc.id
			if expectID == "invalid" {
				expectID = ""
			}

			assertConnectionReport(t, client,
				proto.ConnectEvent{
					Method:          proto.Connection_METHOD_SSH,
					ClientSessionID: expectID,
				},
				proto.DisconnectEvent{
					Code:    tc.code,
					Reason:  string(reason),
					RxBytes: tc.rxBytes,
					TxBytes: wantSomeBytes, // echo output + prompts
				},
			)

			requireDisconnectLog(t, sink, "ssh session closed", expectID, reason)
			if tc.shutdown {
				// Only a shutdown adds a disconnect reason to the connection log.
				requireDisconnectLog(t, sink, "ssh connection complete", expectID, reason)
			}

			requireDebugFlushed(t, sink, reason)
		})
	}
}

func TestAgent_ReconnectingPTYUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// Optional client session ID to pass in or "invalid".
		id string
		// Expected disconnect reason (defaults to graceful).
		reason codersdk.DisconnectReason
		// Expected exit code.
		code int
		// Optional string to write.
		write string
		// Shell to execute (defaults to bash).
		shell string
		// Closes the agent instead of waiting for the session to exit.
		shutdown bool
	}{
		{
			name:  "EmptyClientSessionID",
			write: "echo test\rexit 0\r",
		},
		{
			name:  "InvalidClientSessionID",
			id:    "invalid",
			write: "echo test\rexit 0\r",
		},
		{
			name:  "ValidClientSessionID",
			id:    "0123456789abcdef0123456789abcdef",
			write: "echo test\rexit 0\r",
		},
		{
			name:     "AgentShutdown",
			id:       "0123456789abcdef0123456789abcdef",
			reason:   codersdk.DisconnectReasonServerShutdown,
			write:    "echo test\r",
			shutdown: true,
			code:     1,
		},
		{
			name:   "UnexpectedDisconnect",
			reason: codersdk.DisconnectReasonNetworkError,
			shell:  `#!"`, // Malformed shebang to cause an error.
			code:   1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			sink := testutil.NewFakeSink(t)
			logger := slog.Make(sink).Leveled(slog.LevelInfo).FlightRecorder(1)
			logger.Debug(ctx, "making sure there is at least one debug log")
			//nolint:dogsled
			conn, client, _, _, agnt := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, o *agent.Options) {
				o.Logger = logger
			})
			require.True(t, conn.AwaitReachable(ctx))

			if tc.id != "" {
				conn.SetExtraHeaders(http.Header{
					"baggage": []string{tracing.SessionIDBaggageKey + "=" + tc.id},
				})
			}

			id := uuid.New()
			command := "bash"
			if tc.shell != "" {
				command = tc.shell
			}
			ptyConn, err := conn.ReconnectingPTY(ctx, id, 128, 128, command)
			require.NoError(t, err)
			defer ptyConn.Close()

			initData, err := json.Marshal(workspacesdk.AgentReconnectingPTYInit{
				ID:      id,
				Height:  128,
				Width:   128,
				Command: "bash",
			})
			require.NoError(t, err)

			rxBytes := int64(len(initData) + 2)
			var txBytes int64

			type copyResult struct {
				written int64
				err     error
			}

			// write writes the provided input and returns the actual bytes that were
			// written, a channel that closes on the first write, and a channel that
			// returns the total number of bytes written along with any error.
			write := func(input string) ([]byte, chan copyResult, chan struct{}) {
				copied := make(chan copyResult, 1)
				output := make(chan struct{})
				go func() {
					written, err := io.Copy(&firstWriteNotifier{notify: output}, ptyConn)
					copied <- copyResult{written: written, err: err}
				}()

				data, err := json.Marshal(workspacesdk.ReconnectingPTYRequest{
					Data: input,
				})
				require.NoError(t, err)
				_, err = ptyConn.Write(data)
				require.NoError(t, err)

				return data, copied, output
			}

			if tc.write != "" {
				data, copied, output := write(tc.write)
				if tc.shutdown {
					// Output means the PTY is attached, so the shutdown closes a live
					// connection rather than one still being set up.
					testutil.TryReceive(ctx, t, output)
					require.NoError(t, agnt.Close())
					// The agent may not have read all the input before the shutdown,
					// and the client may not notice the close, so any bytes will do.
					rxBytes = wantSomeBytes
					txBytes = wantSomeBytes
				} else {
					// Should get at least the commands, resulting output, and newlines,
					// but there will be more because of the prompts.
					result := testutil.TryReceive(ctx, t, copied)
					require.NoError(t, result.err)
					rxBytes += int64(len(data))
					txBytes = result.written
					require.Greater(t, txBytes, int64(20))
				}
			}

			reason := codersdk.DisconnectReasonGraceful
			if tc.reason != "" {
				reason = tc.reason
			}
			expectID := tc.id
			if expectID == "invalid" {
				expectID = ""
			}

			assertConnectionReport(t, client,
				proto.ConnectEvent{
					Method:          proto.Connection_METHOD_RECONNECTING_PTY,
					ClientSessionID: expectID,
				},
				proto.DisconnectEvent{
					Code:    tc.code,
					Reason:  string(reason),
					RxBytes: rxBytes,
					TxBytes: txBytes,
				},
			)
			requireDisconnectLog(t, sink, "reconnecting pty closed", expectID, reason)
			requireDebugFlushed(t, sink, reason)
		})
	}
}

// firstWriteNotifier discards writes and closes notify on the first one.
type firstWriteNotifier struct {
	notify chan struct{}
	once   sync.Once
}

func (w *firstWriteNotifier) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.notify) })
	return len(p), nil
}

// requireDisconnectLog requires exactly one agent log line with the given
// message, carrying the client session ID and disconnect reason. Shutdowns
// must also name the agent as the initiator and mark the disconnect expected.
func requireDisconnectLog(t *testing.T, sink *testutil.FakeSink, message, clientSessionID string, reason codersdk.DisconnectReason) {
	t.Helper()
	var entries []slog.SinkEntry
	require.Eventually(t, func() bool {
		entries = sink.Entries(func(e slog.SinkEntry) bool { return e.Message == message })
		return len(entries) > 0
	}, testutil.WaitLong, testutil.IntervalFast)
	require.Len(t, entries, 1)
	fields := make(map[string]any)
	for _, field := range entries[0].Fields {
		fields[field.Name] = field.Value
	}
	require.Equal(t, clientSessionID, fields["client_session_id"])
	require.Equal(t, reason, fields["disconnect_reason"])
	if reason == codersdk.DisconnectReasonServerShutdown {
		require.Equal(t, codersdk.DisconnectInitiatorAgent, fields["disconnect_initiator"])
		require.Equal(t, true, fields["disconnect_expected"])
	}
}

// requireDebugFlushed asserts whether the logger should have flushed.
func requireDebugFlushed(t *testing.T, sink *testutil.FakeSink, reason codersdk.DisconnectReason) {
	// Debug logs should flush based on whether the disconnect was expected.
	debugEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelDebug })
	if !reason.Expected() {
		require.NotEmpty(t, debugEntries)
	} else {
		require.Empty(t, debugEntries)
	}
}
