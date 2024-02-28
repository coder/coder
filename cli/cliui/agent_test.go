package cliui_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func TestAgent(t *testing.T) {
	t.Parallel()

	waitLines := func(t *testing.T, output <-chan string, lines ...string) error {
		t.Helper()

		var got []string
	outerLoop:
		for _, want := range lines {
			for {
				select {
				case line := <-output:
					got = append(got, line)
					if strings.Contains(line, want) {
						continue outerLoop
					}
				case <-time.After(testutil.WaitShort):
					assert.Failf(t, "timed out waiting for line", "want: %q; got: %q", want, got)
					return xerrors.Errorf("timed out waiting for line: %q; got: %q", want, got)
				}
			}
		}
		return nil
	}

	for _, tc := range []struct {
		name    string
		iter    []func(context.Context, *testing.T, *codersdk.WorkspaceAgent, <-chan string, chan []codersdk.WorkspaceAgentLog) error
		logs    chan []codersdk.WorkspaceAgentLog
		opts    cliui.AgentOptions
		want    []string
		wantErr bool
	}{
		{
			name: "Initial connection",
			opts: cliui.AgentOptions{
				FetchInterval: time.Millisecond,
			},
			iter: []func(context.Context, *testing.T, *codersdk.WorkspaceAgent, <-chan string, chan []codersdk.WorkspaceAgentLog) error{
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentConnecting
					return nil
				},
				func(_ context.Context, t *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					return waitLines(t, output, "⧗ Waiting for the workspace agent to connect")
				},
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentConnected
					agent.FirstConnectedAt = ptr.Ref(time.Now())
					return nil
				},
			},
			want: []string{
				"⧗ Waiting for the workspace agent to connect",
				"✔ Waiting for the workspace agent to connect",
				"⧗ Running workspace agent startup scripts (non-blocking)",
				"Notice: The startup scripts are still running and your workspace may be incomplete.",
				"For more information and troubleshooting, see",
			},
		},
		{
			name: "Start timeout",
			opts: cliui.AgentOptions{
				FetchInterval: time.Millisecond,
			},
			iter: []func(context.Context, *testing.T, *codersdk.WorkspaceAgent, <-chan string, chan []codersdk.WorkspaceAgentLog) error{
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentConnecting
					return nil
				},
				func(_ context.Context, t *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					return waitLines(t, output, "⧗ Waiting for the workspace agent to connect")
				},
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentConnected
					agent.LifecycleState = codersdk.WorkspaceAgentLifecycleStartTimeout
					agent.FirstConnectedAt = ptr.Ref(time.Now())
					return nil
				},
			},
			want: []string{
				"⧗ Waiting for the workspace agent to connect",
				"✔ Waiting for the workspace agent to connect",
				"⧗ Running workspace agent startup scripts (non-blocking)",
				"✘ Running workspace agent startup scripts (non-blocking)",
				"Warning: A startup script timed out and your workspace may be incomplete.",
			},
		},
		{
			name: "Initial connection timeout",
			opts: cliui.AgentOptions{
				FetchInterval: 1 * time.Millisecond,
			},
			iter: []func(context.Context, *testing.T, *codersdk.WorkspaceAgent, <-chan string, chan []codersdk.WorkspaceAgentLog) error{
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentConnecting
					agent.LifecycleState = codersdk.WorkspaceAgentLifecycleStarting
					agent.StartedAt = ptr.Ref(time.Now())
					return nil
				},
				func(_ context.Context, t *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					return waitLines(t, output, "⧗ Waiting for the workspace agent to connect")
				},
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentTimeout
					return nil
				},
				func(_ context.Context, t *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					return waitLines(t, output, "The workspace agent is having trouble connecting, wait for it to connect or restart your workspace.")
				},
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentConnected
					agent.FirstConnectedAt = ptr.Ref(time.Now())
					agent.LifecycleState = codersdk.WorkspaceAgentLifecycleReady
					agent.ReadyAt = ptr.Ref(time.Now())
					return nil
				},
			},
			want: []string{
				"⧗ Waiting for the workspace agent to connect",
				"The workspace agent is having trouble connecting, wait for it to connect or restart your workspace.",
				"For more information and troubleshooting, see",
				"✔ Waiting for the workspace agent to connect",
				"⧗ Running workspace agent startup scripts (non-blocking)",
				"✔ Running workspace agent startup scripts (non-blocking)",
			},
		},
		{
			name: "Disconnected",
			opts: cliui.AgentOptions{
				FetchInterval: 1 * time.Millisecond,
			},
			iter: []func(context.Context, *testing.T, *codersdk.WorkspaceAgent, <-chan string, chan []codersdk.WorkspaceAgentLog) error{
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentDisconnected
					agent.FirstConnectedAt = ptr.Ref(time.Now().Add(-1 * time.Minute))
					agent.LastConnectedAt = ptr.Ref(time.Now().Add(-1 * time.Minute))
					agent.DisconnectedAt = ptr.Ref(time.Now())
					agent.LifecycleState = codersdk.WorkspaceAgentLifecycleReady
					agent.StartedAt = ptr.Ref(time.Now().Add(-1 * time.Minute))
					agent.ReadyAt = ptr.Ref(time.Now())
					return nil
				},
				func(_ context.Context, t *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					return waitLines(t, output, "⧗ The workspace agent lost connection")
				},
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentConnected
					agent.DisconnectedAt = nil
					agent.LastConnectedAt = ptr.Ref(time.Now())
					return nil
				},
			},
			want: []string{
				"⧗ The workspace agent lost connection",
				"Wait for it to reconnect or restart your workspace.",
				"For more information and troubleshooting, see",
				"✔ The workspace agent lost connection",
			},
		},
		{
			name: "Startup Logs",
			opts: cliui.AgentOptions{
				FetchInterval: time.Millisecond,
				Wait:          true,
			},
			iter: []func(context.Context, *testing.T, *codersdk.WorkspaceAgent, <-chan string, chan []codersdk.WorkspaceAgentLog) error{
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, logs chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentConnected
					agent.FirstConnectedAt = ptr.Ref(time.Now())
					agent.LifecycleState = codersdk.WorkspaceAgentLifecycleStarting
					agent.StartedAt = ptr.Ref(time.Now())
					agent.LogSources = []codersdk.WorkspaceAgentLogSource{{
						ID:          uuid.Nil,
						DisplayName: "testing",
					}}
					logs <- []codersdk.WorkspaceAgentLog{
						{
							CreatedAt: time.Now(),
							Output:    "Hello world",
							SourceID:  uuid.Nil,
						},
					}
					return nil
				},
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, logs chan []codersdk.WorkspaceAgentLog) error {
					agent.LifecycleState = codersdk.WorkspaceAgentLifecycleReady
					agent.ReadyAt = ptr.Ref(time.Now())
					logs <- []codersdk.WorkspaceAgentLog{
						{
							CreatedAt: time.Now(),
							Output:    "Bye now",
						},
					}
					return nil
				},
			},
			want: []string{
				"⧗ Running workspace agent startup scripts",
				"testing: Hello world",
				"Bye now",
				"✔ Running workspace agent startup scripts",
			},
		},
		{
			name: "Startup script exited with error",
			opts: cliui.AgentOptions{
				FetchInterval: time.Millisecond,
				Wait:          true,
			},
			iter: []func(context.Context, *testing.T, *codersdk.WorkspaceAgent, <-chan string, chan []codersdk.WorkspaceAgentLog) error{
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, logs chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentConnected
					agent.FirstConnectedAt = ptr.Ref(time.Now())
					agent.StartedAt = ptr.Ref(time.Now())
					agent.LifecycleState = codersdk.WorkspaceAgentLifecycleStartError
					agent.ReadyAt = ptr.Ref(time.Now())
					logs <- []codersdk.WorkspaceAgentLog{
						{
							CreatedAt: time.Now(),
							Output:    "Hello world",
						},
					}
					return nil
				},
			},
			want: []string{
				"⧗ Running workspace agent startup scripts",
				"Hello world",
				"✘ Running workspace agent startup scripts",
				"Warning: A startup script exited with an error and your workspace may be incomplete.",
				"For more information and troubleshooting, see",
			},
		},
		{
			name: "Error when shutting down",
			opts: cliui.AgentOptions{
				FetchInterval: time.Millisecond,
			},
			iter: []func(context.Context, *testing.T, *codersdk.WorkspaceAgent, <-chan string, chan []codersdk.WorkspaceAgentLog) error{
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, logs chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentDisconnected
					agent.LifecycleState = codersdk.WorkspaceAgentLifecycleOff
					return nil
				},
			},
			wantErr: true,
		},
		{
			name: "Error when shutting down while waiting",
			opts: cliui.AgentOptions{
				FetchInterval: time.Millisecond,
				Wait:          true,
			},
			iter: []func(context.Context, *testing.T, *codersdk.WorkspaceAgent, <-chan string, chan []codersdk.WorkspaceAgentLog) error{
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, logs chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentConnected
					agent.FirstConnectedAt = ptr.Ref(time.Now())
					agent.LifecycleState = codersdk.WorkspaceAgentLifecycleStarting
					agent.StartedAt = ptr.Ref(time.Now())
					logs <- []codersdk.WorkspaceAgentLog{
						{
							CreatedAt: time.Now(),
							Output:    "Hello world",
						},
					}
					return nil
				},
				func(_ context.Context, t *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					return waitLines(t, output, "Hello world")
				},
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.ReadyAt = ptr.Ref(time.Now())
					agent.LifecycleState = codersdk.WorkspaceAgentLifecycleShuttingDown
					return nil
				},
			},
			want: []string{
				"⧗ Running workspace agent startup scripts",
				"Hello world",
				"✔ Running workspace agent startup scripts",
			},
			wantErr: true,
		},
		{
			name: "Error during fetch",
			opts: cliui.AgentOptions{
				FetchInterval: time.Millisecond,
				Wait:          true,
			},
			iter: []func(context.Context, *testing.T, *codersdk.WorkspaceAgent, <-chan string, chan []codersdk.WorkspaceAgentLog) error{
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentConnecting
					return nil
				},
				func(_ context.Context, t *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					return waitLines(t, output, "⧗ Waiting for the workspace agent to connect")
				},
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					return xerrors.New("bad")
				},
			},
			want: []string{
				"⧗ Waiting for the workspace agent to connect",
			},
			wantErr: true,
		},
		{
			name: "Shows agent troubleshooting URL",
			opts: cliui.AgentOptions{
				FetchInterval: time.Millisecond,
				Wait:          true,
			},
			iter: []func(context.Context, *testing.T, *codersdk.WorkspaceAgent, <-chan string, chan []codersdk.WorkspaceAgentLog) error{
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, _ <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					agent.Status = codersdk.WorkspaceAgentTimeout
					agent.TroubleshootingURL = "https://troubleshoot"
					return nil
				},
				func(_ context.Context, t *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					return waitLines(t, output, "The workspace agent is having trouble connecting, wait for it to connect or restart your workspace.")
				},
				func(_ context.Context, _ *testing.T, agent *codersdk.WorkspaceAgent, output <-chan string, _ chan []codersdk.WorkspaceAgentLog) error {
					return xerrors.New("bad")
				},
			},
			want: []string{
				"⧗ Waiting for the workspace agent to connect",
				"The workspace agent is having trouble connecting, wait for it to connect or restart your workspace.",
				"https://troubleshoot",
			},
			wantErr: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			r, w, err := os.Pipe()
			require.NoError(t, err, "create pipe failed")
			defer r.Close()
			defer w.Close()

			agent := codersdk.WorkspaceAgent{
				ID:             uuid.New(),
				Status:         codersdk.WorkspaceAgentConnecting,
				CreatedAt:      time.Now(),
				LifecycleState: codersdk.WorkspaceAgentLifecycleCreated,
			}
			output := make(chan string, 100) // Buffered to avoid blocking, overflow is discarded.
			logs := make(chan []codersdk.WorkspaceAgentLog, 1)

			cmd := &clibase.Cmd{
				Handler: func(inv *clibase.Invocation) error {
					tc.opts.Fetch = func(_ context.Context, _ uuid.UUID) (codersdk.WorkspaceAgent, error) {
						t.Log("iter", len(tc.iter))
						var err error
						if len(tc.iter) > 0 {
							err = tc.iter[0](ctx, t, &agent, output, logs)
							tc.iter = tc.iter[1:]
						}
						return agent, err
					}
					tc.opts.FetchLogs = func(ctx context.Context, _ uuid.UUID, _ int64, follow bool) (<-chan []codersdk.WorkspaceAgentLog, io.Closer, error) {
						if follow {
							return logs, closeFunc(func() error { return nil }), nil
						}

						fetchLogs := make(chan []codersdk.WorkspaceAgentLog, 1)
						select {
						case <-ctx.Done():
							return nil, nil, ctx.Err()
						case l := <-logs:
							fetchLogs <- l
						default:
						}
						close(fetchLogs)
						return fetchLogs, closeFunc(func() error { return nil }), nil
					}
					err := cliui.Agent(inv.Context(), w, uuid.Nil, tc.opts)
					_ = w.Close()
					return err
				},
			}
			inv := cmd.Invoke()

			waiter := clitest.StartWithWaiter(t, inv)

			s := bufio.NewScanner(r)
			for s.Scan() {
				line := s.Text()
				t.Log(line)
				select {
				case output <- line:
				default:
					t.Logf("output overflow: %s", line)
				}
				if len(tc.want) == 0 {
					require.Fail(t, "unexpected line", line)
				}
				require.Contains(t, line, tc.want[0])
				tc.want = tc.want[1:]
			}
			require.NoError(t, s.Err())
			if len(tc.want) > 0 {
				require.Fail(t, "missing lines: "+strings.Join(tc.want, ", "))
			}

			if tc.wantErr {
				waiter.RequireError()
			} else {
				waiter.RequireSuccess()
			}
		})
	}

	t.Run("NotInfinite", func(t *testing.T) {
		t.Parallel()
		var fetchCalled uint64

		cmd := &clibase.Cmd{
			Handler: func(inv *clibase.Invocation) error {
				buf := bytes.Buffer{}
				err := cliui.Agent(inv.Context(), &buf, uuid.Nil, cliui.AgentOptions{
					FetchInterval: 10 * time.Millisecond,
					Fetch: func(ctx context.Context, agentID uuid.UUID) (codersdk.WorkspaceAgent, error) {
						atomic.AddUint64(&fetchCalled, 1)

						return codersdk.WorkspaceAgent{
							Status:         codersdk.WorkspaceAgentConnected,
							LifecycleState: codersdk.WorkspaceAgentLifecycleReady,
						}, nil
					},
				})
				if err != nil {
					return err
				}

				require.Never(t, func() bool {
					called := atomic.LoadUint64(&fetchCalled)
					return called > 5 || called == 0
				}, time.Second, 100*time.Millisecond)

				return nil
			},
		}
		require.NoError(t, cmd.Invoke().Run())
	})
}

