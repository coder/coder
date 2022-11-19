package provisionersdk

import (
	"context"
	"errors"
	"io"
	"net"
	"os"

	"github.com/hashicorp/yamux"
	"github.com/valyala/fasthttp/fasthttputil"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"github.com/coder/coder/provisionersdk/proto"
)

// ServeOptions are configurations to serve a provisioner.
type ServeOptions struct {
	// Conn specifies a custom transport to serve the dRPC connection.
	Listener net.Listener
}

// Serve starts a dRPC connection for the provisioner and transport provided.
func Serve(ctx context.Context, server proto.DRPCProvisionerServer, options *ServeOptions) error {
	if options == nil {
		options = &ServeOptions{}
	}
	// Default to using stdio.
	if options.Listener == nil {
		config := yamux.DefaultConfig()
		config.LogOutput = io.Discard
		stdio, err := yamux.Server(&readWriteCloser{
			ReadCloser: os.Stdin,
			Writer:     os.Stdout,
		}, config)
		if err != nil {
			return xerrors.Errorf("create yamux: %w", err)
		}
		go func() {
			<-ctx.Done()
			_ = stdio.Close()
		}()
		options.Listener = stdio
	}

	// dRPC is a drop-in replacement for gRPC with less generated code, and faster transports.
	// See: https://www.storj.io/blog/introducing-drpc-our-replacement-for-grpc
	mux := drpcmux.New()
	err := proto.DRPCRegisterProvisioner(mux, server)
	if err != nil {
		return xerrors.Errorf("register provisioner: %w", err)
	}
	srv := drpcserver.New(mux)
	// Only serve a single connection on the transport.
	// Transports are not multiplexed, and provisioners are
	// short-lived processes that can be executed concurrently.
	err = srv.Serve(ctx, options.Listener)
	if err != nil {
		if errors.Is(err, io.EOF) ||
			errors.Is(err, context.Canceled) ||
			errors.Is(err, io.ErrClosedPipe) ||
			errors.Is(err, yamux.ErrSessionShutdown) ||
			errors.Is(err, fasthttputil.ErrInmemoryListenerClosed) {
			return nil
		}

		return xerrors.Errorf("serve transport: %w", err)
	}
	return nil
}

type readWriteCloser struct {
	io.ReadCloser
	io.Writer
}
