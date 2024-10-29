package workspacesdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
	"github.com/coder/retry"
)

var tailnetConnectorGracefulTimeout = time.Second

// tailnetConn is the subset of the tailnet.Conn methods that tailnetAPIConnector uses.  It is
// included so that we can fake it in testing.
//
// @typescript-ignore tailnetConn
type tailnetConn interface {
	tailnet.Coordinatee
	tailnet.DERPMapSetter
}

// tailnetAPIConnector dials the tailnet API (v2+) and then uses the API with a tailnet.Conn to
//
// 1) run the Coordinate API and pass node information back and forth
// 2) stream DERPMap updates and program the Conn
// 3) Send network telemetry events
//
// These functions share the same websocket, and so are combined here so that if we hit a problem
// we tear the whole thing down and start over with a new websocket.
//
// @typescript-ignore tailnetAPIConnector
type tailnetAPIConnector struct {
	// We keep track of two contexts: the main context from the caller, and a "graceful" context
	// that we keep open slightly longer than the main context to give a chance to send the
	// Disconnect message to the coordinator. That tells the coordinator that we really meant to
	// disconnect instead of just losing network connectivity.
	ctx               context.Context
	gracefulCtx       context.Context
	cancelGracefulCtx context.CancelFunc

	logger slog.Logger

	agentID   uuid.UUID
	clock     quartz.Clock
	dialer    tailnet.ControlProtocolDialer
	derpCtrl  tailnet.DERPController
	coordCtrl tailnet.CoordinationController
	telCtrl   *tailnet.BasicTelemetryController
	tokenCtrl tailnet.ResumeTokenController

	closed chan struct{}
}

// Create a new tailnetAPIConnector without running it
func newTailnetAPIConnector(ctx context.Context, logger slog.Logger, agentID uuid.UUID, dialer tailnet.ControlProtocolDialer, clock quartz.Clock) *tailnetAPIConnector {
	return &tailnetAPIConnector{
		ctx:       ctx,
		logger:    logger,
		agentID:   agentID,
		clock:     clock,
		dialer:    dialer,
		closed:    make(chan struct{}),
		telCtrl:   tailnet.NewBasicTelemetryController(logger),
		tokenCtrl: tailnet.NewBasicResumeTokenController(logger, clock),
	}
}

// manageGracefulTimeout allows the gracefulContext to last 1 second longer than the main context
// to allow a graceful disconnect.
func (tac *tailnetAPIConnector) manageGracefulTimeout() {
	defer tac.cancelGracefulCtx()
	<-tac.ctx.Done()
	timer := tac.clock.NewTimer(tailnetConnectorGracefulTimeout, "tailnetAPIClient", "gracefulTimeout")
	defer timer.Stop()
	select {
	case <-tac.closed:
	case <-timer.C:
	}
}

// Runs a tailnetAPIConnector using the provided connection
func (tac *tailnetAPIConnector) runConnector(conn tailnetConn) {
	tac.derpCtrl = tailnet.NewBasicDERPController(tac.logger, conn)
	tac.coordCtrl = tailnet.NewSingleDestController(tac.logger, conn, tac.agentID)
	tac.gracefulCtx, tac.cancelGracefulCtx = context.WithCancel(context.Background())
	go tac.manageGracefulTimeout()
	go func() {
		defer close(tac.closed)
		// Sadly retry doesn't support quartz.Clock yet so this is not
		// influenced by the configured clock.
		for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(tac.ctx); {
			tailnetClients, err := tac.dialer.Dial(tac.ctx, tac.tokenCtrl)
			if err != nil {
				if xerrors.Is(err, context.Canceled) {
					continue
				}
				errF := slog.Error(err)
				var sdkErr *codersdk.Error
				if xerrors.As(err, &sdkErr) {
					errF = slog.Error(sdkErr)
				}
				tac.logger.Error(tac.ctx, "failed to dial tailnet v2+ API", errF)
				continue
			}
			tac.logger.Debug(tac.ctx, "obtained tailnet API v2+ client")
			tac.runConnectorOnce(tailnetClients)
			tac.logger.Debug(tac.ctx, "tailnet API v2+ connection lost")
		}
	}()
}

var permanentErrorStatuses = []int{
	http.StatusConflict,   // returned if client/agent connections disabled (browser only)
	http.StatusBadRequest, // returned if API mismatch
	http.StatusNotFound,   // returned if user doesn't have permission or agent doesn't exist
}

