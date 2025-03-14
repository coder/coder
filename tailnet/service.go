package tailnet

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
)

var ErrUnsupportedVersion = xerrors.New("unsupported version")

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

type WorkspaceUpdatesProvider interface {
	io.Closer
	Subscribe(ctx context.Context, userID uuid.UUID) (Subscription, error)
}

type Subscription interface {
	io.Closer
	Updates() <-chan *proto.WorkspaceUpdate
}

type TunnelAuthorizer interface {
	AuthorizeTunnel(ctx context.Context, agentID uuid.UUID) error
}

type ClientServiceOptions struct {
	Logger                   slog.Logger
	CoordPtr                 *atomic.Pointer[Coordinator]
	DERPMapUpdateFrequency   time.Duration
	DERPMapFn                func() *tailcfg.DERPMap
	NetworkTelemetryHandler  func(batch []*proto.TelemetryEvent)
	ResumeTokenProvider      ResumeTokenProvider
	WorkspaceUpdatesProvider WorkspaceUpdatesProvider
}

// ClientService is a tailnet coordination service that accepts a connection and version from a
// tailnet client, and support versions 2.x of the Tailnet API protocol.
type ClientService struct {
	Logger   slog.Logger
	CoordPtr *atomic.Pointer[Coordinator]
	drpc     *drpcserver.Server
}

// NewClientService returns a ClientService based on the given Coordinator pointer.  The pointer is
// loaded on each processed connection.
func NewClientService(options ClientServiceOptions) (
	*ClientService, error,
) {
	s := &ClientService{Logger: options.Logger, CoordPtr: options.CoordPtr}
	mux := drpcmux.New()
	drpcService := &DRPCService{
		CoordPtr:                 options.CoordPtr,
		Logger:                   options.Logger,
		DerpMapUpdateFrequency:   options.DERPMapUpdateFrequency,
		DerpMapFn:                options.DERPMapFn,
		NetworkTelemetryHandler:  options.NetworkTelemetryHandler,
		ResumeTokenProvider:      options.ResumeTokenProvider,
		WorkspaceUpdatesProvider: options.WorkspaceUpdatesProvider,
	}
	err := proto.DRPCRegisterTailnet(mux, drpcService)
	if err != nil {
		return nil, xerrors.Errorf("register DRPC service: %w", err)
	}
	server := drpcserver.NewWithOptions(mux, drpcserver.Options{
		Log: func(err error) {
			if errors.Is(err, io.EOF) ||
				errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				return
			}
			options.Logger.Debug(context.Background(), "drpc server error", slog.Error(err))
		},
	})
	s.drpc = server
	return s, nil
}

