//go:build linux

package agentegress

import (
	"context"
	"net"
	"net/netip"
	"os/exec"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/pty"

	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

type successfulExecer struct {
	mu       sync.Mutex
	commands int
}

func (e *successfulExecer) CommandContext(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	e.mu.Lock()
	e.commands++
	e.mu.Unlock()
	return exec.CommandContext(ctx, "true")
}

func (*successfulExecer) PTYCommandContext(context.Context, string, ...string) *pty.Cmd {
	panic("not used")
}

func (e *successfulExecer) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.commands
}

func TestEnforcerInstallsWithNoLiveExitNodeReplicas(t *testing.T) {
	t.Parallel()

	proxy, err := New(testutil.Logger(t), Options{
		Dialer: DialerFunc(func(context.Context, netip.AddrPort) (net.Conn, error) {
			panic("no replica should be dialed")
		}),
		Config: agentsdk.EgressConfig{
			ExitNodes:    []agentsdk.EgressExitNode{{ID: uuid.New()}},
			ExitNodePort: 3128,
		},
		ListenAddr:        "127.0.0.1:0",
		UpstreamResolvers: []netip.AddrPort{},
	})
	require.NoError(t, err)
	require.NoError(t, proxy.Start(t.Context()))
	t.Cleanup(func() { _ = proxy.Close() })

	enforcer, err := NewEnforcer(testutil.Logger(t), EnforcerOptions{
		Execer:                  &successfulExecer{},
		ProxyPort:               proxy.Addr().Port(),
		DNSPort:                 proxy.DNSAddr().Port(),
		UDPPort:                 proxy.UDPAddr().Port(),
		LockdownCapabilities:    false,
		LockdownCapabilitiesSet: true,
	})
	require.NoError(t, err)
	require.NoError(t, enforcer.Install(t.Context()))
	t.Cleanup(func() { _ = enforcer.Remove(t.Context()) })
}

func TestEnforcerCapabilityLockdown(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		port     uint16
		disabled bool
	}{
		{name: "enabled", port: 41001},
		{name: "disabled", port: 42001, disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			execer := &successfulExecer{}
			called := false
			opts := EnforcerOptions{
				Execer:    execer,
				ProxyPort: tc.port,
				DNSPort:   tc.port + 1,
				UDPPort:   tc.port + 2,
				capabilityLockdown: func() error {
					called = true
					return nil
				},
			}
			if tc.disabled {
				opts.LockdownCapabilitiesSet = true
			}
			e, err := NewEnforcer(testutil.Logger(t), opts)
			require.NoError(t, err)
			registerTransparentUDP(opts.UDPPort)
			t.Cleanup(func() { unregisterTransparentUDP(opts.UDPPort) })

			require.NoError(t, e.Install(t.Context()))
			require.Equal(t, !tc.disabled, called)
			before := execer.count()
			require.NoError(t, e.Remove(t.Context()))
			if tc.disabled {
				require.Greater(t, execer.count(), before)
			} else {
				require.True(t, e.lockedDown)
				require.Equal(t, before, execer.count())
			}
		})
	}
}
