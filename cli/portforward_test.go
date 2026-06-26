package cli_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"slices"
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
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
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

func listenLocalUDPWithPrefix(t *testing.T, prefix []byte) net.Listener {
	addr := net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	}
	cfg := udp.ListenConfig{AcceptFilter: func(bytes []byte) bool {
		if len(bytes) < len(prefix) {
			return false
		}
		return slices.Equal(prefix, bytes[:len(prefix)])
	}}
	l, err := cfg.Listen("udp", &addr)
	require.NoError(t, err, "create UDP listener")
	return l
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
		// workspace. The prefix is generated per test case and can be used to
		// filter connections.
		setupRemote func(t *testing.T, prefix []byte) net.Listener
		// the local address(es) to "dial"
		localAddress []string
	}{
		{
			name:    "TCP",
			network: "tcp",
			flag:    []string{"--tcp=5555:%v", "--tcp=6666:%v"},
			setupRemote: func(t *testing.T, _ []byte) net.Listener {
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
			setupRemote: func(t *testing.T, _ []byte) net.Listener {
				l, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err, "create TCP listener")
				return l
			},
			localAddress: []string{"[::1]:5566", "[::1]:6655"},
		},
		{
			name:         "UDP",
			network:      "udp",
			flag:         []string{"--udp=7777:%v", "--udp=8888:%v"},
			setupRemote:  listenLocalUDPWithPrefix,
			localAddress: []string{"127.0.0.1:7777", "127.0.0.1:8888"},
		},
		{
			name:         "UDP-opportunistic-ipv6",
			network:      "udp",
			flag:         []string{"--udp=7788:%v", "--udp=8877:%v"},
			setupRemote:  listenLocalUDPWithPrefix,
			localAddress: []string{"[::1]:7788", "[::1]:8877"},
		},
		{
			name:    "TCPWithAddress",
			network: "tcp", flag: []string{"--tcp=10.10.10.99:9999:%v", "--tcp=10.10.10.10:1010:%v"},
			setupRemote: func(t *testing.T, _ []byte) net.Listener {
				l, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err, "create TCP listener")
				return l
			},
			localAddress: []string{"10.10.10.99:9999", "10.10.10.10:1010"},
		},
		{
			name:    "TCP-IPv6",
			network: "tcp", flag: []string{"--tcp=[fe80::99]:9999:%v", "--tcp=[fe80::10]:1010:%v"},
			setupRemote: func(t *testing.T, _ []byte) net.Listener {
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
		t.Run(c.name+"_OnePort", func(t *testing.T) {
			t.Parallel()
			prefix := generateRandomPrefix(t)
			p1 := setupTestListener(t, c.setupRemote(t, prefix), prefix)

			// Create a flag that forwards from local to listener 1.
			flag := fmt.Sprintf(c.flag[0], p1)

			// Launch port-forward in a goroutine so we can start dialing
			// the "local" listener.
			inv, root := clitest.New(t, "-v", "port-forward", workspace.Name, flag)
			clitest.SetupConfig(t, member, root)
			stdout := expecter.NewAttachedToInvocation(t, inv)

			iNet := testutil.NewInProcNet()
			inv.Net = iNet
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()
			errC := make(chan error)
			go func() {
				err := inv.WithContext(ctx).Run()
				t.Logf("command complete; err=%s", err.Error())
				errC <- err
			}()
			stdout.ExpectMatch(ctx, "Ready!")

			// Open two connections simultaneously and test them out of
			// sync.
			dialCtx, dialCtxCancel := context.WithTimeout(ctx, testutil.WaitShort)
			defer dialCtxCancel()
			c1, err := iNet.Dial(dialCtx, testutil.NewAddr(c.network, c.localAddress[0]))
			require.NoError(t, err, "open connection 1 to 'local' listener")
			defer c1.Close()
			c2, err := iNet.Dial(dialCtx, testutil.NewAddr(c.network, c.localAddress[0]))
			require.NoError(t, err, "open connection 2 to 'local' listener")
			defer c2.Close()
			testDial(t, c2, prefix)
			testDial(t, c1, prefix)

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
			prefix := generateRandomPrefix(t)
			p1 := setupTestListener(t, c.setupRemote(t, prefix), prefix)
			p2 := setupTestListener(t, c.setupRemote(t, prefix), prefix)

			// Create a flags for listener 1 and listener 2.
			flag1 := fmt.Sprintf(c.flag[0], p1)
			flag2 := fmt.Sprintf(c.flag[1], p2)

			// Launch port-forward in a goroutine so we can start dialing
			// the "local" listeners.
			inv, root := clitest.New(t, "-v", "port-forward", workspace.Name, flag1, flag2)
			clitest.SetupConfig(t, member, root)
			stdout := expecter.NewAttachedToInvocation(t, inv)

			iNet := testutil.NewInProcNet()
			inv.Net = iNet
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()
			errC := make(chan error)
			go func() {
				errC <- inv.WithContext(ctx).Run()
			}()
			stdout.ExpectMatch(ctx, "Ready!")

			// Open a connection to both listener 1 and 2 simultaneously and
			// then test them out of order.
			dialCtx, dialCtxCancel := context.WithTimeout(ctx, testutil.WaitShort)
			defer dialCtxCancel()
			c1, err := iNet.Dial(dialCtx, testutil.NewAddr(c.network, c.localAddress[0]))
			require.NoError(t, err, "open connection 1 to 'local' listener 1")
			defer c1.Close()
			c2, err := iNet.Dial(dialCtx, testutil.NewAddr(c.network, c.localAddress[1]))
			require.NoError(t, err, "open connection 2 to 'local' listener 2")
			defer c2.Close()
			testDial(t, c2, prefix)
			testDial(t, c1, prefix)

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
			dials = []testutil.Addr{}
			flags = []string{}
		)

		prefix := generateRandomPrefix(t)
		// Start listeners and populate arrays with the cases.
		for _, c := range cases {
			p := setupTestListener(t, c.setupRemote(t, prefix), prefix)

			dials = append(dials, testutil.NewAddr(c.network, c.localAddress[0]))
			flags = append(flags, fmt.Sprintf(c.flag[0], p))
		}

		// Launch port-forward in a goroutine so we can start dialing
		// the "local" listeners.
		inv, root := clitest.New(t, append([]string{"-v", "port-forward", workspace.Name}, flags...)...)
		clitest.SetupConfig(t, member, root)
		stdout := expecter.NewAttachedToInvocation(t, inv)

		iNet := testutil.NewInProcNet()
		inv.Net = iNet
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		errC := make(chan error)
		go func() {
			errC <- inv.WithContext(ctx).Run()
		}()
		stdout.ExpectMatch(ctx, "Ready!")

		// Open connections to all items in the "dial" array.
		var (
			dialCtx, dialCtxCancel = context.WithTimeout(ctx, testutil.WaitShort)
			conns                  = make([]net.Conn, len(dials))
		)
		defer dialCtxCancel()
		for i, a := range dials {
			c, err := iNet.Dial(dialCtx, a)
			require.NoErrorf(t, err, "open connection %v to 'local' listener %v", i+1, i+1)
			t.Cleanup(func() {
				_ = c.Close()
			})
			conns[i] = c
		}

		// Test each connection in reverse order.
		for i := len(conns) - 1; i >= 0; i-- {
			testDial(t, conns[i], prefix)
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

		prefix := generateRandomPrefix(t)

		remoteLis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err, "create TCP listener")
		p1 := setupTestListener(t, remoteLis, prefix)

		// Create a flag that forwards from local 5555 to remote listener port.
		flag := fmt.Sprintf("--tcp=5555:%v", p1)

		// Launch port-forward in a goroutine so we can start dialing
		// the "local" listener.
		inv, root := clitest.New(t, "-v", "port-forward", workspace.Name, flag)
		clitest.SetupConfig(t, member, root)
		stdout := expecter.NewAttachedToInvocation(t, inv)

		iNet := testutil.NewInProcNet()
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
		stdout.ExpectMatch(ctx, "Ready!")

		// Test IPv4 still works
		dialCtx, dialCtxCancel := context.WithTimeout(ctx, testutil.WaitShort)
		defer dialCtxCancel()
		c1, err := iNet.Dial(dialCtx, testutil.NewAddr("tcp", "127.0.0.1:5555"))
		require.NoError(t, err, "open connection 1 to 'local' listener")
		defer c1.Close()
		testDial(t, c1, prefix)

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

// generateRandomPrefix generates a unique prefix per test case to ensure that we can filter out any cross-talk on the
// local network.
func generateRandomPrefix(t *testing.T) []byte {
	t.Helper()
	prefix := make([]byte, 16)
	n, err := rand.Read(prefix)
	require.NoError(t, err)
	require.Equal(t, 16, n)
	return prefix
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
// Returns the listen port.
func setupTestListener(t *testing.T, l net.Listener, prefix []byte) string {
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

			wg.Go(func() {
				echoIfPrefixed(t, c, prefix)
			})
		}
	}()

	addr := l.Addr().String()
	_, port, err := net.SplitHostPort(addr)
	require.NoErrorf(t, err, "split non-Unix listen path %q", addr)
	return port
}

const dialTestPayload = "dean-was-here123"

func newPayload(prefix []byte) []byte {
	payload := make([]byte, 0, len(dialTestPayload)+len(prefix))
	payload = append(payload, prefix...)
	payload = append(payload, dialTestPayload...)
	return payload
}

func testDial(t *testing.T, c net.Conn, prefix []byte) {
	t.Helper()

	assertWritePayload(t, c, prefix)
	assertReadPayload(t, c, prefix)
}

func echoIfPrefixed(t *testing.T, c net.Conn, prefix []byte) {
	t.Helper()
	defer c.Close()

	// here we don't want to assert anything, because the listener is exposed to the OS, so who knows what might
	// connect. If we get the expected prefix to our message, echo it back.
	b := make([]byte, 2048)
	n, err := c.Read(b)
	if err != nil {
		t.Logf("read failed (could be crosstalk): %v", err)
		return
	}
	if n < len(prefix) {
		t.Logf("short read (could be crosstalk): read %x", b[:n])
		return
	}
	if !bytes.HasPrefix(b, prefix) {
		t.Logf("missing prefix (could be crosstalk), wanted %x got %x", prefix, b[:n])
		return
	}
	_, err = c.Write(b[:n])
	if err != nil {
		t.Logf("write failed: %v", err)
	}
}

func assertReadPayload(t *testing.T, r io.Reader, prefix []byte) {
	t.Helper()
	payload := newPayload(prefix)
	b := make([]byte, len(payload)+16)
	n, err := r.Read(b)
	assert.NoError(t, err, "read payload")
	assert.Equal(t, len(payload), n, "read payload length does not match")
	assert.Equal(t, payload, b[:n])
}

func assertWritePayload(t *testing.T, w io.Writer, prefix []byte) {
	t.Helper()
	payload := newPayload(prefix)
	n, err := w.Write(payload)
	assert.NoError(t, err, "write payload")
	assert.Equal(t, len(payload), n, "payload length does not match")
}
