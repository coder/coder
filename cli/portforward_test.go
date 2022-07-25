package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pion/udp"
	"github.com/stretchr/testify/assert"
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
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		cmd, root := clitest.New(t, "port-forward", "blah")
		clitest.SetupConfig(t, client, root)
		buf := newThreadSafeBuffer()
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

				tmpDir := t.TempDir()
				l, err := net.Listen("unix", filepath.Join(tmpDir, "test.sock"))
				require.NoError(t, err, "create UDP listener")
				return l
			},
			setupLocal: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				path := filepath.Join(tmpDir, "test.sock")
				return path, path
			},
		},
	}

	// Setup agent once to be shared between test-cases (avoid expensive
	// non-parallel setup).
	var (
		client       = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user         = coderdtest.CreateFirstUser(t, client)
		_, workspace = runAgent(t, client, user.UserID)
	)

	for _, c := range cases { //nolint:paralleltest // the `c := c` confuses the linter
		c := c
		// Delay parallel tests here because setupLocal reserves
		// a free open port which is not guaranteed to be free
		// between the listener closing and port-forward ready.
		t.Run(c.name, func(t *testing.T) {
			t.Run("OnePort", func(t *testing.T) {
				p1 := setupTestListener(t, c.setupRemote(t))

				// Create a flag that forwards from local to listener 1.
				localAddress, localFlag := c.setupLocal(t)
				flag := fmt.Sprintf(c.flag, localFlag, p1)

				// Launch port-forward in a goroutine so we can start dialing
				// the "local" listener.
				cmd, root := clitest.New(t, "port-forward", workspace.Name, flag)
				clitest.SetupConfig(t, client, root)
				buf := newThreadSafeBuffer()
				cmd.SetOut(buf)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				errC := make(chan error)
				go func() {
					errC <- cmd.ExecuteContext(ctx)
				}()
				waitForPortForwardReady(t, buf)

				t.Parallel() // Port is reserved, enable parallel execution.

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

				cancel()
				err = <-errC
				require.ErrorIs(t, err, context.Canceled)
			})

			//nolint:paralleltest
			t.Run("TwoPorts", func(t *testing.T) {
				var (
					p1 = setupTestListener(t, c.setupRemote(t))
					p2 = setupTestListener(t, c.setupRemote(t))
				)

				// Create a flags for listener 1 and listener 2.
				localAddress1, localFlag1 := c.setupLocal(t)
				localAddress2, localFlag2 := c.setupLocal(t)
				flag1 := fmt.Sprintf(c.flag, localFlag1, p1)
				flag2 := fmt.Sprintf(c.flag, localFlag2, p2)

				// Launch port-forward in a goroutine so we can start dialing
				// the "local" listeners.
				cmd, root := clitest.New(t, "port-forward", workspace.Name, flag1, flag2)
				clitest.SetupConfig(t, client, root)
				buf := newThreadSafeBuffer()
				cmd.SetOut(buf)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				errC := make(chan error)
				go func() {
					errC <- cmd.ExecuteContext(ctx)
				}()
				waitForPortForwardReady(t, buf)

				t.Parallel() // Port is reserved, enable parallel execution.

				// Open a connection to both listener 1 and 2 simultaneously and
				// then test them out of order.
				d := net.Dialer{Timeout: 3 * time.Second}
				c1, err := d.DialContext(ctx, c.network, localAddress1)
				require.NoError(t, err, "open connection 1 to 'local' listener 1")
				defer c1.Close()
				c2, err := d.DialContext(ctx, c.network, localAddress2)
				require.NoError(t, err, "open connection 2 to 'local' listener 2")
				defer c2.Close()
				testDial(t, c2)
				testDial(t, c1)

				cancel()
				err = <-errC
				require.ErrorIs(t, err, context.Canceled)
			})
		})
	}

	// Test doing a TCP -> Unix forward.
	//nolint:paralleltest
	t.Run("TCP2Unix", func(t *testing.T) {
		var (
			// Find the TCP and Unix cases so we can use their setupLocal and
			// setupRemote methods respectively.
			tcpCase  = cases[0]
			unixCase = cases[2]

			// Setup remote Unix listener.
			p1 = setupTestListener(t, unixCase.setupRemote(t))
		)

		// Create a flag that forwards from local TCP to Unix listener 1.
		// Notably this is a --unix flag.
		localAddress, localFlag := tcpCase.setupLocal(t)
		flag := fmt.Sprintf(unixCase.flag, localFlag, p1)

		// Launch port-forward in a goroutine so we can start dialing
		// the "local" listener.
		cmd, root := clitest.New(t, "port-forward", workspace.Name, flag)
		clitest.SetupConfig(t, client, root)
		buf := newThreadSafeBuffer()
		cmd.SetOut(buf)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errC := make(chan error)
		go func() {
			errC <- cmd.ExecuteContext(ctx)
		}()
		waitForPortForwardReady(t, buf)

		t.Parallel() // Port is reserved, enable parallel execution.

		// Open two connections simultaneously and test them out of
		// sync.
		d := net.Dialer{Timeout: 3 * time.Second}
		c1, err := d.DialContext(ctx, tcpCase.network, localAddress)
		require.NoError(t, err, "open connection 1 to 'local' listener")
		defer c1.Close()
		c2, err := d.DialContext(ctx, tcpCase.network, localAddress)
		require.NoError(t, err, "open connection 2 to 'local' listener")
		defer c2.Close()
		testDial(t, c2)
		testDial(t, c1)

		cancel()
		err = <-errC
		require.ErrorIs(t, err, context.Canceled)
	})

	// Test doing TCP, UDP and Unix at the same time.
	//nolint:paralleltest
	t.Run("All", func(t *testing.T) {
		var (
			// These aren't fixed size because we exclude Unix on Windows.
			dials = []addr{}
			flags = []string{}
		)

		// Start listeners and populate arrays with the cases.
		for _, c := range cases {
			if strings.HasPrefix(c.network, "unix") && runtime.GOOS == "windows" {
				// Unix isn't supported on Windows, but we can still
				// test other protocols together.
				continue
			}

			p := setupTestListener(t, c.setupRemote(t))

			localAddress, localFlag := c.setupLocal(t)
			dials = append(dials, addr{
				network: c.network,
				addr:    localAddress,
			})
			flags = append(flags, fmt.Sprintf(c.flag, localFlag, p))
		}

		// Launch port-forward in a goroutine so we can start dialing
		// the "local" listeners.
		cmd, root := clitest.New(t, append([]string{"port-forward", workspace.Name}, flags...)...)
		clitest.SetupConfig(t, client, root)
		buf := newThreadSafeBuffer()
		cmd.SetOut(buf)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errC := make(chan error)
		go func() {
			errC <- cmd.ExecuteContext(ctx)
		}()
		waitForPortForwardReady(t, buf)

		t.Parallel() // Port is reserved, enable parallel execution.

		// Open connections to all items in the "dial" array.
		var (
			d     = net.Dialer{Timeout: 3 * time.Second}
			conns = make([]net.Conn, len(dials))
		)
		for i, a := range dials {
			c, err := d.DialContext(ctx, a.network, a.addr)
			require.NoErrorf(t, err, "open connection %v to 'local' listener %v", i+1, i+1)
			t.Cleanup(func() {
				_ = c.Close()
			})
			conns[i] = c
		}

		// Test each connection in reverse order.
		for i := len(conns) - 1; i >= 0; i-- {
			testDial(t, conns[i])
		}

		cancel()
		err := <-errC
		require.ErrorIs(t, err, context.Canceled)
	})
}

