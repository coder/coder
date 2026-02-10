//go:build linux

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
	t.Run("InvalidFlags", func(t *testing.T) {
		cases := []struct {
			Name          string
			Args          []string
			ErrorContains string
		}{
			{
				Name:          "NoReadFD",
				Args:          []string{"--rpc-write-fd", "10"},
				ErrorContains: "rpc-read-fd",
			},
			{
				Name:          "NoWriteFD",
				Args:          []string{"--rpc-read-fd", "10"},
				ErrorContains: "rpc-write-fd",
			},
			{
				Name:          "NegativeReadFD",
				Args:          []string{"--rpc-read-fd", "-1", "--rpc-write-fd", "10"},
				ErrorContains: "rpc-read-fd",
			},
			{
				Name:          "NegativeWriteFD",
				Args:          []string{"--rpc-read-fd", "10", "--rpc-write-fd", "-1"},
				ErrorContains: "rpc-write-fd",
			},
			{
				Name:          "SameFDs",
				Args:          []string{"--rpc-read-fd", "10", "--rpc-write-fd", "10"},
				ErrorContains: "rpc-read-fd",
			},
		}

		for _, c := range cases {
			t.Run(c.Name, func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)
				inv, _ := clitest.New(t, append([]string{"vpn-daemon", "run"}, c.Args...)...)
				err := inv.WithContext(ctx).Run()
				require.ErrorContains(t, err, c.ErrorContains)
			})
		}
	})

	t.Run("StartsTunnel", func(t *testing.T) {
		r1, w1, err := os.Pipe()
		require.NoError(t, err)
		defer r1.Close()
		defer w1.Close()
		r2, w2, err := os.Pipe()
		require.NoError(t, err)
		defer r2.Close()
		defer w2.Close()

		ctx := testutil.Context(t, testutil.WaitLong)
		inv, _ := clitest.New(t, "vpn-daemon", "run", "--rpc-read-fd", fmt.Sprint(r1.Fd()), "--rpc-write-fd", fmt.Sprint(w2.Fd()))
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
