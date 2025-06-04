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
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestPortForward_None(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	inv, root := clitest.New(t, "port-forward", "blah")
	clitest.SetupConfig(t, member, root)

	err := inv.Run()
	require.Error(t, err)
	require.ErrorContains(t, err, "no port-forwards")
}

func TestPortForward(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		network string
		// The flag(s) to pass to `coder port-forward X` to port-forward this type
		// of connection. Has one format arg (string) for the remote address.
		flag []string
		// setupRemote creates a "remote" listener to emulate a service in the
		// workspace.
		setupRemote func(t *testing.T) net.Listener
		// the local address(es) to "dial"
		localAddress []string
	}{
		{
			name:    "TCP",
			network: "tcp",
			flag:    []string{"--tcp=5555:%v", "--tcp=6666:%v"},
			setupRemote: func(t *testing.T) net.Listener {
				l, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err, "create TCP listener")
				return l
			},
			localAddress: []string{"127.0.0.1:5555", "127.0.0.1:6666"},
		},
		{
			name:    "TCP-opportunistic-ipv6",
			network: "tcp",
			flag:    []string{"--tcp=5566:%v", "--tcp=6655:%v"},
			setupRemote: func(t *testing.T) net.Listener {
				l, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err, "create TCP listener")
				return l
			},
			localAddress: []string{"[::1]:5566", "[::1]:6655"},
		},
		{
			name:    "UDP",
			network: "udp",
			flag:    []string{"--udp=7777:%v", "--udp=8888:%v"},
			setupRemote: func(t *testing.T) net.Listener {
				addr := net.UDPAddr{
					IP:   net.ParseIP("127.0.0.1"),
					Port: 0,
				}
				l, err := udp.Listen("udp", &addr)
				require.NoError(t, err, "create UDP listener")
				return l
			},
			localAddress: []string{"127.0.0.1:7777", "127.0.0.1:8888"},
		},
		{
			name:    "UDP-opportunistic-ipv6",
			network: "udp",
			flag:    []string{"--udp=7788:%v", "--udp=8877:%v"},
			setupRemote: func(t *testing.T) net.Listener {
				addr := net.UDPAddr{
					IP:   net.ParseIP("127.0.0.1"),
					Port: 0,
				}
				l, err := udp.Listen("udp", &addr)
				require.NoError(t, err, "create UDP listener")
				return l
			},
			localAddress: []string{"[::1]:7788", "[::1]:8877"},
		},
		{
			name:    "TCPWithAddress",
			network: "tcp", flag: []string{"--tcp=10.10.10.99:9999:%v", "--tcp=10.10.10.10:1010:%v"},
			setupRemote: func(t *testing.T) net.Listener {
				l, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err, "create TCP listener")
				return l
			},
			localAddress: []string{"10.10.10.99:9999", "10.10.10.10:1010"},
		},
		{
			name:    "TCP-IPv6",
			network: "tcp", flag: []string{"--tcp=[fe80::99]:9999:%v", "--tcp=[fe80::10]:1010:%v"},
			setupRemote: func(t *testing.T) net.Listener {
				l, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err, "create TCP listener")
				return l
			},
			localAddress: []string{"[fe80::99]:9999", "[fe80::10]:1010"},
		},
	}

	// Setup agent once to be shared between test-cases (avoid expensive
	// non-parallel setup).
	var (
		wuTick     = make(chan time.Time)
		wuFlush    = make(chan int, 1)
		client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
			WorkspaceUsageTrackerTick:  wuTick,
			WorkspaceUsageTrackerFlush: wuFlush,
		})
		admin              = coderdtest.CreateFirstUser(t, client)
		member, memberUser = coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		workspace          = runAgent(t, client, memberUser.ID, db)
	)

	for _, c := range cases {
		c := c
		t.Run(c.name+"_OnePort", func(t *testing.T) {
			t.Parallel()
			p1 := setupTestListener(t, c.setupRemote(t))

			// Create a flag that forwards from local to listener 1.
			flag := fmt.Sprintf(c.flag[0], p1)

			// Launch port-forward in a goroutine so we can start dialing
			// the "local" listener.
			inv, root := clitest.New(t, "-v", "port-forward", workspace.Name, flag)
			clitest.SetupConfig(t, member, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()
			inv.Stderr = pty.Output()

			iNet := newInProcNet()
			inv.Net = iNet
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()
			errC := make(chan error)
			go func() {
				err := inv.WithContext(ctx).Run()
				t.Logf("command complete; err=%s", err.Error())
				errC <- err
			}()
			pty.ExpectMatchContext(ctx, "Ready!")

			// Open two connections simultaneously and test them out of
			// sync.
			dialCtx, dialCtxCancel := context.WithTimeout(ctx, testutil.WaitShort)
			defer dialCtxCancel()
			c1, err := iNet.dial(dialCtx, addr{c.network, c.localAddress[0]})
			require.NoError(t, err, "open connection 1 to 'local' listener")
			defer c1.Close()
			c2, err := iNet.dial(dialCtx, addr{c.network, c.localAddress[0]})
			require.NoError(t, err, "open connection 2 to 'local' listener")
			defer c2.Close()
			testDial(t, c2)
			testDial(t, c1)

			cancel()
			err = <-errC
			require.ErrorIs(t, err, context.Canceled)

			flushCtx := testutil.Context(t, testutil.WaitShort)
			testutil.RequireSend(flushCtx, t, wuTick, dbtime.Now())
			_ = testutil.TryReceive(flushCtx, t, wuFlush)
			updated, err := client.Workspace(context.Background(), workspace.ID)
			require.NoError(t, err)
			require.Greater(t, updated.LastUsedAt, workspace.LastUsedAt)
		})

		t.Run(c.name+"_TwoPorts", func(t *testing.T) {
			t.Parallel()
			var (
				p1 = setupTestListener(t, c.setupRemote(t))
				p2 = setupTestListener(t, c.setupRemote(t))
			)

			// Create a flags for listener 1 and listener 2.
			flag1 := fmt.Sprintf(c.flag[0], p1)
			flag2 := fmt.Sprintf(c.flag[1], p2)

			// Launch port-forward in a goroutine so we can start dialing
			// the "local" listeners.
			inv, root := clitest.New(t, "-v", "port-forward", workspace.Name, flag1, flag2)
			clitest.SetupConfig(t, member, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()
			inv.Stderr = pty.Output()

			iNet := newInProcNet()
			inv.Net = iNet
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()
			errC := make(chan error)
			go func() {
				errC <- inv.WithContext(ctx).Run()
			}()
			pty.ExpectMatchContext(ctx, "Ready!")

			// Open a connection to both listener 1 and 2 simultaneously and
			// then test them out of order.
			dialCtx, dialCtxCancel := context.WithTimeout(ctx, testutil.WaitShort)
			defer dialCtxCancel()
			c1, err := iNet.dial(dialCtx, addr{c.network, c.localAddress[0]})
			require.NoError(t, err, "open connection 1 to 'local' listener 1")
			defer c1.Close()
			c2, err := iNet.dial(dialCtx, addr{c.network, c.localAddress[1]})
			require.NoError(t, err, "open connection 2 to 'local' listener 2")
			defer c2.Close()
			testDial(t, c2)
			testDial(t, c1)

			cancel()
			err = <-errC
			require.ErrorIs(t, err, context.Canceled)

			flushCtx := testutil.Context(t, testutil.WaitShort)
			testutil.RequireSend(flushCtx, t, wuTick, dbtime.Now())
			_ = testutil.TryReceive(flushCtx, t, wuFlush)
			updated, err := client.Workspace(context.Background(), workspace.ID)
			require.NoError(t, err)
			require.Greater(t, updated.LastUsedAt, workspace.LastUsedAt)
		})
	}

	t.Run("All", func(t *testing.T) {
		t.Parallel()
		var (
			dials = []addr{}
			flags = []string{}
		)

		// Start listeners and populate arrays with the cases.
		for _, c := range cases {
			p := setupTestListener(t, c.setupRemote(t))

			dials = append(dials, addr{
				network: c.network,
				addr:    c.localAddress[0],
			})
			flags = append(flags, fmt.Sprintf(c.flag[0], p))
		}

		// Launch port-forward in a goroutine so we can start dialing
		// the "local" listeners.
		inv, root := clitest.New(t, append([]string{"-v", "port-forward", workspace.Name}, flags...)...)
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stderr = pty.Output()

		iNet := newInProcNet()
		inv.Net = iNet
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		errC := make(chan error)
		go func() {
			errC <- inv.WithContext(ctx).Run()
		}()
		pty.ExpectMatchContext(ctx, "Ready!")

		// Open connections to all items in the "dial" array.
		var (
			dialCtx, dialCtxCancel = context.WithTimeout(ctx, testutil.WaitShort)
			conns                  = make([]net.Conn, len(dials))
		)
		defer dialCtxCancel()
		for i, a := range dials {
			c, err := iNet.dial(dialCtx, a)
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

		flushCtx := testutil.Context(t, testutil.WaitShort)
		testutil.RequireSend(flushCtx, t, wuTick, dbtime.Now())
		_ = testutil.TryReceive(flushCtx, t, wuFlush)
		updated, err := client.Workspace(context.Background(), workspace.ID)
		require.NoError(t, err)
		require.Greater(t, updated.LastUsedAt, workspace.LastUsedAt)
	})

	t.Run("IPv6Busy", func(t *testing.T) {
		t.Parallel()

		remoteLis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err, "create TCP listener")
		p1 := setupTestListener(t, remoteLis)

		// Create a flag that forwards from local 5555 to remote listener port.
		flag := fmt.Sprintf("--tcp=5555:%v", p1)

		// Launch port-forward in a goroutine so we can start dialing
		// the "local" listener.
		inv, root := clitest.New(t, "-v", "port-forward", workspace.Name, flag)
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()

		iNet := newInProcNet()
		inv.Net = iNet

		// listen on port 5555 on IPv6 so it's busy when we try to port forward
		busyLis, err := iNet.Listen("tcp", "[::1]:5555")
		require.NoError(t, err)
		defer busyLis.Close()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		errC := make(chan error)
		go func() {
			err := inv.WithContext(ctx).Run()
			t.Logf("command complete; err=%s", err.Error())
			errC <- err
		}()
		pty.ExpectMatchContext(ctx, "Ready!")

		// Test IPv4 still works
		dialCtx, dialCtxCancel := context.WithTimeout(ctx, testutil.WaitShort)
		defer dialCtxCancel()
		c1, err := iNet.dial(dialCtx, addr{"tcp", "127.0.0.1:5555"})
		require.NoError(t, err, "open connection 1 to 'local' listener")
		defer c1.Close()
		testDial(t, c1)

		cancel()
		err = <-errC
		require.ErrorIs(t, err, context.Canceled)

		flushCtx := testutil.Context(t, testutil.WaitShort)
		testutil.RequireSend(flushCtx, t, wuTick, dbtime.Now())
		_ = testutil.TryReceive(flushCtx, t, wuFlush)
		updated, err := client.Workspace(context.Background(), workspace.ID)
		require.NoError(t, err)
		require.Greater(t, updated.LastUsedAt, workspace.LastUsedAt)
	})
}

// runAgent creates a fake workspace and starts an agent locally for that
// workspace. The agent will be cleaned up on test completion.
// nolint:unused
func runAgent(t *testing.T, client *codersdk.Client, owner uuid.UUID, db database.Store) database.WorkspaceTable {
	user, err := client.User(context.Background(), codersdk.Me)
	require.NoError(t, err, "specified user does not exist")
	require.Greater(t, len(user.OrganizationIDs), 0, "user has no organizations")
	orgID := user.OrganizationIDs[0]
	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: orgID,
		OwnerID:        owner,
	}).WithAgent().Do()

	_ = agenttest.New(t, client.URL, r.AgentToken,
		func(o *agent.Options) {
			o.SSHMaxTimeout = 60 * time.Second
		},
	)
	coderdtest.AwaitWorkspaceAgents(t, client, r.Workspace.ID)
	return r.Workspace
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

