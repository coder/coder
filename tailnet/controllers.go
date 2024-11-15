package tailnet

import (
	"context"
	"fmt"
	"io"
	"maps"
	"math"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"storj.io/drpc"
	"storj.io/drpc/drpcerr"
	"tailscale.com/tailcfg"
	"tailscale.com/util/dnsname"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
	"github.com/coder/retry"
)

// A Controller connects to the tailnet control plane, and then uses the control protocols to
// program a tailnet.Conn in production (in test it could be an interface simulating the Conn). It
// delegates this task to sub-controllers responsible for the main areas of the tailnet control
// protocol: coordination, DERP map updates, resume tokens, telemetry, and workspace updates.
type Controller struct {
	Dialer               ControlProtocolDialer
	CoordCtrl            CoordinationController
	DERPCtrl             DERPController
	ResumeTokenCtrl      ResumeTokenController
	TelemetryCtrl        TelemetryController
	WorkspaceUpdatesCtrl WorkspaceUpdatesController

	ctx               context.Context
	gracefulCtx       context.Context
	cancelGracefulCtx context.CancelFunc
	logger            slog.Logger
	closedCh          chan struct{}

	// Testing only
	clock           quartz.Clock
	gracefulTimeout time.Duration
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

type WorkspaceUpdatesClient interface {
	Close() error
	Recv() (*proto.WorkspaceUpdate, error)
}

type WorkspaceUpdatesController interface {
	New(WorkspaceUpdatesClient) CloserWaiter
}

// DNSHostsSetter is something that you can set a mapping of DNS names to IPs on. It's the subset
// of the tailnet.Conn that we use to configure DNS records.
type DNSHostsSetter interface {
	SetDNSHosts(hosts map[dnsname.FQDN][]netip.Addr) error
}

// ControlProtocolClients represents an abstract interface to the tailnet control plane via a set
// of protocol clients. The Closer should close all the clients (e.g. by closing the underlying
// connection).
type ControlProtocolClients struct {
	Closer           io.Closer
	Coordinator      CoordinatorClient
	DERP             DERPClient
	ResumeToken      ResumeTokenClient
	Telemetry        TelemetryClient
	WorkspaceUpdates WorkspaceUpdatesClient
}

type ControlProtocolDialer interface {
	// Dial connects to the tailnet control plane and returns clients for the different control
	// sub-protocols (coordination, DERP maps, resume tokens, and telemetry).  If the
	// ResumeTokenController is not nil, the dialer should query for a resume token and use it to
	// dial, if available.
	Dial(ctx context.Context, r ResumeTokenController) (ControlProtocolClients, error)
}

// BasicCoordinationController handles the basic coordination operations common to all types of
// tailnet consumers:
//
//  1. sending local node updates to the Coordinator
//  2. receiving peer node updates and programming them into the Coordinatee (e.g. tailnet.Conn)
//  3. (optionally) sending ReadyToHandshake acknowledgements for peer updates.
//
// It is designed to be used on its own, or composed into more advanced CoordinationControllers.
type BasicCoordinationController struct {
	Logger      slog.Logger
	Coordinatee Coordinatee
	SendAcks    bool
}

// New satisfies the method on the CoordinationController interface
func (c *BasicCoordinationController) New(client CoordinatorClient) CloserWaiter {
	return c.NewCoordination(client)
}

// NewCoordination creates a BasicCoordination
func (c *BasicCoordinationController) NewCoordination(client CoordinatorClient) *BasicCoordination {
	b := &BasicCoordination{
		logger:       c.Logger,
		errChan:      make(chan error, 1),
		coordinatee:  c.Coordinatee,
		Client:       client,
		respLoopDone: make(chan struct{}),
		sendAcks:     c.SendAcks,
	}

	c.Coordinatee.SetNodeCallback(func(node *Node) {
		pn, err := NodeToProto(node)
		if err != nil {
			b.logger.Critical(context.Background(), "failed to convert node", slog.Error(err))
			b.SendErr(err)
			return
		}
		b.Lock()
		defer b.Unlock()
		if b.closed {
			b.logger.Debug(context.Background(), "ignored node update because coordination is closed")
			return
		}
		err = b.Client.Send(&proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: pn}})
		if err != nil {
			b.SendErr(xerrors.Errorf("write: %w", err))
		}
	})
	go b.respLoop()

	return b
}