// runAgent creates a fake workspace and starts an agent locally for that
// workspace. The agent will be cleaned up on test completion.
func runAgent(t *testing.T, client *codersdk.Client, userID uuid.UUID) ([]codersdk.WorkspaceResource, codersdk.Workspace) {
	ctx := context.Background()
	user, err := client.User(ctx, userID.String())
	require.NoError(t, err, "specified user does not exist")
	require.Greater(t, len(user.OrganizationIDs), 0, "user has no organizations")
	orgID := user.OrganizationIDs[0]

	// Setup template
	agentToken := uuid.NewString()
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
	cmd, root := clitest.New(t, "agent", "--agent-token", agentToken, "--agent-url", client.URL.String(), "--wireguard=false")
	clitest.SetupConfig(t, client, root)
	errC := make(chan error)
	agentCtx, agentCancel := context.WithCancel(ctx)
	t.Cleanup(func() {
		agentCancel()
		err := <-errC
		require.NoError(t, err)
	})
	go func() {
		errC <- cmd.ExecuteContext(agentCtx)
	}()

	coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
	resources, err := client.WorkspaceResourcesByBuild(context.Background(), workspace.LatestBuild.ID)
	require.NoError(t, err)

	return resources, workspace
}

// setupTestListener starts accepting connections and echoing a single packet.
// Returns the listener and the listen port or Unix path.
func setupTestListener(t *testing.T, l net.Listener) string {
	t.Helper()

	// Wait for listener to completely exit before releasing.
	done := make(chan struct{})
	t.Cleanup(func() {
		_ = l.Close()
		<-done
	})
	go func() {
		defer close(done)
		// Guard against testAccept running require after test completion.
		var wg sync.WaitGroup
		defer wg.Wait()

		for {
			c, err := l.Accept()
			if err != nil {
				_ = l.Close()
				return
			}

			wg.Add(1)
			go func() {
				testAccept(t, c)
				wg.Done()
			}()
		}
	}()

	addr := l.Addr().String()
	if !strings.HasPrefix(l.Addr().Network(), "unix") {
		_, port, err := net.SplitHostPort(addr)
		require.NoErrorf(t, err, "split non-Unix listen path %q", addr)
		addr = port
	}

	return addr
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
	t.Helper()
	b := make([]byte, len(payload)+16)
	n, err := r.Read(b)
	assert.NoError(t, err, "read payload")
	assert.Equal(t, len(payload), n, "read payload length does not match")
	assert.Equal(t, payload, b[:n])
}

