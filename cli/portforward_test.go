package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pion/udp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestPortForward(t *testing.T) {
	t.Parallel()

	t.Run("None", func(t *testing.T) {
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		cmd, root := clitest.New(t, "port-forward", "blah")
		clitest.SetupConfig(t, client, root)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)

		err := cmd.Execute()
		require.Error(t, err)
		require.ErrorContains(t, err, "no port-forwards")

		// Check that the help was printed.
		require.Contains(t, buf.String(), "port-forward <workspace>")
	})

	cases := []struct {
		name    string
		network string
		// The flag to pass to `coder port-forward X` to port-forward this type
		// of connection. Has two format args (both strings), the first is the
		// local address and the second is the remote address.
		flag string
		// setupRemote creates a "remote" listener to emulate a service in the
		// workspace.
		setupRemote func(t *testing.T) net.Listener
		// setupLocal returns an available port or Unix socket path that the
		// port-forward command will listen on "locally". Returns the address
		// you pass to net.Dial, and the port/path you pass to `coder
		// port-forward`.
		setupLocal func(t *testing.T) (string, string)
	}{
		{
			name:    "TCP",
			network: "tcp",
			flag:    "--tcp=%v:%v",
			setupRemote: func(t *testing.T) net.Listener {
				l, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err, "create TCP listener")
				return l
			},
			setupLocal: func(t *testing.T) (string, string) {
				l, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err, "create TCP listener to generate random port")
				defer l.Close()

				_, port, err := net.SplitHostPort(l.Addr().String())
				require.NoErrorf(t, err, "split TCP address %q", l.Addr().String())
				return l.Addr().String(), port
			},
		},
		{
			name:    "UDP",
			network: "udp",
			flag:    "--udp=%v:%v",
			setupRemote: func(t *testing.T) net.Listener {
				addr := net.UDPAddr{
					IP:   net.ParseIP("127.0.0.1"),
					Port: 0,
				}
				l, err := udp.Listen("udp", &addr)
				require.NoError(t, err, "create UDP listener")
				return l
			},
			setupLocal: func(t *testing.T) (string, string) {
				addr := net.UDPAddr{
					IP:   net.ParseIP("127.0.0.1"),
					Port: 0,
				}
				l, err := udp.Listen("udp", &addr)
				require.NoError(t, err, "create UDP listener to generate random port")
				defer l.Close()

				_, port, err := net.SplitHostPort(l.Addr().String())
				require.NoErrorf(t, err, "split UDP address %q", l.Addr().String())
				return l.Addr().String(), port
			},
		},
		{
			name:    "Unix",
			network: "unix",
			flag:    "--unix=%v:%v",
			setupRemote: func(t *testing.T) net.Listener {
				if runtime.GOOS == "windows" {
					t.Skip("Unix socket forwarding isn't supported on Windows")
				}

				tmpDir, err := os.MkdirTemp("", "coderd_agent_test_")
				require.NoError(t, err, "create temp dir for unix listener")
				t.Cleanup(func() {
					_ = os.RemoveAll(tmpDir)
				})

				l, err := net.Listen("unix", filepath.Join(tmpDir, "test.sock"))
				require.NoError(t, err, "create UDP listener")
				return l
			},
			setupLocal: func(t *testing.T) (string, string) {
				tmpDir, err := os.MkdirTemp("", "coderd_agent_test_")
				require.NoError(t, err, "create temp dir for unix listener")
				t.Cleanup(func() {
					_ = os.RemoveAll(tmpDir)
				})

				path := filepath.Join(tmpDir, "test.sock")
				return path, path
			},
		},
	}

	for _, c := range cases {
		if c.name != "Unix" {
			continue
		}
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			t.Run("One", func(t *testing.T) {
				t.Parallel()
				var (
					client       = coderdtest.New(t, nil)
					user         = coderdtest.CreateFirstUser(t, client)
					_, workspace = runAgent(t, client, user.UserID)
					l1, p1       = setupTestListener(t, c.setupRemote(t))
				)
				t.Cleanup(func() {
					_ = l1.Close()
				})

				// Create a flag that forwards from local to listener 1.
				localAddress, localFlag := c.setupLocal(t)
				flag := fmt.Sprintf(c.flag, localFlag, p1)

				// Launch port-forward in a goroutine so we can start dialing
				// the "local" listener.
				cmd, root := clitest.New(t, "port-forward", workspace.Name, flag)
				clitest.SetupConfig(t, client, root)
				buf := new(bytes.Buffer)
				cmd.SetOut(io.MultiWriter(buf, os.Stderr))
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go func() {
					err := cmd.ExecuteContext(ctx)
					require.Error(t, err)
					require.ErrorIs(t, err, context.Canceled)
				}()
				waitForPortForwardReady(t, buf)

				// Open two connections simultaneously and test them out of
				// sync.
				d := net.Dialer{Timeout: 3 * time.Second}
				c1, err := d.DialContext(ctx, c.network, localAddress)
				require.NoError(t, err, "open connection 1 to 'local' listener")
				defer c1.Close()
				c2, err := d.DialContext(ctx, c.network, localAddress)
				require.NoError(t, err, "open connection 2 to 'local' listener")
				defer c2.Close()
				testDial(t, c2)
				testDial(t, c1)
			})
		})
	}
}