// BasicCoordination handles:
//
// 1. Sending local node updates to the control plane
// 2. Reading remote updates from the control plane and programming them into the Coordinatee.
//
// It does *not* handle adding any Tunnels, but these can be handled by composing
// BasicCoordinationController with a more advanced controller.
type BasicCoordination struct {
	sync.Mutex
	closed       bool
	errChan      chan error
	coordinatee  Coordinatee
	logger       slog.Logger
	Client       CoordinatorClient
	respLoopDone chan struct{}
	sendAcks     bool
}

// Close the coordination gracefully. If the context expires before the remote API server has hung
// up on us, we forcibly close the Client connection.
func (c *BasicCoordination) Close(ctx context.Context) (retErr error) {
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
		protoErr := c.Client.Close()
		<-c.respLoopDone
		if retErr == nil {
			retErr = protoErr
		}
	}()
	err := c.Client.Send(&proto.CoordinateRequest{Disconnect: &proto.CoordinateRequest_Disconnect{}})
	if err != nil && !xerrors.Is(err, io.EOF) {
		// Coordinator RPC hangs up when it gets disconnect, so EOF is expected.
		return xerrors.Errorf("send disconnect: %w", err)
	}
	c.logger.Debug(context.Background(), "sent disconnect")
	return nil
}

// Wait for the Coordination to complete
func (c *BasicCoordination) Wait() <-chan error {
	return c.errChan
}

// SendErr is not part of the CloserWaiter interface, and is intended to be called internally, or
// by Controllers that use BasicCoordinationController in composition.  It triggers Wait() to
// report the error if an error has not already been reported.
func (c *BasicCoordination) SendErr(err error) {
	select {
	case c.errChan <- err:
	default:
	}
}

func (c *BasicCoordination) respLoop() {
	defer func() {
		cErr := c.Client.Close()
		if cErr != nil {
			c.logger.Debug(context.Background(),
				"failed to close coordinate client after respLoop exit", slog.Error(cErr))
		}
		c.coordinatee.SetAllPeersLost()
		close(c.respLoopDone)
	}()
	for {
		resp, err := c.Client.Recv()
		if err != nil {
			c.logger.Debug(context.Background(),
				"failed to read from protocol", slog.Error(err))
			c.SendErr(xerrors.Errorf("read: %w", err))
			return
		}

		err = c.coordinatee.UpdatePeers(resp.GetPeerUpdates())
		if err != nil {
			c.logger.Debug(context.Background(), "failed to update peers", slog.Error(err))
			c.SendErr(xerrors.Errorf("update peers: %w", err))
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
				err := c.Client.Send(&proto.CoordinateRequest{
					ReadyForHandshake: rfh,
				})
				if err != nil {
					c.logger.Debug(context.Background(),
						"failed to send ready for handshake", slog.Error(err))
					c.SendErr(xerrors.Errorf("send: %w", err))
					return
				}
			}
		}
	}
}

type TunnelSrcCoordController struct {
	*BasicCoordinationController

	mu           sync.Mutex
	dests        map[uuid.UUID]struct{}
	coordination *BasicCoordination
}

// NewTunnelSrcCoordController creates a CoordinationController for peers that are exclusively
// tunnel sources (that is, they create tunnel --- Coder clients not workspaces).
func NewTunnelSrcCoordController(
	logger slog.Logger, coordinatee Coordinatee,
) *TunnelSrcCoordController {
	return &TunnelSrcCoordController{
		BasicCoordinationController: &BasicCoordinationController{
			Logger:      logger,
			Coordinatee: coordinatee,
			SendAcks:    false,
		},
		dests: make(map[uuid.UUID]struct{}),
	}
}

func (c *TunnelSrcCoordController) New(client CoordinatorClient) CloserWaiter {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.BasicCoordinationController.NewCoordination(client)
	c.coordination = b
	// resync destinations on reconnect
	for dest := range c.dests {
		err := client.Send(&proto.CoordinateRequest{
			AddTunnel: &proto.CoordinateRequest_Tunnel{Id: UUIDToByteSlice(dest)},
		})
		if err != nil {
			b.SendErr(err)
			c.coordination = nil
			cErr := client.Close()
			if cErr != nil {
				c.Logger.Debug(
					context.Background(),
					"failed to close coordinator client after add tunnel failure",
					slog.Error(cErr),
				)
			}
			break
		}
	}
	return b
}

