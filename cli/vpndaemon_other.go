//go:build !windows
package cli
import (
	"errors"
	"github.com/coder/serpent"
)
func (*RootCmd) vpnDaemonRun() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "run",
		Short: "Run the VPN daemon on Windows.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(_ *serpent.Invocation) error {
			return errors.New("vpn-daemon subcommand is not supported on this platform")
		},
	}
	return cmd
}