func assertWritePayload(t *testing.T, w io.Writer, payload []byte) {
	t.Helper()
	n, err := w.Write(payload)
	assert.NoError(t, err, "write payload")
	assert.Equal(t, len(payload), n, "payload length does not match")
}

func waitForPortForwardReady(t *testing.T, output *threadSafeBuffer) {
	t.Helper()
	for i := 0; i < 100; i++ {
		time.Sleep(250 * time.Millisecond)

		data := output.String()
		if strings.Contains(data, "Ready!") {
			return
		}
	}

	t.Fatal("port-forward command did not become ready in time")
}

type addr struct {
	network string
	addr    string
}

type threadSafeBuffer struct {
	b   *bytes.Buffer
	mut *sync.RWMutex
}

func newThreadSafeBuffer() *threadSafeBuffer {
	return &threadSafeBuffer{
		b:   bytes.NewBuffer(nil),
		mut: new(sync.RWMutex),
	}
}

var (
	_ io.Reader = &threadSafeBuffer{}
	_ io.Writer = &threadSafeBuffer{}
)

// Read implements io.Reader.
func (b *threadSafeBuffer) Read(p []byte) (int, error) {
	b.mut.RLock()
	defer b.mut.RUnlock()

	return b.b.Read(p)
}

// Write implements io.Writer.
func (b *threadSafeBuffer) Write(p []byte) (int, error) {
	b.mut.Lock()
	defer b.mut.Unlock()

	return b.b.Write(p)
}

func (b *threadSafeBuffer) String() string {
	b.mut.RLock()
	defer b.mut.RUnlock()

	return b.b.String()
}
