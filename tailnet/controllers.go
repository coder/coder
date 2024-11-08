package tailnet

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"storj.io/drpc"
	"storj.io/drpc/drpcerr"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
	"github.com/coder/retry"
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
	if drpcerr.Code(err) == drpcerr.Unimplemented ||
		drpc.ProtocolError.Has(err) &&
			strings.Contains(err.Error(), "unknown rpc: ") {
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
	if err != nil {
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
			c.logger.Debug(c.ctx, "obtained tailnet API v2+ client")
			c.runControllersOnce(tailnetClients)
			c.logger.Debug(c.ctx, "tailnet API v2+ connection lost")
		}
	}()
}

// runControllersOnce uses the provided clients to call into the controllers once. It is combined
// into one function so that a problem with one tears down the other and triggers a retry (if
// appropriate). We typically multiplex all RPCs over the same websocket, so we want them to share
// the same fate.
func (c *Controller) runControllersOnce(clients ControlProtocolClients) {
	defer func() {
		closeErr := clients.Closer.Close()
		if closeErr != nil &&
			!xerrors.Is(closeErr, io.EOF) &&
			!xerrors.Is(closeErr, context.Canceled) &&
			!xerrors.Is(closeErr, context.DeadlineExceeded) {
			c.logger.Error(c.ctx, "error closing DRPC connection", slog.Error(closeErr))
		}
	}()

	if c.TelemetryCtrl != nil {
		c.TelemetryCtrl.New(clients.Telemetry) // synchronous, doesn't need a goroutine
	}

	wg := sync.WaitGroup{}

	if c.CoordCtrl != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.coordinate(clients.Coordinator)
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
				_ = clients.Closer.Close()
				// Note that derpMap() logs it own errors, we don't bother here.
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
