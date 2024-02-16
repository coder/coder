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
	"storj.io/drpc"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

const (
	CurrentMajor = 1
	CurrentMinor = 0
)

// VersionCurrent is the current provisionerd API version.
// Breaking changes to the provisionerd API **MUST** increment
// CurrentMajor above.
var VersionCurrent = apiversion.New(CurrentMajor, CurrentMinor)

// ServeOptions are configurations to serve a provisioner.
type ServeOptions struct {
	// Listener serves multiple connections. Cannot be combined with Conn.
	Listener net.Listener
	// Conn is a single connection to serve. Cannot be combined with Listener.
	Conn          drpc.Transport
	Logger        slog.Logger
	WorkDirectory string
}

type Server interface {
	Parse(s *Session, r *proto.ParseRequest, canceledOrComplete <-chan struct{}) *proto.ParseComplete
	Plan(s *Session, r *proto.PlanRequest, canceledOrComplete <-chan struct{}) *proto.PlanComplete
	Apply(s *Session, r *proto.ApplyRequest, canceledOrComplete <-chan struct{}) *proto.ApplyComplete
}

// Serve starts a dRPC connection for the provisioner and transport provided.
func Serve(ctx context.Context, server Server, options *ServeOptions) error {
	if options == nil {
		options = &ServeOptions{}
	}
	if options.Listener != nil && options.Conn != nil {
		return xerrors.New("specify Listener or Conn, not both")
	}
	// Default to using stdio with yamux as a Listener
	if options.Listener == nil && options.Conn == nil {
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
	if options.WorkDirectory == "" {
		var err error
		options.WorkDirectory, err = os.MkdirTemp("", "coderprovisioner")
		if err != nil {
			return xerrors.Errorf("failed to init temp work dir: %w", err)
		}
	}

	// dRPC is a drop-in replacement for gRPC with less generated code, and faster transports.
	// See: https://www.storj.io/blog/introducing-drpc-our-replacement-for-grpc
	mux := drpcmux.New()
	ps := &protoServer{
		server: server,
		opts:   *options,
	}
	err := proto.DRPCRegisterProvisioner(mux, ps)
	if err != nil {
		return xerrors.Errorf("register provisioner: %w", err)
	}
	srv := drpcserver.New(&tracing.DRPCHandler{Handler: mux})

	if options.Listener != nil {
		err = srv.Serve(ctx, options.Listener)
	} else if options.Conn != nil {
		err = srv.ServeOne(ctx, options.Conn)
	}
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
