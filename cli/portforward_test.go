package cli_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pion/udp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestPortForward_None(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	inv, root := clitest.New(t, "port-forward", "blah")
	clitest.SetupConfig(t, client, root)
	pty := ptytest.New(t).Attach(inv)
	inv.Stderr = pty.Output()

	err := inv.Run()
	require.Error(t, err)
	require.ErrorContains(t, err, "no port-forwards")

	// Check that the help was printed.
	pty.ExpectMatch("port-forward <workspace>")
}

//nolint:tparallel,paralleltest // Subtests require setup that must not be done in parallel.
func TestPortForward(t *testing.T) {
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
		// setupLocal returns an available port that the
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
			name:    "TCPWithAddress",
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
				return l.Addr().String(), fmt.Sprint("0.0.0.0:", port)
			},
		},
	}

	// Setup agent once to be shared between test-cases (avoid expensive
	// non-parallel setup).
	var (
		client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user      = coderdtest.CreateFirstUser(t, client)
		workspace = runAgent(t, client, user.UserID)
	)

	for _, c := range cases {
		c := c
		// Delay parallel tests here because setupLocal reserves
		// a free open port which is not guaranteed to be free
		// between the listener closing and port-forward ready.
		t.Run(c.name+"_OnePort", func(t *testing.T) {
			p1 := setupTestListener(t, c.setupRemote(t))

			// Create a flag that forwards from local to listener 1.
			localAddress, localFlag := c.setupLocal(t)
			flag := fmt.Sprintf(c.flag, localFlag, p1)

			// Launch port-forward in a goroutine so we can start dialing
			// the "local" listener.
			inv, root := clitest.New(t, "-v", "port-forward", workspace.Name, flag)
			clitest.SetupConfig(t, client, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()
			inv.Stderr = pty.Output()
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()
			errC := make(chan error)
			go func() {
				errC <- inv.WithContext(ctx).Run()
			}()
			pty.ExpectMatchContext(ctx, "Ready!")

			t.Parallel() // Port is reserved, enable parallel execution.

			// Open two connections simultaneously and test them out of
			// sync.
			d := net.Dialer{Timeout: testutil.WaitShort}
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

		t.Run(c.name+"_TwoPorts", func(t *testing.T) {
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
			inv, root := clitest.New(t, "-v", "port-forward", workspace.Name, flag1, flag2)
			clitest.SetupConfig(t, client, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()
			inv.Stderr = pty.Output()
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()
			errC := make(chan error)
			go func() {
				errC <- inv.WithContext(ctx).Run()
			}()
			pty.ExpectMatchContext(ctx, "Ready!")

			t.Parallel() // Port is reserved, enable parallel execution.

			// Open a connection to both listener 1 and 2 simultaneously and
			// then test them out of order.
			d := net.Dialer{Timeout: testutil.WaitShort}
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
	}

	// Test doing TCP and UDP at the same time.
	t.Run("All", func(t *testing.T) {
		var (
			dials = []addr{}
			flags = []string{}
		)

		// Start listeners and populate arrays with the cases.
		for _, c := range cases {
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
		inv, root := clitest.New(t, append([]string{"-v", "port-forward", workspace.Name}, flags...)...)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stderr = pty.Output()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		errC := make(chan error)
		go func() {
			errC <- inv.WithContext(ctx).Run()
		}()
		pty.ExpectMatchContext(ctx, "Ready!")

		t.Parallel() // Port is reserved, enable parallel execution.

		// Open connections to all items in the "dial" array.
		var (
			d     = net.Dialer{Timeout: testutil.WaitShort}
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
// nolint:unused
func runAgent(t *testing.T, client *codersdk.Client, userID uuid.UUID) codersdk.Workspace {
	ctx := context.Background()
	user, err := client.User(ctx, userID.String())
	require.NoError(t, err, "specified user does not exist")
	require.Greater(t, len(user.OrganizationIDs), 0, "user has no organizations")
	orgID := user.OrganizationIDs[0]

	// Setup template
	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, orgID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(agentToken),
	})

	// Create template and workspace
	template := coderdtest.CreateTemplate(t, client, orgID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, orgID, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	_ = agenttest.New(t, client.URL, agentToken,
		func(o *agent.Options) {
			o.SSHMaxTimeout = 60 * time.Second
		},
	)
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	return workspace
}

// setupTestListener starts accepting connections and echoing a single packet.
// Returns the listener and the listen port.
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
	_, port, err := net.SplitHostPort(addr)
	require.NoErrorf(t, err, "split non-Unix listen path %q", addr)
	addr = port

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

type addr struct {
	network string
	addr    string
}
