//go:build darwin

package cli

import (
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/vpn"
	"github.com/coder/serpent"
)

func (*RootCmd) vpnDaemonRun() *serpent.Command {
	var (
		rpcReadFD  int64
		rpcWriteFD int64
	)

	cmd := &serpent.Command{
		Use:   "run",
		Short: "Run the VPN daemon on macOS.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Options: serpent.OptionSet{
			{
				Flag:        "rpc-read-fd",
				Env:         "CODER_VPN_DAEMON_RPC_READ_FD",
				Description: "The file descriptor for the pipe to read from the RPC connection.",
				Value:       serpent.Int64Of(&rpcReadFD),
				Required:    true,
			},
			{
				Flag:        "rpc-write-fd",
				Env:         "CODER_VPN_DAEMON_RPC_WRITE_FD",
				Description: "The file descriptor for the pipe to write to the RPC connection.",
				Value:       serpent.Int64Of(&rpcWriteFD),
				Required:    true,
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			if rpcReadFD < 0 || rpcWriteFD < 0 {
				return xerrors.Errorf("rpc-read-fd (%v) and rpc-write-fd (%v) must be positive", rpcReadFD, rpcWriteFD)
			}
			if rpcReadFD == rpcWriteFD {
				return xerrors.Errorf("rpc-read-fd (%v) and rpc-write-fd (%v) must be different", rpcReadFD, rpcWriteFD)
			}

			pipe, err := vpn.NewBidirectionalPipe(uintptr(rpcReadFD), uintptr(rpcWriteFD))
			if err != nil {
				return xerrors.Errorf("create bidirectional RPC pipe: %w", err)
			}
			defer pipe.Close()

			tunnel, err := vpn.NewTunnel(ctx, slog.Make().Leveled(slog.LevelDebug), pipe,
				vpn.NewClient(),
				vpn.UseOSNetworkingStack(),
				vpn.UseAsLogger(),
			)
			if err != nil {
				return xerrors.Errorf("create new tunnel for client: %w", err)
			}
			defer tunnel.Close()

			<-ctx.Done()
			return nil
		},
	}

	return cmd
}