func (c *TunnelSrcCoordController) AddDestination(dest uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Coordinatee.SetTunnelDestination(dest) // this prepares us for an ack
	c.dests[dest] = struct{}{}
	if c.coordination == nil {
		return
	}
	err := c.coordination.Client.Send(
		&proto.CoordinateRequest{
			AddTunnel: &proto.CoordinateRequest_Tunnel{Id: UUIDToByteSlice(dest)},
		})
	if err != nil {
		c.coordination.SendErr(err)
		cErr := c.coordination.Client.Close() // close the client so we don't gracefully disconnect
		if cErr != nil {
			c.Logger.Debug(context.Background(),
				"failed to close coordinator client after add tunnel failure",
				slog.Error(cErr))
		}
		c.coordination = nil
	}
}

func (c *TunnelSrcCoordController) RemoveDestination(dest uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.dests, dest)
	if c.coordination == nil {
		return
	}
	err := c.coordination.Client.Send(
		&proto.CoordinateRequest{
			RemoveTunnel: &proto.CoordinateRequest_Tunnel{Id: UUIDToByteSlice(dest)},
		})
	if err != nil {
		c.coordination.SendErr(err)
		cErr := c.coordination.Client.Close() // close the client so we don't gracefully disconnect
		if cErr != nil {
			c.Logger.Debug(context.Background(),
				"failed to close coordinator client after remove tunnel failure",
				slog.Error(cErr))
		}
		c.coordination = nil
	}
}

func (c *TunnelSrcCoordController) SyncDestinations(destinations []uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	toAdd := make(map[uuid.UUID]struct{})
	toRemove := maps.Clone(c.dests)
	all := make(map[uuid.UUID]struct{})
	for _, dest := range destinations {
		all[dest] = struct{}{}
		delete(toRemove, dest)
		if _, ok := c.dests[dest]; !ok {
			toAdd[dest] = struct{}{}
		}
	}
	c.dests = all
	if c.coordination == nil {
		return
	}
	var err error
	defer func() {
		if err != nil {
			c.coordination.SendErr(err)
			cErr := c.coordination.Client.Close() // don't gracefully disconnect
			if cErr != nil {
				c.Logger.Debug(context.Background(),
					"failed to close coordinator client during sync destinations",
					slog.Error(cErr))
			}
			c.coordination = nil
		}
	}()
	for dest := range toAdd {
		c.Coordinatee.SetTunnelDestination(dest)
		err = c.coordination.Client.Send(
			&proto.CoordinateRequest{
				AddTunnel: &proto.CoordinateRequest_Tunnel{Id: UUIDToByteSlice(dest)},
			})
		if err != nil {
			return
		}
	}
	for dest := range toRemove {
		err = c.coordination.Client.Send(
			&proto.CoordinateRequest{
				RemoveTunnel: &proto.CoordinateRequest_Tunnel{Id: UUIDToByteSlice(dest)},
			})
		if err != nil {
			return
		}
	}
}

// NewAgentCoordinationController creates a CoordinationController for Coder Agents, which never
// create tunnels and always send ReadyToHandshake acknowledgements.
func NewAgentCoordinationController(
	logger slog.Logger, coordinatee Coordinatee,
) CoordinationController {
	return &BasicCoordinationController{
		Logger:      logger,
		Coordinatee: coordinatee,
		SendAcks:    true,
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
	clientID uuid.UUID,
	auth CoordinateeAuth,
	coordinator Coordinator,
) CoordinatorClient {
	logger = logger.With(slog.F("client_id", clientID))
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

type DERPMapSetter interface {
	SetDERPMap(derpMap *tailcfg.DERPMap)
}

type basicDERPController struct {
	logger slog.Logger
	setter DERPMapSetter
}

func (b *basicDERPController) New(client DERPClient) CloserWaiter {
	l := &derpSetLoop{
		logger:       b.logger,
		setter:       b.setter,
		client:       client,
		errChan:      make(chan error, 1),
		recvLoopDone: make(chan struct{}),
	}
	go l.recvLoop()
	return l
}

func NewBasicDERPController(logger slog.Logger, setter DERPMapSetter) DERPController {
	return &basicDERPController{
		logger: logger,
		setter: setter,
	}
}

type derpSetLoop struct {
	logger slog.Logger
	setter DERPMapSetter
	client DERPClient

	sync.Mutex
	closed       bool
	errChan      chan error
	recvLoopDone chan struct{}
}

func (l *derpSetLoop) Close(ctx context.Context) error {
	l.Lock()
	defer l.Unlock()
	if l.closed {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.recvLoopDone:
			return nil
		}
	}
	l.closed = true
	cErr := l.client.Close()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.recvLoopDone:
		return cErr
	}
}

