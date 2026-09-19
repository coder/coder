//go:build linux

package agentegress

import (
	"context"
	"os/exec"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/pty"

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