// runConnectorOnce uses the provided client to coordinate and stream DERP Maps. It is combined
// into one function so that a problem with one tears down the other and triggers a retry (if
// appropriate). We multiplex both RPCs over the same websocket, so we want them to share the same
// fate.
func (tac *tailnetAPIConnector) runConnectorOnce(clients tailnet.ControlProtocolClients) {
	defer func() {
		closeErr := clients.Closer.Close()
		if closeErr != nil &&
			!xerrors.Is(closeErr, io.EOF) &&
			!xerrors.Is(closeErr, context.Canceled) &&
			!xerrors.Is(closeErr, context.DeadlineExceeded) {
			tac.logger.Error(tac.ctx, "error closing DRPC connection", slog.Error(closeErr))
		}
	}()

	tac.telCtrl.New(clients.Telemetry) // synchronous, doesn't need a goroutine

	refreshTokenCtx, refreshTokenCancel := context.WithCancel(tac.ctx)
	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() {
		defer wg.Done()
		tac.coordinate(clients.Coordinator)
	}()
	go func() {
		defer wg.Done()
		defer refreshTokenCancel()
		dErr := tac.derpMap(clients.DERP)
		if dErr != nil && tac.ctx.Err() == nil {
			// The main context is still active, meaning that we want the tailnet data plane to stay
			// up, even though we hit some error getting DERP maps on the control plane.  That means
			// we do NOT want to gracefully disconnect on the coordinate() routine.  So, we'll just
			// close the underlying connection. This will trigger a retry of the control plane in
			// run().
			clients.Closer.Close()
			// Note that derpMap() logs it own errors, we don't bother here.
		}
	}()
	go func() {
		defer wg.Done()
		tac.refreshToken(refreshTokenCtx, clients.ResumeToken)
	}()
	wg.Wait()
}

func (tac *tailnetAPIConnector) coordinate(client tailnet.CoordinatorClient) {
	defer func() {
		cErr := client.Close()
		if cErr != nil {
			tac.logger.Debug(tac.ctx, "error closing Coordinate RPC", slog.Error(cErr))
		}
	}()
	coordination := tac.coordCtrl.New(client)
	tac.logger.Debug(tac.ctx, "serving coordinator")
	select {
	case <-tac.ctx.Done():
		tac.logger.Debug(tac.ctx, "main context canceled; do graceful disconnect")
		crdErr := coordination.Close(tac.gracefulCtx)
		if crdErr != nil {
			tac.logger.Warn(tac.ctx, "failed to close remote coordination", slog.Error(crdErr))
		}
	case err := <-coordination.Wait():
		if err != nil &&
			!xerrors.Is(err, io.EOF) &&
			!xerrors.Is(err, context.Canceled) &&
			!xerrors.Is(err, context.DeadlineExceeded) {
			tac.logger.Error(tac.ctx, "remote coordination error", slog.Error(err))
		}
	}
}

func (tac *tailnetAPIConnector) derpMap(client tailnet.DERPClient) error {
	defer func() {
		cErr := client.Close()
		if cErr != nil {
			tac.logger.Debug(tac.ctx, "error closing StreamDERPMaps RPC", slog.Error(cErr))
		}
	}()
	cw := tac.derpCtrl.New(client)
	select {
	case <-tac.ctx.Done():
		cErr := client.Close()
		if cErr != nil {
			tac.logger.Warn(tac.ctx, "failed to close StreamDERPMaps RPC", slog.Error(cErr))
		}
		return nil
	case err := <-cw.Wait():
		if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		if err != nil && !xerrors.Is(err, io.EOF) {
			tac.logger.Error(tac.ctx, "error receiving DERP Map", slog.Error(err))
		}
		return err
	}
}

func (tac *tailnetAPIConnector) refreshToken(ctx context.Context, client tailnet.ResumeTokenClient) {
	cw := tac.tokenCtrl.New(client)
	go func() {
		<-ctx.Done()
		cErr := cw.Close(tac.ctx)
		if cErr != nil {
			tac.logger.Error(tac.ctx, "error closing token refresher", slog.Error(cErr))
		}
	}()

	err := <-cw.Wait()
	if err != nil && !xerrors.Is(err, context.Canceled) && !xerrors.Is(err, context.DeadlineExceeded) {
		tac.logger.Error(tac.ctx, "error receiving refresh token", slog.Error(err))
	}
}

