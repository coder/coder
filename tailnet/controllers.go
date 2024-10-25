package tailnet

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"storj.io/drpc"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet/proto"
)

// A Controller connects to the tailnet control plane, and then uses the control protocols to
// program a tailnet.Conn in production (in test it could be an interface simulating the Conn). It
// delegates this task to sub-controllers responsible for the main areas of the tailnet control
// protocol: coordination, DERP map updates, resume tokens, and telemetry.
type Controller struct {
	Dialer          ControlProtocolDialer
	CoordCtrl       CoordinationController
	DERPCtrl        DERPController
	ResumeTokenCtrl ResumeTokenController
	TelemetryCtrl   TelemetryController
}

type CloserWaiter interface {
	Close(context.Context) error
	Wait() <-chan error
}

// CoordinatorClient is an abstraction of the Coordinator's control protocol interface from the
// perspective of a protocol client (i.e. the Coder Agent is also a client of this interface).
type CoordinatorClient interface {
	Close() error
	Send(*proto.CoordinateRequest) error
	Recv() (*proto.CoordinateResponse, error)
}

// A CoordinationController accepts connections to the control plane, and handles the Coordination
// protocol on behalf of some Coordinatee (tailnet.Conn in production).  This is the "glue" code
// between them.
type CoordinationController interface {
	New(CoordinatorClient) CloserWaiter
}

// DERPClient is an abstraction of the stream of DERPMap updates from the control plane.
type DERPClient interface {
	Close() error
	Recv() (*tailcfg.DERPMap, error)
}

// A DERPController accepts connections to the control plane, and handles the DERPMap updates
// delivered over them by programming the data plane (tailnet.Conn or some test interface).
type DERPController interface {
	New(DERPClient) CloserWaiter
}

type ResumeTokenClient interface {
	RefreshResumeToken(ctx context.Context, in *proto.RefreshResumeTokenRequest) (*proto.RefreshResumeTokenResponse, error)
}

type ResumeTokenController interface {
	New(ResumeTokenClient) CloserWaiter
	Token() (string, bool)
}

type TelemetryClient interface {
	PostTelemetry(ctx context.Context, in *proto.TelemetryRequest) (*proto.TelemetryResponse, error)
}

type TelemetryController interface {
	New(TelemetryClient)
}

// ControlProtocolClients represents an abstract interface to the tailnet control plane via a set
// of protocol clients. The Closer should close all the clients (e.g. by closing the underlying
// connection).
type ControlProtocolClients struct {
	Closer      io.Closer
	Coordinator CoordinatorClient
	DERP        DERPClient
	ResumeToken ResumeTokenClient
	Telemetry   TelemetryClient
}

type ControlProtocolDialer interface {
	// Dial connects to the tailnet control plane and returns clients for the different control
	// sub-protocols (coordination, DERP maps, resume tokens, and telemetry).  If the
	// ResumeTokenController is not nil, the dialer should query for a resume token and use it to
	// dial, if available.
	Dial(ctx context.Context, r ResumeTokenController) (ControlProtocolClients, error)
}

// basicCoordinationController handles the basic coordination operations common to all types of
// tailnet consumers:
//
//  1. sending local node updates to the Coordinator
//  2. receiving peer node updates and programming them into the Coordinatee (e.g. tailnet.Conn)
//  3. (optionally) sending ReadyToHandshake acknowledgements for peer updates.
type basicCoordinationController struct {
	logger      slog.Logger
	coordinatee Coordinatee
	sendAcks    bool
}

func (c *basicCoordinationController) New(client CoordinatorClient) CloserWaiter {
	b := &basicCoordination{
		logger:       c.logger,
		errChan:      make(chan error, 1),
		coordinatee:  c.coordinatee,
		client:       client,
		respLoopDone: make(chan struct{}),
		sendAcks:     c.sendAcks,
	}

	c.coordinatee.SetNodeCallback(func(node *Node) {
		pn, err := NodeToProto(node)
		if err != nil {
			b.logger.Critical(context.Background(), "failed to convert node", slog.Error(err))
			b.sendErr(err)
			return
		}
		b.Lock()
		defer b.Unlock()
		if b.closed {
			b.logger.Debug(context.Background(), "ignored node update because coordination is closed")
			return
		}
		err = b.client.Send(&proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: pn}})
		if err != nil {
			b.sendErr(xerrors.Errorf("write: %w", err))
		}
	})
	go b.respLoop()

	return b
}

type basicCoordination struct {
	sync.Mutex
	closed       bool
	errChan      chan error
	coordinatee  Coordinatee
	logger       slog.Logger
	client       CoordinatorClient
	respLoopDone chan struct{}
	sendAcks     bool
}

func (c *basicCoordination) Close(ctx context.Context) (retErr error) {
	c.Lock()
	defer c.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	defer func() {
		// We shouldn't just close the protocol right away, because the way dRPC streams work is
		// that if you close them, that could take effect immediately, even before the Disconnect
		// message is processed. Coordinators are supposed to hang up on us once they get a
		// Disconnect message, so we should wait around for that until the context expires.
		select {
		case <-c.respLoopDone:
			c.logger.Debug(ctx, "responses closed after disconnect")
			return
		case <-ctx.Done():
			c.logger.Warn(ctx, "context expired while waiting for coordinate responses to close")
		}
		// forcefully close the stream
		protoErr := c.client.Close()
		<-c.respLoopDone
		if retErr == nil {
			retErr = protoErr
		}
	}()
	err := c.client.Send(&proto.CoordinateRequest{Disconnect: &proto.CoordinateRequest_Disconnect{}})
	if err != nil && !xerrors.Is(err, io.EOF) {
		// Coordinator RPC hangs up when it gets disconnect, so EOF is expected.
		return xerrors.Errorf("send disconnect: %w", err)
	}
	c.logger.Debug(context.Background(), "sent disconnect")
	return nil
}