func (l *derpSetLoop) Wait() <-chan error {
	return l.errChan
}

func (l *derpSetLoop) recvLoop() {
	defer close(l.recvLoopDone)
	for {
		dm, err := l.client.Recv()
		if err != nil {
			l.logger.Debug(context.Background(), "failed to receive DERP message", slog.Error(err))
			select {
			case l.errChan <- err:
			default:
			}
			return
		}
		l.logger.Debug(context.Background(), "got new DERP Map", slog.F("derp_map", dm))
		l.setter.SetDERPMap(dm)
	}
}

type BasicTelemetryController struct {
	logger slog.Logger

	sync.Mutex
	client      TelemetryClient
	unavailable bool
}

func (b *BasicTelemetryController) New(client TelemetryClient) {
	b.Lock()
	defer b.Unlock()
	b.client = client
	b.unavailable = false
	b.logger.Debug(context.Background(), "new telemetry client connected to controller")
}

func (b *BasicTelemetryController) SendTelemetryEvent(event *proto.TelemetryEvent) {
	b.Lock()
	if b.client == nil {
		b.Unlock()
		b.logger.Debug(context.Background(),
			"telemetry event dropped; no client", slog.F("event", event))
		return
	}
	if b.unavailable {
		b.Unlock()
		b.logger.Debug(context.Background(),
			"telemetry event dropped; unavailable", slog.F("event", event))
		return
	}
	client := b.client
	b.Unlock()
	unavailable := sendTelemetry(b.logger, client, event)
	if unavailable {
		b.Lock()
		defer b.Unlock()
		if b.client == client {
			b.unavailable = true
		}
	}
}

func NewBasicTelemetryController(logger slog.Logger) *BasicTelemetryController {
	return &BasicTelemetryController{logger: logger}
}

var (
	_ TelemetrySink       = &BasicTelemetryController{}
	_ TelemetryController = &BasicTelemetryController{}
)

func sendTelemetry(
	logger slog.Logger, client TelemetryClient, event *proto.TelemetryEvent,
) (
	unavailable bool,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.PostTelemetry(ctx, &proto.TelemetryRequest{
		Events: []*proto.TelemetryEvent{event},
	})
	if IsDRPCUnimplementedError(err) {
		logger.Debug(
			context.Background(),
			"attempted to send telemetry to a server that doesn't support it",
			slog.Error(err),
		)
		return true
	} else if err != nil {
		logger.Warn(
			context.Background(),
			"failed to post telemetry event",
			slog.F("event", event), slog.Error(err),
		)
	}
	return false
}

// IsDRPCUnimplementedError returns true if the error indicates the RPC called is not implemented
// by the server.
func IsDRPCUnimplementedError(err error) bool {
	return drpcerr.Code(err) == drpcerr.Unimplemented ||
		drpc.ProtocolError.Has(err) &&
			strings.Contains(err.Error(), "unknown rpc: ")
}

type basicResumeTokenController struct {
	logger slog.Logger

	sync.Mutex
	token     *proto.RefreshResumeTokenResponse
	refresher *basicResumeTokenRefresher

	// for testing
	clock quartz.Clock
}

func (b *basicResumeTokenController) New(client ResumeTokenClient) CloserWaiter {
	b.Lock()
	defer b.Unlock()
	if b.refresher != nil {
		cErr := b.refresher.Close(context.Background())
		if cErr != nil {
			b.logger.Debug(context.Background(), "closed previous refresher", slog.Error(cErr))
		}
	}
	b.refresher = newBasicResumeTokenRefresher(b.logger, b.clock, b, client)
	return b.refresher
}

func (b *basicResumeTokenController) Token() (string, bool) {
	b.Lock()
	defer b.Unlock()
	if b.token == nil {
		return "", false
	}
	if b.token.ExpiresAt.AsTime().Before(b.clock.Now()) {
		return "", false
	}
	return b.token.Token, true
}

func NewBasicResumeTokenController(logger slog.Logger, clock quartz.Clock) ResumeTokenController {
	return &basicResumeTokenController{
		logger: logger,
		clock:  clock,
	}
}

type basicResumeTokenRefresher struct {
	logger slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
	ctrl   *basicResumeTokenController
	client ResumeTokenClient
	errCh  chan error

	sync.Mutex
	closed bool
	timer  *quartz.Timer
}

func (r *basicResumeTokenRefresher) Close(_ context.Context) error {
	r.cancel()
	r.Lock()
	defer r.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	r.timer.Stop()
	select {
	case r.errCh <- nil:
	default: // already have an error
	}
	return nil
}

