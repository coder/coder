package workspacesdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"storj.io/drpc"
	"storj.io/drpc/drpcerr"
	"tailscale.com/tailcfg"

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
	SetDERPMap(derpMap *tailcfg.DERPMap)
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

	agentID       uuid.UUID
	coordinateURL string
	clock         quartz.Clock
	dialOptions   *websocket.DialOptions
	conn          tailnetConn
	customDialFn  func() (proto.DRPCTailnetClient, error)

	clientMu sync.RWMutex
	client   proto.DRPCTailnetClient

	connected   chan error
	resumeToken *proto.RefreshResumeTokenResponse
	isFirst     bool
	closed      chan struct{}

	// Only set to true if we get a response from the server that it doesn't support
	// network telemetry.
	telemetryUnavailable atomic.Bool
}

// Create a new tailnetAPIConnector without running it
func newTailnetAPIConnector(ctx context.Context, logger slog.Logger, agentID uuid.UUID, coordinateURL string, clock quartz.Clock, dialOptions *websocket.DialOptions) *tailnetAPIConnector {
	return &tailnetAPIConnector{
		ctx:           ctx,
		logger:        logger,
		agentID:       agentID,
		coordinateURL: coordinateURL,
		clock:         clock,
		dialOptions:   dialOptions,
		conn:          nil,
		connected:     make(chan error, 1),
		closed:        make(chan struct{}),
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
	tac.conn = conn
	tac.gracefulCtx, tac.cancelGracefulCtx = context.WithCancel(context.Background())
	go tac.manageGracefulTimeout()
	go func() {
		tac.isFirst = true
		defer close(tac.closed)
		// Sadly retry doesn't support quartz.Clock yet so this is not
		// influenced by the configured clock.
		for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(tac.ctx); {
			tailnetClient, err := tac.dial()
			if err != nil {
				continue
			}
			tac.clientMu.Lock()
			tac.client = tailnetClient
			tac.clientMu.Unlock()
			tac.logger.Debug(tac.ctx, "obtained tailnet API v2+ client")
			tac.runConnectorOnce(tailnetClient)
			tac.logger.Debug(tac.ctx, "tailnet API v2+ connection lost")
		}
	}()
}

var permanentErrorStatuses = []int{
	http.StatusConflict,   // returned if client/agent connections disabled (browser only)
	http.StatusBadRequest, // returned if API mismatch
	http.StatusNotFound,   // returned if user doesn't have permission or agent doesn't exist
}

func (tac *tailnetAPIConnector) dial() (proto.DRPCTailnetClient, error) {
	if tac.customDialFn != nil {
		return tac.customDialFn()
	}
	tac.logger.Debug(tac.ctx, "dialing Coder tailnet v2+ API")

	u, err := url.Parse(tac.coordinateURL)
	if err != nil {
		return nil, xerrors.Errorf("parse URL %q: %w", tac.coordinateURL, err)
	}
	if tac.resumeToken != nil {
		q := u.Query()
		q.Set("resume_token", tac.resumeToken.Token)
		u.RawQuery = q.Encode()
		tac.logger.Debug(tac.ctx, "using resume token", slog.F("resume_token", tac.resumeToken))
	}

	coordinateURL := u.String()
	tac.logger.Debug(tac.ctx, "using coordinate URL", slog.F("url", coordinateURL))

	// nolint:bodyclose
	ws, res, err := websocket.Dial(tac.ctx, coordinateURL, tac.dialOptions)
	if tac.isFirst {
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
			tac.connected <- err
			return nil, err
		}
		tac.isFirst = false
		close(tac.connected)
	}
	if err != nil {
		bodyErr := codersdk.ReadBodyAsError(res)
		var sdkErr *codersdk.Error
		if xerrors.As(bodyErr, &sdkErr) {
			for _, v := range sdkErr.Validations {
				if v.Field == "resume_token" {
					// Unset the resume token for the next attempt
					tac.logger.Warn(tac.ctx, "failed to dial tailnet v2+ API: server replied invalid resume token; unsetting for next connection attempt")
					tac.resumeToken = nil
					return nil, err
				}
			}
		}
		if !errors.Is(err, context.Canceled) {
			tac.logger.Error(tac.ctx, "failed to dial tailnet v2+ API", slog.Error(err), slog.F("sdk_err", sdkErr))
		}
		return nil, err
	}
	client, err := tailnet.NewDRPCClient(
		websocket.NetConn(tac.gracefulCtx, ws, websocket.MessageBinary),
		tac.logger,
	)
	if err != nil {
		tac.logger.Debug(tac.ctx, "failed to create DRPCClient", slog.Error(err))
		_ = ws.Close(websocket.StatusInternalError, "")
		return nil, err
	}
	return client, err
}