func TestPeerDiagnostics(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		diags tailnet.PeerDiagnostics
		want  []*regexp.Regexp // must be ordered, can omit lines
	}{
		{
			name: "noPreferredDERP",
			diags: tailnet.PeerDiagnostics{
				PreferredDERP:          0,
				DERPRegionNames:        make(map[int]string),
				SentNode:               true,
				ReceivedNode:           &tailcfg.Node{DERP: "127.3.3.40:999"},
				LastWireguardHandshake: time.Now(),
			},
			want: []*regexp.Regexp{
				regexp.MustCompile("^✘ not connected to DERP$"),
			},
		},
		{
			name: "preferredDERP",
			diags: tailnet.PeerDiagnostics{
				PreferredDERP: 23,
				DERPRegionNames: map[int]string{
					23: "testo",
				},
				SentNode:               true,
				ReceivedNode:           &tailcfg.Node{DERP: "127.3.3.40:999"},
				LastWireguardHandshake: time.Now(),
			},
			want: []*regexp.Regexp{
				regexp.MustCompile(`^✔ preferred DERP region: 23 \(testo\)$`),
			},
		},
		{
			name: "sentNode",
			diags: tailnet.PeerDiagnostics{
				PreferredDERP:          0,
				DERPRegionNames:        map[int]string{},
				SentNode:               true,
				ReceivedNode:           &tailcfg.Node{DERP: "127.3.3.40:999"},
				LastWireguardHandshake: time.Time{},
			},
			want: []*regexp.Regexp{
				regexp.MustCompile(`^✔ sent local data to Coder networking coodinator$`),
			},
		},
		{
			name: "didntSendNode",
			diags: tailnet.PeerDiagnostics{
				PreferredDERP:          0,
				DERPRegionNames:        map[int]string{},
				SentNode:               false,
				ReceivedNode:           &tailcfg.Node{DERP: "127.3.3.40:999"},
				LastWireguardHandshake: time.Time{},
			},
			want: []*regexp.Regexp{
				regexp.MustCompile(`^✘ have not sent local data to Coder networking coordinator$`),
			},
		},
		{
			name: "receivedNodeDERPOKNoEndpoints",
			diags: tailnet.PeerDiagnostics{
				PreferredDERP:          0,
				DERPRegionNames:        map[int]string{999: "Embedded"},
				SentNode:               true,
				ReceivedNode:           &tailcfg.Node{DERP: "127.3.3.40:999"},
				LastWireguardHandshake: time.Time{},
			},
			want: []*regexp.Regexp{
				regexp.MustCompile(`^✔ received remote agent data from Coder networking coordinator$`),
				regexp.MustCompile(`preferred DERP region: 999 \(Embedded\)$`),
				regexp.MustCompile(`endpoints: $`),
			},
		},
		{
			name: "receivedNodeDERPUnknownNoEndpoints",
			diags: tailnet.PeerDiagnostics{
				PreferredDERP:          0,
				DERPRegionNames:        map[int]string{},
				SentNode:               true,
				ReceivedNode:           &tailcfg.Node{DERP: "127.3.3.40:999"},
				LastWireguardHandshake: time.Time{},
			},
			want: []*regexp.Regexp{
				regexp.MustCompile(`^✔ received remote agent data from Coder networking coordinator$`),
				regexp.MustCompile(`preferred DERP region: 999 \(unknown\)$`),
				regexp.MustCompile(`endpoints: $`),
			},
		},
		{
			name: "receivedNodeEndpointsNoDERP",
			diags: tailnet.PeerDiagnostics{
				PreferredDERP:          0,
				DERPRegionNames:        map[int]string{999: "Embedded"},
				SentNode:               true,
				ReceivedNode:           &tailcfg.Node{Endpoints: []string{"99.88.77.66:4555", "33.22.11.0:3444"}},
				LastWireguardHandshake: time.Time{},
			},
			want: []*regexp.Regexp{
				regexp.MustCompile(`^✔ received remote agent data from Coder networking coordinator$`),
				regexp.MustCompile(`preferred DERP region:\s*$`),
				regexp.MustCompile(`endpoints: 99\.88\.77\.66:4555, 33\.22\.11\.0:3444$`),
			},
		},
		{
			name: "didntReceiveNode",
			diags: tailnet.PeerDiagnostics{
				PreferredDERP:          0,
				DERPRegionNames:        map[int]string{},
				SentNode:               false,
				ReceivedNode:           nil,
				LastWireguardHandshake: time.Time{},
			},
			want: []*regexp.Regexp{
				regexp.MustCompile(`^✘ have not received remote agent data from Coder networking coordinator$`),
			},
		},
		{
			name: "noWireguardHandshake",
			diags: tailnet.PeerDiagnostics{
				PreferredDERP:          0,
				DERPRegionNames:        map[int]string{},
				SentNode:               false,
				ReceivedNode:           nil,
				LastWireguardHandshake: time.Time{},
			},
			want: []*regexp.Regexp{
				regexp.MustCompile(`^✘ Wireguard is not connected$`),
			},
		},
		{
			name: "wireguardHandshakeRecent",
			diags: tailnet.PeerDiagnostics{
				PreferredDERP:          0,
				DERPRegionNames:        map[int]string{},
				SentNode:               false,
				ReceivedNode:           nil,
				LastWireguardHandshake: time.Now().Add(-5 * time.Second),
			},
			want: []*regexp.Regexp{
				regexp.MustCompile(`^✔ Wireguard handshake \d+s ago$`),
			},
		},
		{
			name: "wireguardHandshakeOld",
			diags: tailnet.PeerDiagnostics{
				PreferredDERP:          0,
				DERPRegionNames:        map[int]string{},
				SentNode:               false,
				ReceivedNode:           nil,
				LastWireguardHandshake: time.Now().Add(-450 * time.Second), // 7m30s
			},
			want: []*regexp.Regexp{
				regexp.MustCompile(`^⚠ Wireguard handshake 7m\d+s ago$`),
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, w := io.Pipe()
			go func() {
				defer w.Close()
				cliui.PeerDiagnostics(w, tc.diags)
			}()
			s := bufio.NewScanner(r)
			i := 0
			got := make([]string, 0)
			for s.Scan() {
				got = append(got, s.Text())
				if i < len(tc.want) {
					reg := tc.want[i]
					if reg.Match(s.Bytes()) {
						i++
					}
				}
			}
			if i < len(tc.want) {
				t.Logf("failed to match regexp: %s\ngot:\n%s", tc.want[i].String(), strings.Join(got, "\n"))
				t.FailNow()
			}
		})
	}
}
