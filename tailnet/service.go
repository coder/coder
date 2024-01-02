package tailnet

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet/proto"

	"golang.org/x/xerrors"
)

const (
	CurrentMajor = 2
	CurrentMinor = 0
)

var SupportedMajors = []int{2, 1}

func ValidateVersion(version string) error {
	major, minor, err := parseVersion(version)
	if err != nil {
		return err
	}
	if major > CurrentMajor {
		return xerrors.Errorf("server is at version %d.%d, behind requested version %s",
			CurrentMajor, CurrentMinor, version)
	}
	if major == CurrentMajor {
		if minor > CurrentMinor {
			return xerrors.Errorf("server is at version %d.%d, behind requested version %s",
				CurrentMajor, CurrentMinor, version)
		}
		return nil
	}
	for _, mjr := range SupportedMajors {
		if major == mjr {
			return nil
		}
	}
	return xerrors.Errorf("version %s is no longer supported", version)
}

func parseVersion(version string) (major int, minor int, err error) {
	parts := strings.Split(version, ".")
	if len(parts) != 2 {
		return 0, 0, xerrors.Errorf("invalid version string: %s", version)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, xerrors.Errorf("invalid major version: %s", version)
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, xerrors.Errorf("invalid minor version: %s", version)
	}
	return major, minor, nil
}

type streamIDContextKey struct{}

// StreamID identifies the caller of the CoordinateTailnet RPC.  We store this
// on the context, since the information is extracted at the HTTP layer for
// remote clients of the API, or set outside tailnet for local clients (e.g.
// Coderd's single_tailnet)
type StreamID struct {
	Name string
	ID   uuid.UUID
	Auth TunnelAuth
}

func WithStreamID(ctx context.Context, streamID StreamID) context.Context {
	return context.WithValue(ctx, streamIDContextKey{}, streamID)
}

// ClientService is a tailnet coordination service that accepts a connection and version from a
// tailnet client, and support versions 1.0 and 2.x of the Tailnet API protocol.
type ClientService struct {
	logger   slog.Logger
	coordPtr *atomic.Pointer[Coordinator]
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
	s := &ClientService{logger: logger, coordPtr: coordPtr}
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
			if xerrors.Is(err, io.EOF) {
				return
			}
			logger.Debug(context.Background(), "drpc server error", slog.Error(err))
		},
	})
	s.drpc = server
	return s, nil
}

func (s *ClientService) ServeClient(ctx context.Context, version string, conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	major, _, err := parseVersion(version)
	if err != nil {
		s.logger.Warn(ctx, "serve client called with unparsable version", slog.Error(err))
		return err
	}
	switch major {
	case 1:
		coord := *(s.coordPtr.Load())
		return coord.ServeClient(conn, id, agent)
	case 2:
		config := yamux.DefaultConfig()
		config.LogOutput = io.Discard
		session, err := yamux.Server(conn, config)
		if err != nil {
			return xerrors.Errorf("yamux init failed: %w", err)
		}
		auth := ClientTunnelAuth{AgentID: agent}
		streamID := StreamID{
			Name: "client",
			ID:   id,
			Auth: auth,
		}
		ctx = WithStreamID(ctx, streamID)
		return s.drpc.Serve(ctx, session)
	default:
		s.logger.Warn(ctx, "serve client called with unsupported version", slog.F("version", version))
		return xerrors.New("unsupported version")
	}
}

// DRPCService is the dRPC-based, version 2.x of the tailnet API and implements proto.DRPCClientServer
type DRPCService struct {
	CoordPtr               *atomic.Pointer[Coordinator]
	Logger                 slog.Logger
	DerpMapUpdateFrequency time.Duration
	DerpMapFn              func() *tailcfg.DERPMap
}

func (s *DRPCService) StreamDERPMaps(_ *proto.StreamDERPMapsRequest, stream proto.DRPCTailnet_StreamDERPMapsStream) error {
	defer stream.Close()

	ticker := time.NewTicker(s.DerpMapUpdateFrequency)
	defer ticker.Stop()

	var lastDERPMap *tailcfg.DERPMap
	for {
		derpMap := s.DerpMapFn()
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