func (r *basicResumeTokenRefresher) Wait() <-chan error {
	return r.errCh
}

const never time.Duration = math.MaxInt64

func newBasicResumeTokenRefresher(
	logger slog.Logger, clock quartz.Clock,
	ctrl *basicResumeTokenController, client ResumeTokenClient,
) *basicResumeTokenRefresher {
	r := &basicResumeTokenRefresher{
		logger: logger,
		ctrl:   ctrl,
		client: client,
		errCh:  make(chan error, 1),
	}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	r.timer = clock.AfterFunc(never, r.refresh, "basicResumeTokenRefresher")
	go r.refresh()
	return r
}

func (r *basicResumeTokenRefresher) refresh() {
	if r.ctx.Err() != nil {
		return // context done, no need to refresh
	}
	res, err := r.client.RefreshResumeToken(r.ctx, &proto.RefreshResumeTokenRequest{})
	if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
		// these can only come from being closed, no need to log
		select {
		case r.errCh <- nil:
		default: // already have an error
		}
		return
	}
	if IsDRPCUnimplementedError(err) {
		r.logger.Info(r.ctx, "resume token is not supported by the server")
		select {
		case r.errCh <- nil:
		default: // already have an error
		}
		return
	} else if err != nil {
		r.logger.Error(r.ctx, "error refreshing coordinator resume token", slog.Error(err))
		select {
		case r.errCh <- err:
		default: // already have an error
		}
		return
	}
	r.logger.Debug(r.ctx, "refreshed coordinator resume token",
		slog.F("expires_at", res.GetExpiresAt()),
		slog.F("refresh_in", res.GetRefreshIn()),
	)
	r.ctrl.Lock()
	if r.ctrl.refresher == r { // don't overwrite if we're not the current refresher
		r.ctrl.token = res
	} else {
		r.logger.Debug(context.Background(), "not writing token because we have a new client")
	}
	r.ctrl.Unlock()
	dur := res.RefreshIn.AsDuration()
	if dur <= 0 {
		// A sensible delay to refresh again.
		dur = 30 * time.Minute
	}
	r.Lock()
	defer r.Unlock()
	if r.closed {
		return
	}
	r.timer.Reset(dur, "basicResumeTokenRefresher", "refresh")
}

type tunnelAllWorkspaceUpdatesController struct {
	coordCtrl     *TunnelSrcCoordController
	dnsHostSetter DNSHostsSetter
	logger        slog.Logger
}

type workspace struct {
	id     uuid.UUID
	name   string
	agents map[uuid.UUID]agent
}

// addAllDNSNames adds names for all of its agents to the given map of names
func (w workspace) addAllDNSNames(names map[dnsname.FQDN][]netip.Addr) error {
	for _, a := range w.agents {
		// TODO: technically, DNS labels cannot start with numbers, but the rules are often not
		//       strictly enforced.
		// TODO: support <agent>.<workspace>.<username>.coder
		fqdn, err := dnsname.ToFQDN(fmt.Sprintf("%s.%s.me.coder.", a.name, w.name))
		if err != nil {
			return err
		}
		names[fqdn] = []netip.Addr{CoderServicePrefix.AddrFromUUID(a.id)}
	}
	// TODO: Possibly support <workspace>.coder. alias if there is only one agent
	return nil
}

type agent struct {
	id   uuid.UUID
	name string
}

func (t *tunnelAllWorkspaceUpdatesController) New(client WorkspaceUpdatesClient) CloserWaiter {
	updater := &tunnelUpdater{
		client:         client,
		errChan:        make(chan error, 1),
		logger:         t.logger,
		coordCtrl:      t.coordCtrl,
		dnsHostsSetter: t.dnsHostSetter,
		recvLoopDone:   make(chan struct{}),
		workspaces:     make(map[uuid.UUID]*workspace),
	}
	go updater.recvLoop()
	return updater
}

type tunnelUpdater struct {
	errChan        chan error
	logger         slog.Logger
	client         WorkspaceUpdatesClient
	coordCtrl      *TunnelSrcCoordController
	dnsHostsSetter DNSHostsSetter
	recvLoopDone   chan struct{}

	// don't need the mutex since only manipulated by the recvLoop
	workspaces map[uuid.UUID]*workspace

	sync.Mutex
	closed bool
}

func (t *tunnelUpdater) Close(ctx context.Context) error {
	t.Lock()
	defer t.Unlock()
	if t.closed {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.recvLoopDone:
			return nil
		}
	}
	t.closed = true
	cErr := t.client.Close()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.recvLoopDone:
		return cErr
	}
}

