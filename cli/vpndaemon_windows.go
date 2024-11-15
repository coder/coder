//go:build windows

package cli

import (
	"io"
	"os"

	"cdr.dev/slog"
	"golang.org/x/xerrors"

	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/vpn"
	"github.com/coder/serpent"
)

func (r *RootCmd) vpnDaemonRun() *serpent.Command {
	var (
		rpcReadHandleInt  int64
		rpcWriteHandleInt int64
		logPath           string
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
			{
				Flag:        "log-path",
				Env:         "CODER_VPN_DAEMON_LOG_PATH",
				Description: "The path to the log file to write to.",
				Value:       serpent.StringOf(&logPath),
				Required:    false, // logs will also be written to stderr
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			if rpcReadHandleInt < 0 || rpcWriteHandleInt < 0 {
				return xerrors.Errorf("rpc-read-handle (%v) and rpc-write-handle (%v) must be positive", rpcReadHandleInt, rpcWriteHandleInt)
			}
			if rpcReadHandleInt == rpcWriteHandleInt {
				return xerrors.Errorf("rpc-read-handle (%v) and rpc-write-handle (%v) must be different", rpcReadHandleInt, rpcWriteHandleInt)
			}

			logger := inv.Logger.AppendSinks(sloghuman.Sink(inv.Stderr)).Leveled(slog.LevelDebug)
			if logPath != "" {
				f, err := os.Create(logPath)
				if err != nil {
					return xerrors.Errorf("create log file: %w", err)
				}
				defer f.Close()
				logger = logger.AppendSinks(sloghuman.Sink(f))
			}

			logger.Info(ctx, "opening bidirectional RPC pipe", slog.F("rpc_read_handle", rpcReadHandleInt), slog.F("rpc_write_handle", rpcWriteHandleInt))
			pipe, err := newBidiPipe(uintptr(rpcReadHandleInt), uintptr(rpcWriteHandleInt))
			if err != nil {
				return xerrors.Errorf("create bidirectional RPC pipe: %w", err)
			}
			defer pipe.Close()

			logger.Info(ctx, "starting tunnel")
			tunnel, err := vpn.NewTunnel(ctx, logger, pipe)
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

type bidiPipe struct {
	read  *os.File
	write *os.File
}

var _ io.ReadWriteCloser = bidiPipe{}

func newBidiPipe(readHandle, writeHandle uintptr) (bidiPipe, error) {
	read := os.NewFile(readHandle, "rpc_read")
	_, err := read.Stat()
	if err != nil {
		return bidiPipe{}, xerrors.Errorf("stat rpc_read pipe (handle=%v): %w", readHandle, err)
	}
	write := os.NewFile(writeHandle, "rpc_write")
	_, err = write.Stat()
	if err != nil {
		return bidiPipe{}, xerrors.Errorf("stat rpc_write pipe (handle=%v): %w", writeHandle, err)
	}
	return bidiPipe{
		read:  read,
		write: write,
	}, nil
}

// Read implements io.Reader. Data is read from the read pipe.
func (b bidiPipe) Read(p []byte) (int, error) {
	n, err := b.read.Read(p)
	if err != nil {
		return n, xerrors.Errorf("read from rpc_read pipe (handle=%v): %w", b.read.Fd(), err)
	}
	return n, nil
}

// Write implements io.Writer. Data is written to the write pipe.
func (b bidiPipe) Write(p []byte) (n int, err error) {
	n, err = b.write.Write(p)
	if err != nil {
		return n, xerrors.Errorf("write to rpc_write pipe (handle=%v): %w", b.write.Fd(), err)
	}
	return n, nil
}

// Close implements io.Closer. Both the read and write pipes are closed.
func (b bidiPipe) Close() error {
	err := b.read.Close()
	if err != nil {
		return xerrors.Errorf("close rpc_read pipe (handle=%v): %w", b.read.Fd(), err)
	}
	err = b.write.Close()
	if err != nil {
		return xerrors.Errorf("close rpc_write pipe (handle=%v): %w", b.write.Fd(), err)
	}
	return nil
}