// runConnectorOnce uses the provided client to coordinate and stream DERP Maps. It is combined
// into one function so that a problem with one tears down the other and triggers a retry (if
// appropriate). We multiplex both RPCs over the same websocket, so we want them to share the same
// fate.
func (tac *tailnetAPIConnector) runConnectorOnce(client proto.DRPCTailnetClient) {
	defer func() {
		conn := client.DRPCConn()
		closeErr := conn.Close()
		if closeErr != nil &&
			!xerrors.Is(closeErr, io.EOF) &&
			!xerrors.Is(closeErr, context.Canceled) &&
			!xerrors.Is(closeErr, context.DeadlineExceeded) {
			tac.logger.Error(tac.ctx, "error closing DRPC connection", slog.Error(closeErr))
			<-conn.Closed()
		}
	}()

	refreshTokenCtx, refreshTokenCancel := context.WithCancel(tac.ctx)
	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() {
		defer wg.Done()
		tac.coordinate(client)
	}()
	go func() {
		defer wg.Done()
		defer refreshTokenCancel()
		dErr := tac.derpMap(client)
		if dErr != nil && tac.ctx.Err() == nil {
			// The main context is still active, meaning that we want the tailnet data plane to stay
			// up, even though we hit some error getting DERP maps on the control plane.  That means
			// we do NOT want to gracefully disconnect on the coordinate() routine.  So, we'll just
			// close the underlying connection. This will trigger a retry of the control plane in
			// run().
			tac.clientMu.Lock()
			client.DRPCConn().Close()
			tac.client = nil
			tac.clientMu.Unlock()
			// Note that derpMap() logs it own errors, we don't bother here.
		}
	}()
	go func() {
		defer wg.Done()
		tac.refreshToken(refreshTokenCtx, client)
	}()
	wg.Wait()
}

func (tac *tailnetAPIConnector) coordinate(client proto.DRPCTailnetClient) {
	// we use the gracefulCtx here so that we'll have time to send the graceful disconnect
	coord, err := client.Coordinate(tac.gracefulCtx)
	if err != nil {
		tac.logger.Error(tac.ctx, "failed to connect to Coordinate RPC", slog.Error(err))
		return
	}
	defer func() {
		cErr := coord.Close()
		if cErr != nil {
			tac.logger.Debug(tac.ctx, "error closing Coordinate RPC", slog.Error(cErr))
		}
	}()
	coordination := tailnet.NewRemoteCoordination(tac.logger, coord, tac.conn, tac.agentID)
	tac.logger.Debug(tac.ctx, "serving coordinator")
	select {
	case <-tac.ctx.Done():
		tac.logger.Debug(tac.ctx, "main context canceled; do graceful disconnect")
		crdErr := coordination.Close(tac.gracefulCtx)
		if crdErr != nil {
			tac.logger.Warn(tac.ctx, "failed to close remote coordination", slog.Error(err))
		}
	case err = <-coordination.Error():
		if err != nil &&
			!xerrors.Is(err, io.EOF) &&
			!xerrors.Is(err, context.Canceled) &&
			!xerrors.Is(err, context.DeadlineExceeded) {
			tac.logger.Error(tac.ctx, "remote coordination error", slog.Error(err))
		}
	}
}

func (tac *tailnetAPIConnector) derpMap(client proto.DRPCTailnetClient) error {
	s, err := client.StreamDERPMaps(tac.ctx, &proto.StreamDERPMapsRequest{})
	if err != nil {
		return xerrors.Errorf("failed to connect to StreamDERPMaps RPC: %w", err)
	}
	defer func() {
		cErr := s.Close()
		if cErr != nil {
			tac.logger.Debug(tac.ctx, "error closing StreamDERPMaps RPC", slog.Error(cErr))
		}
	}()
	for {
		dmp, err := s.Recv()
		if err != nil {
			if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			if !xerrors.Is(err, io.EOF) {
				tac.logger.Error(tac.ctx, "error receiving DERP Map", slog.Error(err))
			}
			return err
		}
		tac.logger.Debug(tac.ctx, "got new DERP Map", slog.F("derp_map", dmp))
		dm := tailnet.DERPMapFromProto(dmp)
		tac.conn.SetDERPMap(dm)
	}
}

func (tac *tailnetAPIConnector) refreshToken(ctx context.Context, client proto.DRPCTailnetClient) {
	ticker := tac.clock.NewTicker(15*time.Second, "tailnetAPIConnector", "refreshToken")
	defer ticker.Stop()

	initialCh := make(chan struct{}, 1)
	initialCh <- struct{}{}
	defer close(initialCh)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-initialCh:
		}

		attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		res, err := client.RefreshResumeToken(attemptCtx, &proto.RefreshResumeTokenRequest{})
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				tac.logger.Error(tac.ctx, "error refreshing coordinator resume token", slog.Error(err))
			}
			return
		}
		tac.logger.Debug(tac.ctx, "refreshed coordinator resume token", slog.F("resume_token", res))
		tac.resumeToken = res
		dur := res.RefreshIn.AsDuration()
		if dur <= 0 {
			// A sensible delay to refresh again.
			dur = 30 * time.Minute
		}
		ticker.Reset(dur, "tailnetAPIConnector", "refreshToken", "reset")
	}
}

func (tac *tailnetAPIConnector) SendTelemetryEvent(event *proto.TelemetryEvent) {
	tac.clientMu.RLock()
	// We hold the lock for the entire telemetry request, but this would only block
	// a coordinate retry, and closing the connection.
	defer tac.clientMu.RUnlock()
	if tac.client == nil || tac.telemetryUnavailable.Load() {
		return
	}
	ctx, cancel := context.WithTimeout(tac.ctx, 5*time.Second)
	defer cancel()
	_, err := tac.client.PostTelemetry(ctx, &proto.TelemetryRequest{
		Events: []*proto.TelemetryEvent{event},
	})
	if drpcerr.Code(err) == drpcerr.Unimplemented || drpc.ProtocolError.Has(err) && strings.Contains(err.Error(), "unknown rpc: ") {
		tac.logger.Debug(tac.ctx, "attempted to send telemetry to a server that doesn't support it", slog.Error(err))
		tac.telemetryUnavailable.Store(true)
	}
}
