package provisionersdk

import (
	"context"
	"errors"
	"io"

	"golang.org/x/xerrors"
	"storj.io/drpc"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"github.com/coder/coder/provisionersdk/proto"
)

// ServeOptions are configurations to serve a provisioner.
type ServeOptions struct {
	// Transport specifies a custom transport to serve the dRPC connection.
	Transport drpc.Transport
}

// Serve starts a dRPC connection for the provisioner and transport provided.
func Serve(ctx context.Context, server proto.DRPCProvisionerServer, options *ServeOptions) error {
	if options == nil {
		options = &ServeOptions{}
	}
	// Default to using stdio.
	if options.Transport == nil {
		options.Transport = TransportStdio()
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
	err = srv.ServeOne(ctx, options.Transport)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if errors.Is(err, io.ErrClosedPipe) {
			// This may occur if the transport on either end is
			// closed before the context. It's fine to return
			// nil here, since the server has nothing to
			// communicate with.
			return nil
		}
		return xerrors.Errorf("serve transport: %w", err)
	}
	return nil
}