func (a addr) Network() string {
	return a.network
}

func (a addr) Address() string {
	return a.addr
}

func (a addr) String() string {
	return a.network + "|" + a.addr
}

type inProcNet struct {
	sync.Mutex

	listeners map[addr]*inProcListener
}

type inProcListener struct {
	c chan net.Conn
	n *inProcNet
	a addr
	o sync.Once
}

func newInProcNet() *inProcNet {
	return &inProcNet{listeners: make(map[addr]*inProcListener)}
}

func (n *inProcNet) Listen(network, address string) (net.Listener, error) {
	a := addr{network, address}
	n.Lock()
	defer n.Unlock()
	if _, ok := n.listeners[a]; ok {
		return nil, xerrors.New("busy")
	}
	l := newInProcListener(n, a)
	n.listeners[a] = l
	return l, nil
}

func (n *inProcNet) dial(ctx context.Context, a addr) (net.Conn, error) {
	n.Lock()
	defer n.Unlock()
	l, ok := n.listeners[a]
	if !ok {
		return nil, xerrors.Errorf("nothing listening on %s", a)
	}
	x, y := net.Pipe()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case l.c <- x:
		return y, nil
	}
}

func newInProcListener(n *inProcNet, a addr) *inProcListener {
	return &inProcListener{
		c: make(chan net.Conn),
		n: n,
		a: a,
	}
}

func (l *inProcListener) Accept() (net.Conn, error) {
	c, ok := <-l.c
	if !ok {
		return nil, net.ErrClosed
	}
	return c, nil
}

func (l *inProcListener) Close() error {
	l.o.Do(func() {
		l.n.Lock()
		defer l.n.Unlock()
		delete(l.n.listeners, l.a)
		close(l.c)
	})
	return nil
}

func (l *inProcListener) Addr() net.Addr {
	return l.a
}
