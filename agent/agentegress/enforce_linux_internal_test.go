//go:build linux

package agentegress

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/testutil"
)

type failingExecer struct{}

func (failingExecer) CommandContext(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "false")
}

func (failingExecer) PTYCommandContext(context.Context, string, ...string) *pty.Cmd {
	panic("not used")
}

func TestProbeErrorMatchesUnavailable(t *testing.T) {
	t.Parallel()

	e, err := NewEnforcer(testutil.Logger(t), EnforcerOptions{
		Execer:    failingExecer{},
		ProxyPort: 1,
		DNSPort:   2,
		UDPPort:   3,
	})
	require.NoError(t, err)
	err = e.probe(t.Context())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrEnforcementUnavailable)
}

func TestHandleIPv6InstallFailure(t *testing.T) {
	t.Parallel()

	installErr := xerrors.New("ip6tables unavailable")
	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		e := &Enforcer{
			logger: testutil.Logger(t),
			readFile: func(path string) ([]byte, error) {
				return []byte("1\n"), nil
			},
		}
		require.NoError(t, e.handleIPv6InstallFailure(t.Context(), installErr))
	})
	t.Run("Disable succeeds", func(t *testing.T) {
		t.Parallel()
		writes := 0
		e := &Enforcer{
			logger: testutil.Logger(t),
			readFile: func(path string) ([]byte, error) {
				if path == "/proc/sys/net/ipv6/conf/all/disable_ipv6" {
					return []byte("0\n"), nil
				}
				return []byte("20010db8000000000000000000000001 02 40 00 80 eth0\n"), nil
			},
			writeFile: func(string, []byte, os.FileMode) error {
				writes++
				return nil
			},
		}
		require.NoError(t, e.handleIPv6InstallFailure(t.Context(), installErr))
		require.Equal(t, 2, writes)
	})
	t.Run("Disable fails and cleans up", func(t *testing.T) {
		t.Parallel()
		execer := &successfulExecer{}
		e := &Enforcer{
			logger:    testutil.Logger(t),
			execer:    execer,
			prefix:    []string{},
			installed: true,
			readFile: func(path string) ([]byte, error) {
				if path == "/proc/sys/net/ipv6/conf/all/disable_ipv6" {
					return []byte("0\n"), nil
				}
				return []byte("20010db8000000000000000000000001 02 40 00 80 eth0\n"), nil
			},
			writeFile: func(string, []byte, os.FileMode) error {
				return xerrors.New("read-only")
			},
		}
		err := e.handleIPv6InstallFailure(t.Context(), installErr)
		require.Error(t, err)
		require.ErrorIs(t, err, installErr)
		require.False(t, e.installed)
		require.Positive(t, execer.count())
	})
}

func TestIPv6Enabled(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		disabled   string
		interfaces string
		want       bool
	}{
		{name: "disabled", disabled: "1\n"},
		{name: "loopback only", disabled: "0\n", interfaces: "00000000000000000000000000000001 01 80 10 80 lo\n"},
		{name: "non-loopback", disabled: "0\n", interfaces: "20010db8000000000000000000000001 02 40 00 80 eth0\n", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := &Enforcer{readFile: func(path string) ([]byte, error) {
				switch path {
				case "/proc/sys/net/ipv6/conf/all/disable_ipv6":
					return []byte(tc.disabled), nil
				case "/proc/net/if_inet6":
					return []byte(tc.interfaces), nil
				default:
					return nil, os.ErrNotExist
				}
			}}
			got, err := e.ipv6Enabled()
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestDisableIPv6(t *testing.T) {
	t.Parallel()

	written := make(map[string]string)
	e := &Enforcer{writeFile: func(path string, data []byte, _ os.FileMode) error {
		written[path] = string(data)
		return nil
	}}
	require.NoError(t, e.disableIPv6())
	require.Equal(t, map[string]string{
		"/proc/sys/net/ipv6/conf/all/disable_ipv6":     "1\n",
		"/proc/sys/net/ipv6/conf/default/disable_ipv6": "1\n",
	}, written)

	e.writeFile = func(string, []byte, os.FileMode) error { return xerrors.New("read-only") }
	require.Error(t, e.disableIPv6())
}