// runAgent creates a fake workspace and starts an agent locally for that
// workspace. The agent will be cleaned up on test completion.
func runAgent(t *testing.T, client *codersdk.Client, userID uuid.UUID) ([]codersdk.WorkspaceResource, codersdk.Workspace) {
	ctx := context.Background()
	user, err := client.User(ctx, userID)
	require.NoError(t, err, "specified user does not exist")
	require.Greater(t, len(user.OrganizationIDs), 0, "user has no organizations")
	orgID := user.OrganizationIDs[0]

	// Setup echo provisioner
	agentToken := uuid.NewString()
	coderdtest.NewProvisionerDaemon(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, orgID, &echo.Responses{
		Parse:           echo.ParseComplete,
		ProvisionDryRun: echo.ProvisionComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "somename",
						Type: "someinstance",
						Agents: []*proto.Agent{{
							Auth: &proto.Agent_Token{
								Token: agentToken,
							},
						}},
					}},
				},
			},
		}},
	})

	// Create template and workspace
	template := coderdtest.CreateTemplate(t, client, orgID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, orgID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	// Start workspace agent in a goroutine
	cmd, root := clitest.New(t, "agent", "--agent-token", agentToken, "--agent-url", client.URL.String())
	agentClient := &*client
	clitest.SetupConfig(t, agentClient, root)
	agentCtx, agentCancel := context.WithCancel(ctx)
	t.Cleanup(agentCancel)
	go func() {
		err := cmd.ExecuteContext(agentCtx)
		require.NoError(t, err)
	}()

	coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
	resources, err := client.WorkspaceResourcesByBuild(context.Background(), workspace.LatestBuild.ID)
	require.NoError(t, err)

	return resources, workspace
}

// setupTestListener starts accepting connections and echoing a single packet.
// Returns the listener and the listen port or Unix path.
func setupTestListener(t *testing.T, l net.Listener) (net.Listener, string) {
	t.Cleanup(func() {
		_ = l.Close()
	})
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}

			go testAccept(t, c)
		}
	}()

	addr := l.Addr().String()
	if !strings.HasPrefix(l.Addr().Network(), "unix") {
		_, port, err := net.SplitHostPort(addr)
		require.NoErrorf(t, err, "split non-Unix listen path %q", addr)
		addr = port
	}

	return l, addr
}

var dialTestPayload = []byte("dean-was-here123")

func testDial(t *testing.T, c net.Conn) {
	t.Helper()

	assertWritePayload(t, c, dialTestPayload)
	assertReadPayload(t, c, dialTestPayload)
}

func testAccept(t *testing.T, c net.Conn) {
	t.Helper()
	defer c.Close()

	assertReadPayload(t, c, dialTestPayload)
	assertWritePayload(t, c, dialTestPayload)
}

func assertReadPayload(t *testing.T, r io.Reader, payload []byte) {
	b := make([]byte, len(payload)+16)
	n, err := r.Read(b)
	require.NoError(t, err, "read payload")
	require.Equal(t, len(payload), n, "read payload length does not match")
	require.Equal(t, payload, b[:n])
}

func assertWritePayload(t *testing.T, w io.Writer, payload []byte) {
	n, err := w.Write(payload)
	require.NoError(t, err, "write payload")
	require.Equal(t, len(payload), n, "payload length does not match")
}

func waitForPortForwardReady(t *testing.T, output *bytes.Buffer) {
	for i := 0; i < 100; i++ {
		time.Sleep(250 * time.Millisecond)

		data := output.String()
		if strings.Contains(data, "Ready!") {
			return
		}
	}

	t.Fatal("port-forward command did not become ready in time")
}
