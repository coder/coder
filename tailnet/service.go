package tailnet

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"storj.io/drpc/drpcerr"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/tailnet/proto"

	"golang.org/x/xerrors"
)

type streamIDContextKey struct{}

// StreamID identifies the caller of the CoordinateTailnet RPC.  We store this
// on the context, since the information is extracted at the HTTP layer for
// remote clients of the API, or set outside tailnet for local clients (e.g.
// Coderd's single_tailnet)
type StreamID struct {
	Name string
	ID   uuid.UUID
	Auth CoordinateeAuth
}

func WithStreamID(ctx context.Context, streamID StreamID) context.Context {
	return context.WithValue(ctx, streamIDContextKey{}, streamID)
}

// ClientService is a tailnet coordination service that accepts a connection and version from a
// tailnet client, and support versions 1.0 and 2.x of the Tailnet API protocol.
type ClientService struct {
	Logger   slog.Logger
	CoordPtr *atomic.Pointer[Coordinator]
	drpc     *drpcserver.Server
}

// NewClientService returns a ClientService based on the given Coordinator pointer.  The pointer is
// loaded on each processed connection.
func NewClientService(
	logger slog.Logger,
	coordPtr *atomic.Pointer[Coordinator],
	derpMapUpdateFrequency time.Duration,
	derpMapFn func() *tailcfg.DERPMap,
) (
	*ClientService, error,
) {
	s := &ClientService{Logger: logger, CoordPtr: coordPtr}
	mux := drpcmux.New()
	drpcService := &DRPCService{
		CoordPtr:               coordPtr,
		Logger:                 logger,
		DerpMapUpdateFrequency: derpMapUpdateFrequency,
		DerpMapFn:              derpMapFn,
	}
	err := proto.DRPCRegisterTailnet(mux, drpcService)
	if err != nil {
		return nil, xerrors.Errorf("register DRPC service: %w", err)
	}
	server := drpcserver.NewWithOptions(mux, drpcserver.Options{
		Log: func(err error) {
			if xerrors.Is(err, io.EOF) ||
				xerrors.Is(err, context.Canceled) ||
				xerrors.Is(err, context.DeadlineExceeded) {
				return
			}
			logger.Debug(context.Background(), "drpc server error", slog.Error(err))
		},
	})
	s.drpc = server
	return s, nil
}

func (s *ClientService) ServeClient(ctx context.Context, version string, conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	major, _, err := apiversion.Parse(version)
	if err != nil {
		s.Logger.Warn(ctx, "serve client called with unparsable version", slog.Error(err))
		return err
	}
	switch major {
	case 1:
		coord := *(s.CoordPtr.Load())
		return coord.ServeClient(conn, id, agent)
	case 2:
		auth := ClientCoordinateeAuth{AgentID: agent}
		streamID := StreamID{
			Name: "client",
			ID:   id,
			Auth: auth,
		}
		return s.ServeConnV2(ctx, conn, streamID)
	default:
		s.Logger.Warn(ctx, "serve client called with unsupported version", slog.F("version", version))
		return xerrors.New("unsupported version")
	}
}

func (s ClientService) ServeConnV2(ctx context.Context, conn net.Conn, streamID StreamID) error {
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Server(conn, config)
	if err != nil {
		return xerrors.Errorf("yamux init failed: %w", err)
	}
	ctx = WithStreamID(ctx, streamID)
	return s.drpc.Serve(ctx, session)
}

// DRPCService is the dRPC-based, version 2.x of the tailnet API and implements proto.DRPCClientServer
type DRPCService struct {
	CoordPtr               *atomic.Pointer[Coordinator]
	Logger                 slog.Logger
	DerpMapUpdateFrequency time.Duration
	DerpMapFn              func() *tailcfg.DERPMap
}

func (*DRPCService) PostTelemetry(context.Context, *proto.TelemetryRequest) (*proto.TelemetryResponse, error) {
	return nil, drpcerr.WithCode(xerrors.New("Unimplemented"), drpcerr.Unimplemented)
}

func (s *DRPCService) StreamDERPMaps(_ *proto.StreamDERPMapsRequest, stream proto.DRPCTailnet_StreamDERPMapsStream) error {
	defer stream.Close()

	ticker := time.NewTicker(s.DerpMapUpdateFrequency)
	defer ticker.Stop()

	var lastDERPMap *tailcfg.DERPMap
	for {
		derpMap := s.DerpMapFn()
		if derpMap == nil {
			// in testing, we send nil to close the stream.
			return io.EOF
		}
		if lastDERPMap == nil || !CompareDERPMaps(lastDERPMap, derpMap) {
			protoDERPMap := DERPMapToProto(derpMap)
			err := stream.Send(protoDERPMap)
			if err != nil {
				return xerrors.Errorf("send derp map: %w", err)
			}
			lastDERPMap = derpMap
		}

		ticker.Reset(s.DerpMapUpdateFrequency)
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *DRPCService) Coordinate(stream proto.DRPCTailnet_CoordinateStream) error {
	ctx := stream.Context()
	streamID, ok := ctx.Value(streamIDContextKey{}).(StreamID)
	if !ok {
		_ = stream.Close()
		return xerrors.New("no Stream ID")
	}
	logger := s.Logger.With(slog.F("peer_id", streamID), slog.F("name", streamID.Name))
	logger.Debug(ctx, "starting tailnet Coordinate")
	coord := *(s.CoordPtr.Load())
	reqs, resps := coord.Coordinate(ctx, streamID.ID, streamID.Name, streamID.Auth)
	c := communicator{
		logger: logger,
		stream: stream,
		reqs:   reqs,
		resps:  resps,
	}
	c.communicate()
	return nil
}

type communicator struct {
	logger slog.Logger
	stream proto.DRPCTailnet_CoordinateStream
	reqs   chan<- *proto.CoordinateRequest
	resps  <-chan *proto.CoordinateResponse
}

func (c communicator) communicate() {
	go c.loopReq()
	c.loopResp()
}

func (c communicator) loopReq() {
	ctx := c.stream.Context()
	defer close(c.reqs)
	for {
		req, err := c.stream.Recv()
		if err != nil {
			c.logger.Debug(ctx, "error receiving requests from DRPC stream", slog.Error(err))
			return
		}
		err = SendCtx(ctx, c.reqs, req)
		if err != nil {
			c.logger.Debug(ctx, "context done while sending coordinate request", slog.Error(ctx.Err()))
			return
		}
	}
}

func (c communicator) loopResp() {
	ctx := c.stream.Context()
	defer func() {
		err := c.stream.Close()
		if err != nil {
			c.logger.Debug(ctx, "loopResp hit error closing stream", slog.Error(err))
		}
	}()
	for {
		resp, err := RecvCtx(ctx, c.resps)
		if err != nil {
			c.logger.Debug(ctx, "loopResp failed to get response", slog.Error(err))
			return
		}
		err = c.stream.Send(resp)
		if err != nil {
			c.logger.Debug(ctx, "loopResp failed to send response to DRPC stream", slog.Error(err))
			return
		}
	}
}
