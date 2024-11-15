//go:build windows

package cli_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/testutil"
)

func TestVPNDaemonRun(t *testing.T) {
	t.Parallel()

	t.Run("InvalidFlags", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			Name          string
			Args          []string
			ErrorContains string
		}{
			{
				Name:          "NoReadHandle",
				Args:          []string{"--rpc-write-handle", "10"},
				ErrorContains: "rpc-read-handle",
			},
			{
				Name:          "NoWriteHandle",
				Args:          []string{"--rpc-read-handle", "10"},
				ErrorContains: "rpc-write-handle",
			},
			{
				Name:          "NegativeReadHandle",
				Args:          []string{"--rpc-read-handle", "-1", "--rpc-write-handle", "10"},
				ErrorContains: "rpc-read-handle",
			},
			{
				Name:          "NegativeWriteHandle",
				Args:          []string{"--rpc-read-handle", "10", "--rpc-write-handle", "-1"},
				ErrorContains: "rpc-write-handle",
			},
			{
				Name:          "SameHandles",
				Args:          []string{"--rpc-read-handle", "10", "--rpc-write-handle", "10"},
				ErrorContains: "rpc-read-handle",
			},
		}

		for _, c := range cases {
			c := c
			t.Run(c.Name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				inv, _ := clitest.New(t, append([]string{"vpn-daemon", "run"}, c.Args...)...)
				err := inv.WithContext(ctx).Run()
				require.ErrorContains(t, err, c.ErrorContains)
			})
		}
	})

	t.Run("StartsTunnel", func(t *testing.T) {
		t.Parallel()

		r1, w1, err := os.Pipe()
		require.NoError(t, err)
		defer r1.Close()
		defer w1.Close()
		r2, w2, err := os.Pipe()
		require.NoError(t, err)
		defer r2.Close()
		defer w2.Close()

		ctx := testutil.Context(t, testutil.WaitLong)
		inv, _ := clitest.New(t, "vpn-daemon", "run", "--rpc-read-handle", fmt.Sprint(r1.Fd()), "--rpc-write-handle", fmt.Sprint(w2.Fd()))
		waiter := clitest.StartWithWaiter(t, inv.WithContext(ctx))

		// Send garbage which should cause the handshake to fail and the daemon
		// to exit.
		_, err = w1.Write([]byte("garbage"))
		require.NoError(t, err)
		waiter.Cancel()
		err = waiter.Wait()
		require.ErrorContains(t, err, "handshake failed")
	})

	// TODO: once the VPN tunnel functionality is implemented, add tests that
	// actually try to instantiate a tunnel to a workspace
}
