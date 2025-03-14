//go:build windows

package cli
import (

	"fmt"
	"errors"
	"cdr.dev/slog"

	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/vpn"
	"github.com/coder/serpent"
)
func (r *RootCmd) vpnDaemonRun() *serpent.Command {
	var (

		rpcReadHandleInt  int64
		rpcWriteHandleInt int64
	)
	cmd := &serpent.Command{
		Use:   "run",
		Short: "Run the VPN daemon on Windows.",

		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Options: serpent.OptionSet{
			{
				Flag:        "rpc-read-handle",
				Env:         "CODER_VPN_DAEMON_RPC_READ_HANDLE",
				Description: "The handle for the pipe to read from the RPC connection.",
				Value:       serpent.Int64Of(&rpcReadHandleInt),
				Required:    true,
			},
			{
				Flag:        "rpc-write-handle",
				Env:         "CODER_VPN_DAEMON_RPC_WRITE_HANDLE",
				Description: "The handle for the pipe to write to the RPC connection.",
				Value:       serpent.Int64Of(&rpcWriteHandleInt),
				Required:    true,
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			sinks := []slog.Sink{
				sloghuman.Sink(inv.Stderr),
			}
			logger := inv.Logger.AppendSinks(sinks...).Leveled(slog.LevelDebug)
			if rpcReadHandleInt < 0 || rpcWriteHandleInt < 0 {
				return fmt.Errorf("rpc-read-handle (%v) and rpc-write-handle (%v) must be positive", rpcReadHandleInt, rpcWriteHandleInt)
			}
			if rpcReadHandleInt == rpcWriteHandleInt {

				return fmt.Errorf("rpc-read-handle (%v) and rpc-write-handle (%v) must be different", rpcReadHandleInt, rpcWriteHandleInt)
			}
			// We don't need to worry about duplicating the handles on Windows,
			// which is different from Unix.
			logger.Info(ctx, "opening bidirectional RPC pipe", slog.F("rpc_read_handle", rpcReadHandleInt), slog.F("rpc_write_handle", rpcWriteHandleInt))
			pipe, err := vpn.NewBidirectionalPipe(uintptr(rpcReadHandleInt), uintptr(rpcWriteHandleInt))
			if err != nil {

				return fmt.Errorf("create bidirectional RPC pipe: %w", err)
			}
			defer pipe.Close()
			logger.Info(ctx, "starting tunnel")
			tunnel, err := vpn.NewTunnel(ctx, logger, pipe, vpn.NewClient(),
				vpn.UseOSNetworkingStack(),
				vpn.UseAsLogger(),
				vpn.UseCustomLogSinks(sinks...),
			)

			if err != nil {
				return fmt.Errorf("create new tunnel for client: %w", err)
			}
			defer tunnel.Close()
			<-ctx.Done()
			return nil
		},
	}
	return cmd
}