func (t *tunnelUpdater) Wait() <-chan error {
	return t.errChan
}

func (t *tunnelUpdater) recvLoop() {
	t.logger.Debug(context.Background(), "tunnel updater recvLoop started")
	defer t.logger.Debug(context.Background(), "tunnel updater recvLoop done")
	defer close(t.recvLoopDone)
	for {
		update, err := t.client.Recv()
		if err != nil {
			t.logger.Debug(context.Background(), "failed to receive workspace Update", slog.Error(err))
			select {
			case t.errChan <- err:
			default:
			}
			return
		}
		t.logger.Debug(context.Background(), "got workspace update",
			slog.F("workspace_update", update),
		)
		err = t.handleUpdate(update)
		if err != nil {
			t.logger.Critical(context.Background(), "failed to handle workspace Update", slog.Error(err))
			cErr := t.client.Close()
			if cErr != nil {
				t.logger.Warn(context.Background(), "failed to close client", slog.Error(cErr))
			}
			select {
			case t.errChan <- err:
			default:
			}
			return
		}
	}
}

func (t *tunnelUpdater) handleUpdate(update *proto.WorkspaceUpdate) error {
	for _, uw := range update.UpsertedWorkspaces {
		workspaceID, err := uuid.FromBytes(uw.Id)
		if err != nil {
			return xerrors.Errorf("failed to parse workspace ID: %w", err)
		}
		w := workspace{
			id:     workspaceID,
			name:   uw.Name,
			agents: make(map[uuid.UUID]agent),
		}
		t.upsertWorkspace(w)
	}

	// delete agents before deleting workspaces, since the agents have workspace ID references
	for _, da := range update.DeletedAgents {
		agentID, err := uuid.FromBytes(da.Id)
		if err != nil {
			return xerrors.Errorf("failed to parse agent ID: %w", err)
		}
		workspaceID, err := uuid.FromBytes(da.WorkspaceId)
		if err != nil {
			return xerrors.Errorf("failed to parse workspace ID: %w", err)
		}
		err = t.deleteAgent(workspaceID, agentID)
		if err != nil {
			return xerrors.Errorf("failed to delete agent: %w", err)
		}
	}
	for _, dw := range update.DeletedWorkspaces {
		workspaceID, err := uuid.FromBytes(dw.Id)
		if err != nil {
			return xerrors.Errorf("failed to parse workspace ID: %w", err)
		}
		t.deleteWorkspace(workspaceID)
	}

	// upsert agents last, after all workspaces have been added and deleted, since agents reference
	// workspace ID.
	for _, ua := range update.UpsertedAgents {
		agentID, err := uuid.FromBytes(ua.Id)
		if err != nil {
			return xerrors.Errorf("failed to parse agent ID: %w", err)
		}
		workspaceID, err := uuid.FromBytes(ua.WorkspaceId)
		if err != nil {
			return xerrors.Errorf("failed to parse workspace ID: %w", err)
		}
		a := agent{name: ua.Name, id: agentID}
		err = t.upsertAgent(workspaceID, a)
		if err != nil {
			return xerrors.Errorf("failed to upsert agent: %w", err)
		}
	}
	allAgents := t.allAgentIDs()
	t.coordCtrl.SyncDestinations(allAgents)
	if t.dnsHostsSetter != nil {
		t.logger.Debug(context.Background(), "updating dns hosts")
		dnsNames := t.allDNSNames()
		err := t.dnsHostsSetter.SetDNSHosts(dnsNames)
		if err != nil {
			return xerrors.Errorf("failed to set DNS hosts: %w", err)
		}
	} else {
		t.logger.Debug(context.Background(), "skipping setting DNS names because we have no setter")
	}
	return nil
}

func (t *tunnelUpdater) upsertWorkspace(w workspace) {
	old, ok := t.workspaces[w.id]
	if !ok {
		t.workspaces[w.id] = &w
		return
	}
	old.name = w.name
}

func (t *tunnelUpdater) deleteWorkspace(id uuid.UUID) {
	delete(t.workspaces, id)
}

func (t *tunnelUpdater) upsertAgent(workspaceID uuid.UUID, a agent) error {
	w, ok := t.workspaces[workspaceID]
	if !ok {
		return xerrors.Errorf("workspace %s not found", workspaceID)
	}
	w.agents[a.id] = a
	return nil
}