func (s *ClientService) ServeClient(ctx context.Context, version string, conn net.Conn, streamID StreamID) error {
	major, _, err := apiversion.Parse(version)
	if err != nil {
		s.Logger.Warn(ctx, "serve client called with unparsable version", slog.Error(err))
		return err
	}
	switch major {
	case 2:
		return s.ServeConnV2(ctx, conn, streamID)
	default:
		s.Logger.Warn(ctx, "serve client called with unsupported version", slog.F("version", version))
		return ErrUnsupportedVersion
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
	s.Logger.Debug(ctx, "serving dRPC tailnet v2 API session",
		slog.F("peer_id", streamID.ID.String()))
	return s.drpc.Serve(ctx, session)
}

// DRPCService is the dRPC-based, version 2.x of the tailnet API and implements proto.DRPCClientServer
type DRPCService struct {
	CoordPtr                 *atomic.Pointer[Coordinator]
	Logger                   slog.Logger
	DerpMapUpdateFrequency   time.Duration
	DerpMapFn                func() *tailcfg.DERPMap
	NetworkTelemetryHandler  func(batch []*proto.TelemetryEvent)
	ResumeTokenProvider      ResumeTokenProvider
	WorkspaceUpdatesProvider WorkspaceUpdatesProvider
}

func (s *DRPCService) PostTelemetry(_ context.Context, req *proto.TelemetryRequest) (*proto.TelemetryResponse, error) {
	if s.NetworkTelemetryHandler != nil {
		s.NetworkTelemetryHandler(req.Events)
	}
	return &proto.TelemetryResponse{}, nil
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

func (s *DRPCService) RefreshResumeToken(ctx context.Context, _ *proto.RefreshResumeTokenRequest) (*proto.RefreshResumeTokenResponse, error) {
	streamID, ok := ctx.Value(streamIDContextKey{}).(StreamID)
	if !ok {
		return nil, xerrors.New("no Stream ID")
	}

	res, err := s.ResumeTokenProvider.GenerateResumeToken(ctx, streamID.ID)
	if err != nil {
		return nil, xerrors.Errorf("generate resume token: %w", err)
	}
	return res, nil
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

func (s *DRPCService) WorkspaceUpdates(req *proto.WorkspaceUpdatesRequest, stream proto.DRPCTailnet_WorkspaceUpdatesStream) error {
	defer stream.Close()

	ctx := stream.Context()

	ownerID, err := uuid.FromBytes(req.WorkspaceOwnerId)
	if err != nil {
		return xerrors.Errorf("parse workspace owner ID: %w", err)
	}

	sub, err := s.WorkspaceUpdatesProvider.Subscribe(ctx, ownerID)
	if err != nil {
		return xerrors.Errorf("subscribe to workspace updates: %w", err)
	}
	defer sub.Close()

	for {
		select {
		case updates, ok := <-sub.Updates():
			if !ok {
				return nil
			}
			err := stream.Send(updates)
			if err != nil {
				return xerrors.Errorf("send workspace update: %w", err)
			}
		case <-stream.Context().Done():
			return nil
		}
	}
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

type NetworkTelemetryBatcher struct {
	clock     quartz.Clock
	frequency time.Duration
	maxSize   int
	batchFn   func(batch []*proto.TelemetryEvent)

	mu      sync.Mutex
	closed  chan struct{}
	done    chan struct{}
	ticker  *quartz.Ticker
	pending []*proto.TelemetryEvent
}

func NewNetworkTelemetryBatcher(clk quartz.Clock, frequency time.Duration, maxSize int, batchFn func(batch []*proto.TelemetryEvent)) *NetworkTelemetryBatcher {
	b := &NetworkTelemetryBatcher{
		clock:     clk,
		frequency: frequency,
		maxSize:   maxSize,
		batchFn:   batchFn,
		closed:    make(chan struct{}),
		done:      make(chan struct{}),
	}
	if b.batchFn == nil {
		b.batchFn = func(batch []*proto.TelemetryEvent) {}
	}
	b.start()
	return b
}

func (b *NetworkTelemetryBatcher) Close() error {
	close(b.closed)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		return xerrors.New("timed out waiting for batcher to close")
	case <-b.done:
	}
	return nil
}

func (b *NetworkTelemetryBatcher) sendTelemetryBatch() {
	b.mu.Lock()
	defer b.mu.Unlock()
	events := b.pending
	if len(events) == 0 {
		return
	}
	b.pending = []*proto.TelemetryEvent{}
	b.batchFn(events)
}

func (b *NetworkTelemetryBatcher) start() {
	b.ticker = b.clock.NewTicker(b.frequency)

	go func() {
		defer func() {
			// The lock prevents Handler from racing with Close.
			b.mu.Lock()
			defer b.mu.Unlock()
			close(b.done)
			b.ticker.Stop()
		}()

		for {
			select {
			case <-b.ticker.C:
				b.sendTelemetryBatch()
				b.ticker.Reset(b.frequency)
			case <-b.closed:
				// Send any remaining telemetry events before exiting.
				b.sendTelemetryBatch()
				return
			}
		}
	}()
}

func (b *NetworkTelemetryBatcher) Handler(events []*proto.TelemetryEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.closed:
		return
	default:
	}

	for _, event := range events {
		b.pending = append(b.pending, event)

		if len(b.pending) >= b.maxSize {
			// This can't call sendTelemetryBatch directly because we already
			// hold the lock.
			events := b.pending
			b.pending = []*proto.TelemetryEvent{}
			// Resetting the ticker is best effort. We don't care if the ticker
			// has already fired or has a pending message, because the only risk
			// is that we send two telemetry events in short succession (which
			// is totally fine).
			b.ticker.Reset(b.frequency)
			// Perform the send in a goroutine to avoid blocking the DRPC call.
			go b.batchFn(events)
		}
	}
}