func (tac *tailnetAPIConnector) SendTelemetryEvent(event *proto.TelemetryEvent) {
	tac.telCtrl.SendTelemetryEvent(event)
}

type WebsocketDialer struct {
	logger            slog.Logger
	dialOptions       *websocket.DialOptions
	url               *url.URL
	resumeTokenFailed bool
	connected         chan error
	isFirst           bool
}

func (w *WebsocketDialer) Dial(ctx context.Context, r tailnet.ResumeTokenController,
) (
	tailnet.ControlProtocolClients, error,
) {
	w.logger.Debug(ctx, "dialing Coder tailnet v2+ API")

	u := new(url.URL)
	*u = *w.url
	if r != nil && !w.resumeTokenFailed {
		if token, ok := r.Token(); ok {
			q := u.Query()
			q.Set("resume_token", token)
			u.RawQuery = q.Encode()
			w.logger.Debug(ctx, "using resume token on dial")
		}
	}

	// nolint:bodyclose
	ws, res, err := websocket.Dial(ctx, u.String(), w.dialOptions)
	if w.isFirst {
		if res != nil && slices.Contains(permanentErrorStatuses, res.StatusCode) {
			err = codersdk.ReadBodyAsError(res)
			// A bit more human-readable help in the case the API version was rejected
			var sdkErr *codersdk.Error
			if xerrors.As(err, &sdkErr) {
				if sdkErr.Message == AgentAPIMismatchMessage &&
					sdkErr.StatusCode() == http.StatusBadRequest {
					sdkErr.Helper = fmt.Sprintf(
						"Ensure your client release version (%s, different than the API version) matches the server release version",
						buildinfo.Version())
				}
			}
			w.connected <- err
			return tailnet.ControlProtocolClients{}, err
		}
		w.isFirst = false
		close(w.connected)
	}
	if err != nil {
		bodyErr := codersdk.ReadBodyAsError(res)
		var sdkErr *codersdk.Error
		if xerrors.As(bodyErr, &sdkErr) {
			for _, v := range sdkErr.Validations {
				if v.Field == "resume_token" {
					// Unset the resume token for the next attempt
					w.logger.Warn(ctx, "failed to dial tailnet v2+ API: server replied invalid resume token; unsetting for next connection attempt")
					w.resumeTokenFailed = true
					return tailnet.ControlProtocolClients{}, err
				}
			}
		}
		if !errors.Is(err, context.Canceled) {
			w.logger.Error(ctx, "failed to dial tailnet v2+ API", slog.Error(err), slog.F("sdk_err", sdkErr))
		}
		return tailnet.ControlProtocolClients{}, err
	}
	w.resumeTokenFailed = false

	client, err := tailnet.NewDRPCClient(
		websocket.NetConn(context.Background(), ws, websocket.MessageBinary),
		w.logger,
	)
	if err != nil {
		w.logger.Debug(ctx, "failed to create DRPCClient", slog.Error(err))
		_ = ws.Close(websocket.StatusInternalError, "")
		return tailnet.ControlProtocolClients{}, err
	}
	coord, err := client.Coordinate(context.Background())
	if err != nil {
		w.logger.Debug(ctx, "failed to create Coordinate RPC", slog.Error(err))
		_ = ws.Close(websocket.StatusInternalError, "")
		return tailnet.ControlProtocolClients{}, err
	}

	derps := &tailnet.DERPFromDRPCWrapper{}
	derps.Client, err = client.StreamDERPMaps(context.Background(), &proto.StreamDERPMapsRequest{})
	if err != nil {
		w.logger.Debug(ctx, "failed to create DERPMap stream", slog.Error(err))
		_ = ws.Close(websocket.StatusInternalError, "")
		return tailnet.ControlProtocolClients{}, err
	}

	return tailnet.ControlProtocolClients{
		Closer:      client.DRPCConn(),
		Coordinator: coord,
		DERP:        derps,
		ResumeToken: client,
		Telemetry:   client,
	}, nil
}

func (w *WebsocketDialer) Connected() <-chan error {
	return w.connected
}

func NewWebsocketDialer(logger slog.Logger, u *url.URL, opts *websocket.DialOptions) *WebsocketDialer {
	return &WebsocketDialer{
		logger:      logger,
		dialOptions: opts,
		url:         u,
		connected:   make(chan error, 1),
		isFirst:     true,
	}
}