func (t *tunnelUpdater) deleteAgent(workspaceID, id uuid.UUID) error {
	w, ok := t.workspaces[workspaceID]
	if !ok {
		return xerrors.Errorf("workspace %s not found", workspaceID)
	}
	delete(w.agents, id)
	return nil
}

func (t *tunnelUpdater) allAgentIDs() []uuid.UUID {
	out := make([]uuid.UUID, 0, len(t.workspaces))
	for _, w := range t.workspaces {
		for id := range w.agents {
			out = append(out, id)
		}
	}
	return out
}

func (t *tunnelUpdater) allDNSNames() map[dnsname.FQDN][]netip.Addr {
	names := make(map[dnsname.FQDN][]netip.Addr)
	for _, w := range t.workspaces {
		err := w.addAllDNSNames(names)
		if err != nil {
			// This should never happen in production, because converting the FQDN only fails
			// if names are too long, and we put strict length limits on agent, workspace, and user
			// names.
			t.logger.Critical(context.Background(),
				"failed to include DNS name(s)",
				slog.F("workspace_id", w.id),
				slog.Error(err))
		}
	}
	return names
}

// NewTunnelAllWorkspaceUpdatesController creates a WorkspaceUpdatesController that creates tunnels
// (via the TunnelSrcCoordController) to all agents received over the WorkspaceUpdates RPC. If a
// DNSHostSetter is provided, it also programs DNS hosts based on the agent and workspace names.
func NewTunnelAllWorkspaceUpdatesController(
	logger slog.Logger, c *TunnelSrcCoordController, d DNSHostsSetter,
) WorkspaceUpdatesController {
	return &tunnelAllWorkspaceUpdatesController{logger: logger, coordCtrl: c, dnsHostSetter: d}
}

// NewController creates a new Controller without running it
func NewController(logger slog.Logger, dialer ControlProtocolDialer, opts ...ControllerOpt) *Controller {
	c := &Controller{
		logger:          logger,
		clock:           quartz.NewReal(),
		gracefulTimeout: time.Second,
		Dialer:          dialer,
		closedCh:        make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type ControllerOpt func(*Controller)

func WithTestClock(clock quartz.Clock) ControllerOpt {
	return func(c *Controller) {
		c.clock = clock
	}
}

func WithGracefulTimeout(timeout time.Duration) ControllerOpt {
	return func(c *Controller) {
		c.gracefulTimeout = timeout
	}
}

// manageGracefulTimeout allows the gracefulContext to last longer than the main context
// to allow a graceful disconnect.
func (c *Controller) manageGracefulTimeout() {
	defer c.cancelGracefulCtx()
	<-c.ctx.Done()
	timer := c.clock.NewTimer(c.gracefulTimeout, "tailnetAPIClient", "gracefulTimeout")
	defer timer.Stop()
	select {
	case <-c.closedCh:
	case <-timer.C:
	}
}

// Run dials the API and uses it with the provided controllers.
func (c *Controller) Run(ctx context.Context) {
	c.ctx = ctx
	c.gracefulCtx, c.cancelGracefulCtx = context.WithCancel(context.Background())
	go c.manageGracefulTimeout()
	go func() {
		defer close(c.closedCh)
		// Sadly retry doesn't support quartz.Clock yet so this is not
		// influenced by the configured clock.
		for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(c.ctx); {
			tailnetClients, err := c.Dialer.Dial(c.ctx, c.ResumeTokenCtrl)
			if err != nil {
				if xerrors.Is(err, context.Canceled) {
					continue
				}
				errF := slog.Error(err)
				var sdkErr *codersdk.Error
				if xerrors.As(err, &sdkErr) {
					errF = slog.Error(sdkErr)
				}
				c.logger.Error(c.ctx, "failed to dial tailnet v2+ API", errF)
				continue
			}
			c.logger.Info(c.ctx, "obtained tailnet API v2+ client")
			c.runControllersOnce(tailnetClients)
			c.logger.Info(c.ctx, "tailnet API v2+ connection lost")
		}
	}()
}

// runControllersOnce uses the provided clients to call into the controllers once. It is combined
// into one function so that a problem with one tears down the other and triggers a retry (if
// appropriate). We typically multiplex all RPCs over the same websocket, so we want them to share
// the same fate.
func (c *Controller) runControllersOnce(clients ControlProtocolClients) {
	// clients.Closer.Close should nominally be idempotent, but let's not press our luck
	closeOnce := sync.Once{}
	closeClients := func() {
		closeOnce.Do(func() {
			closeErr := clients.Closer.Close()
			if closeErr != nil &&
				!xerrors.Is(closeErr, io.EOF) &&
				!xerrors.Is(closeErr, context.Canceled) &&
				!xerrors.Is(closeErr, context.DeadlineExceeded) {
				c.logger.Error(c.ctx, "error closing tailnet clients", slog.Error(closeErr))
			}
		})
	}
	defer closeClients()

	if c.TelemetryCtrl != nil {
		c.TelemetryCtrl.New(clients.Telemetry) // synchronous, doesn't need a goroutine
	}

	wg := sync.WaitGroup{}

	if c.CoordCtrl != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.coordinate(clients.Coordinator)
			if c.ctx.Err() == nil {
				// Main context is still active, but our coordination exited, due to some error.
				// Close down all the rest of the clients so we'll exit and retry.
				closeClients()
			}
		}()
	}
	if c.DERPCtrl != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dErr := c.derpMap(clients.DERP)
			if dErr != nil && c.ctx.Err() == nil {
				// The main context is still active, meaning that we want the tailnet data plane to stay
				// up, even though we hit some error getting DERP maps on the control plane.  That means
				// we do NOT want to gracefully disconnect on the coordinate() routine.  So, we'll just
				// close the underlying connection. This will trigger a retry of the control plane in
				// run().
				closeClients()
			}
		}()
	}

	// Refresh token is a little different, in that we don't want its controller to hold open the
	// connection on its own.  So we keep it separate from the other wait group, and cancel its
	// context as soon as the other routines exit.
	refreshTokenCtx, refreshTokenCancel := context.WithCancel(c.ctx)
	refreshTokenDone := make(chan struct{})
	defer func() {
		<-refreshTokenDone
	}()
	defer refreshTokenCancel()
	go func() {
		defer close(refreshTokenDone)
		if c.ResumeTokenCtrl != nil {
			c.refreshToken(refreshTokenCtx, clients.ResumeToken)
		}
	}()

	wg.Wait()
}