func (c *basicCoordination) Wait() <-chan error {
	return c.errChan
}

func (c *basicCoordination) sendErr(err error) {
	select {
	case c.errChan <- err:
	default:
	}
}

func (c *basicCoordination) respLoop() {
	defer func() {
		cErr := c.client.Close()
		if cErr != nil {
			c.logger.Debug(context.Background(), "failed to close coordinate client after respLoop exit", slog.Error(cErr))
		}
		c.coordinatee.SetAllPeersLost()
		close(c.respLoopDone)
	}()
	for {
		resp, err := c.client.Recv()
		if err != nil {
			c.logger.Debug(context.Background(), "failed to read from protocol", slog.Error(err))
			c.sendErr(xerrors.Errorf("read: %w", err))
			return
		}

		err = c.coordinatee.UpdatePeers(resp.GetPeerUpdates())
		if err != nil {
			c.logger.Debug(context.Background(), "failed to update peers", slog.Error(err))
			c.sendErr(xerrors.Errorf("update peers: %w", err))
			return
		}

		// Only send ReadyForHandshake acks from peers without a target.
		if c.sendAcks {
			// Send an ack back for all received peers. This could
			// potentially be smarter to only send an ACK once per client,
			// but there's nothing currently stopping clients from reusing
			// IDs.
			rfh := []*proto.CoordinateRequest_ReadyForHandshake{}
			for _, peer := range resp.GetPeerUpdates() {
				if peer.Kind != proto.CoordinateResponse_PeerUpdate_NODE {
					continue
				}

				rfh = append(rfh, &proto.CoordinateRequest_ReadyForHandshake{Id: peer.Id})
			}
			if len(rfh) > 0 {
				err := c.client.Send(&proto.CoordinateRequest{
					ReadyForHandshake: rfh,
				})
				if err != nil {
					c.logger.Debug(context.Background(), "failed to send ready for handshake", slog.Error(err))
					c.sendErr(xerrors.Errorf("send: %w", err))
					return
				}
			}
		}
	}
}

type singleDestController struct {
	*basicCoordinationController
	dest uuid.UUID
}

// NewSingleDestController creates a CoordinationController for Coder clients that connect to a
// single tunnel destination, e.g. `coder ssh`, which connects to a single workspace Agent.
func NewSingleDestController(logger slog.Logger, coordinatee Coordinatee, dest uuid.UUID) CoordinationController {
	coordinatee.SetTunnelDestination(dest)
	return &singleDestController{
		basicCoordinationController: &basicCoordinationController{
			logger:      logger,
			coordinatee: coordinatee,
			sendAcks:    false,
		},
		dest: dest,
	}
}

func (c *singleDestController) New(client CoordinatorClient) CloserWaiter {
	// nolint: forcetypeassert
	b := c.basicCoordinationController.New(client).(*basicCoordination)
	err := client.Send(&proto.CoordinateRequest{AddTunnel: &proto.CoordinateRequest_Tunnel{Id: c.dest[:]}})
	if err != nil {
		b.sendErr(err)
	}
	return b
}

// NewAgentCoordinationController creates a CoordinationController for Coder Agents, which never
// create tunnels and always send ReadyToHandshake acknowledgements.
func NewAgentCoordinationController(logger slog.Logger, coordinatee Coordinatee) CoordinationController {
	return &basicCoordinationController{
		logger:      logger,
		coordinatee: coordinatee,
		sendAcks:    true,
	}
}

type inMemoryCoordClient struct {
	sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
	logger slog.Logger
	resps  <-chan *proto.CoordinateResponse
	reqs   chan<- *proto.CoordinateRequest
}

func (c *inMemoryCoordClient) Close() error {
	c.cancel()
	c.Lock()
	defer c.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	close(c.reqs)
	return nil
}

func (c *inMemoryCoordClient) Send(request *proto.CoordinateRequest) error {
	c.Lock()
	defer c.Unlock()
	if c.closed {
		return drpc.ClosedError.New("in-memory coordinator client closed")
	}
	select {
	case c.reqs <- request:
		return nil
	case <-c.ctx.Done():
		return drpc.ClosedError.New("in-memory coordinator client closed")
	}
}

func (c *inMemoryCoordClient) Recv() (*proto.CoordinateResponse, error) {
	select {
	case resp, ok := <-c.resps:
		if ok {
			return resp, nil
		}
		// response from Coordinator was closed, so close the send direction as well, so that the
		// Coordinator won't be waiting for us while shutting down.
		_ = c.Close()
		return nil, io.EOF
	case <-c.ctx.Done():
		return nil, drpc.ClosedError.New("in-memory coord client closed")
	}
}

// NewInMemoryCoordinatorClient creates a coordination client that uses channels to connect to a
// local Coordinator. (The typical alternative is a DRPC-based client.)
func NewInMemoryCoordinatorClient(
	logger slog.Logger,
	clientID, agentID uuid.UUID,
	coordinator Coordinator,
) CoordinatorClient {
	logger = logger.With(slog.F("agent_id", agentID), slog.F("client_id", clientID))
	auth := ClientCoordinateeAuth{AgentID: agentID}
	c := &inMemoryCoordClient{logger: logger}
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// use the background context since we will depend exclusively on closing the req channel to
	// tell the coordinator we are done.
	c.reqs, c.resps = coordinator.Coordinate(context.Background(),
		clientID, fmt.Sprintf("inmemory%s", clientID),
		auth,
	)
	return c
}
