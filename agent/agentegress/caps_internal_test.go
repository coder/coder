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

	execer := &successfulExecer{}
	called := false
	e, err := NewEnforcer(testutil.Logger(t), EnforcerOptions{
		Execer:    execer,
		ProxyPort: 41001,
		DNSPort:   41002,
		UDPPort:   41003,
		capabilityLockdown: func() error {
			called = true
			return nil
		},
	})
	require.NoError(t, err)
	registerTransparentUDP(41003)
	t.Cleanup(func() { unregisterTransparentUDP(41003) })

	require.NoError(t, e.Install(t.Context()))
	require.True(t, called)
	require.True(t, e.lockedDown)
	before := execer.count()
	require.NoError(t, e.Remove(t.Context()))
	require.Equal(t, before, execer.count(), "Remove must not exec after lockdown")
}

func TestEnforcerCapabilityLockdownDisabled(t *testing.T) {
	t.Parallel()

	execer := &successfulExecer{}
	called := false
	e, err := NewEnforcer(testutil.Logger(t), EnforcerOptions{
		Execer:                  execer,
		ProxyPort:               42001,
		DNSPort:                 42002,
		UDPPort:                 42003,
		LockdownCapabilities:    false,
		LockdownCapabilitiesSet: true,
		capabilityLockdown: func() error {
			called = true
			return nil
		},
	})
	require.NoError(t, err)
	registerTransparentUDP(42003)
	t.Cleanup(func() { unregisterTransparentUDP(42003) })

	require.NoError(t, e.Install(t.Context()))
	require.False(t, called)
	require.NoError(t, e.Remove(t.Context()))
}