func (c *Controller) coordinate(client CoordinatorClient) {
	defer func() {
		cErr := client.Close()
		if cErr != nil {
			c.logger.Debug(c.ctx, "error closing Coordinate RPC", slog.Error(cErr))
		}
	}()
	coordination := c.CoordCtrl.New(client)
	c.logger.Debug(c.ctx, "serving coordinator")
	select {
	case <-c.ctx.Done():
		c.logger.Debug(c.ctx, "main context canceled; do graceful disconnect")
		crdErr := coordination.Close(c.gracefulCtx)
		if crdErr != nil {
			c.logger.Warn(c.ctx, "failed to close remote coordination", slog.Error(crdErr))
		}
	case err := <-coordination.Wait():
		if err != nil &&
			!xerrors.Is(err, io.EOF) &&
			!xerrors.Is(err, context.Canceled) &&
			!xerrors.Is(err, context.DeadlineExceeded) {
			c.logger.Error(c.ctx, "remote coordination error", slog.Error(err))
		}
	}
}

func (c *Controller) derpMap(client DERPClient) error {
	defer func() {
		cErr := client.Close()
		if cErr != nil {
			c.logger.Debug(c.ctx, "error closing StreamDERPMaps RPC", slog.Error(cErr))
		}
	}()
	cw := c.DERPCtrl.New(client)
	select {
	case <-c.ctx.Done():
		cErr := client.Close()
		if cErr != nil {
			c.logger.Warn(c.ctx, "failed to close StreamDERPMaps RPC", slog.Error(cErr))
		}
		return nil
	case err := <-cw.Wait():
		if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		if err != nil && !xerrors.Is(err, io.EOF) {
			c.logger.Error(c.ctx, "error receiving DERP Map", slog.Error(err))
		}
		return err
	}
}

func (c *Controller) refreshToken(ctx context.Context, client ResumeTokenClient) {
	cw := c.ResumeTokenCtrl.New(client)
	go func() {
		<-ctx.Done()
		cErr := cw.Close(c.ctx)
		if cErr != nil {
			c.logger.Error(c.ctx, "error closing token refresher", slog.Error(cErr))
		}
	}()

	err := <-cw.Wait()
	if err != nil && !xerrors.Is(err, context.Canceled) && !xerrors.Is(err, context.DeadlineExceeded) {
		c.logger.Error(c.ctx, "error receiving refresh token", slog.Error(err))
	}
}

func (c *Controller) Closed() <-chan struct{} {
	return c.closedCh
}
