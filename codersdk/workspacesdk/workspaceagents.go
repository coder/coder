package workspacesdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/retry"
)

type Client struct {
	client *codersdk.Client
}

func NewClient(c *codersdk.Client) *Client {
	return &Client{client: c}
}

// AgentConnectionInfo returns required information for establishing
// a connection with a workspace.
// @typescript-ignore AgentConnectionInfo
type AgentConnectionInfo struct {
	DERPMap                  *tailcfg.DERPMap `json:"derp_map"`
	DERPForceWebSockets      bool             `json:"derp_force_websockets"`
	DisableDirectConnections bool             `json:"disable_direct_connections"`
}

func (c *Client) AgentConnectionInfoGeneric(ctx context.Context) (AgentConnectionInfo, error) {
	res, err := c.client.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/connection", nil)
	if err != nil {
		return AgentConnectionInfo{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AgentConnectionInfo{}, codersdk.ReadBodyAsError(res)
	}

	var connInfo AgentConnectionInfo
	return connInfo, json.NewDecoder(res.Body).Decode(&connInfo)
}

func (c *Client) AgentConnectionInfo(ctx context.Context, agentID uuid.UUID) (AgentConnectionInfo, error) {
	res, err := c.client.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/connection", agentID), nil)
	if err != nil {
		return AgentConnectionInfo{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AgentConnectionInfo{}, codersdk.ReadBodyAsError(res)
	}

	var connInfo AgentConnectionInfo
	return connInfo, json.NewDecoder(res.Body).Decode(&connInfo)
}

// @typescript-ignore DialAgentOptions
type DialAgentOptions struct {
	Logger slog.Logger
	// BlockEndpoints forced a direct connection through DERP. The Client may
	// have DisableDirect set which will override this value.
	BlockEndpoints bool
}

func (c *Client) DialAgent(dialCtx context.Context, agentID uuid.UUID, options *DialAgentOptions) (agentConn *AgentConn, err error) {
	if options == nil {
		options = &DialAgentOptions{}
	}

	connInfo, err := c.AgentConnectionInfo(dialCtx, agentID)
	if err != nil {
		return nil, xerrors.Errorf("get connection info: %w", err)
	}
	if connInfo.DisableDirectConnections {
		options.BlockEndpoints = true
	}

	ip := tailnet.IP()
	var header http.Header
	if headerTransport, ok := c.client.HTTPClient.Transport.(*codersdk.HeaderTransport); ok {
		header = headerTransport.Header
	}
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:           []netip.Prefix{netip.PrefixFrom(ip, 128)},
		DERPMap:             connInfo.DERPMap,
		DERPHeader:          &header,
		DERPForceWebSockets: connInfo.DERPForceWebSockets,
		Logger:              options.Logger,
		BlockEndpoints:      c.client.DisableDirectConnections || options.BlockEndpoints,
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet: %w", err)
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()

	headers := make(http.Header)
	tokenHeader := codersdk.SessionTokenHeader
	if c.client.SessionTokenHeader != "" {
		tokenHeader = c.client.SessionTokenHeader
	}
	headers.Set(tokenHeader, c.client.SessionToken())

	// New context, separate from dialCtx. We don't want to cancel the
	// connection if dialCtx is canceled.
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	coordinateURL, err := c.client.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/coordinate", agentID))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	q := coordinateURL.Query()
	q.Add("version", proto.CurrentVersion.String())
	coordinateURL.RawQuery = q.Encode()

	connector := runTailnetAPIConnector(ctx, options.Logger,
		agentID, coordinateURL.String(),
		&websocket.DialOptions{
			HTTPClient: c.client.HTTPClient,
			HTTPHeader: headers,
			// Need to disable compression to avoid a data-race.
			CompressionMode: websocket.CompressionDisabled,
		},
		conn,
	)
	options.Logger.Debug(ctx, "running tailnet API v2+ connector")

	select {
	case <-dialCtx.Done():
		return nil, xerrors.Errorf("timed out waiting for coordinator and derp map: %w", dialCtx.Err())
	case err = <-connector.connected:
		if err != nil {
			options.Logger.Error(ctx, "failed to connect to tailnet v2+ API", slog.Error(err))
			return nil, xerrors.Errorf("start connector: %w", err)
		}
		options.Logger.Debug(ctx, "connected to tailnet v2+ API")
	}

	agentConn = NewAgentConn(conn, AgentConnOptions{
		AgentID: agentID,
		CloseFunc: func() error {
			cancel()
			<-connector.closed
			return conn.Close()
		},
	})

	if !agentConn.AwaitReachable(dialCtx) {
		_ = agentConn.Close()
		return nil, xerrors.Errorf("timed out waiting for agent to become reachable: %w", dialCtx.Err())
	}

	return agentConn, nil
}

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
	dialOptions   *websocket.DialOptions
	conn          tailnetConn

	connected chan error
	isFirst   bool
	closed    chan struct{}
}

// runTailnetAPIConnector creates and runs a tailnetAPIConnector
func runTailnetAPIConnector(
	ctx context.Context, logger slog.Logger,
	agentID uuid.UUID, coordinateURL string, dialOptions *websocket.DialOptions,
	conn tailnetConn,
) *tailnetAPIConnector {
	tac := &tailnetAPIConnector{
		ctx:           ctx,
		logger:        logger,
		agentID:       agentID,
		coordinateURL: coordinateURL,
		dialOptions:   dialOptions,
		conn:          conn,
		connected:     make(chan error, 1),
		closed:        make(chan struct{}),
	}
	tac.gracefulCtx, tac.cancelGracefulCtx = context.WithCancel(context.Background())
	go tac.manageGracefulTimeout()
	go tac.run()
	return tac
}

// manageGracefulTimeout allows the gracefulContext to last 1 second longer than the main context
// to allow a graceful disconnect.
func (tac *tailnetAPIConnector) manageGracefulTimeout() {
	defer tac.cancelGracefulCtx()
	<-tac.ctx.Done()
	select {
	case <-tac.closed:
	case <-time.After(time.Second):
	}
}

func (tac *tailnetAPIConnector) run() {
	tac.isFirst = true
	defer close(tac.closed)
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(tac.ctx); {
		tailnetClient, err := tac.dial()
		if err != nil {
			continue
		}
		tac.logger.Debug(tac.ctx, "obtained tailnet API v2+ client")
		tac.coordinateAndDERPMap(tailnetClient)
		tac.logger.Debug(tac.ctx, "tailnet API v2+ connection lost")
	}
}

func (tac *tailnetAPIConnector) dial() (proto.DRPCTailnetClient, error) {
	tac.logger.Debug(tac.ctx, "dialing Coder tailnet v2+ API")
	// nolint:bodyclose
	ws, res, err := websocket.Dial(tac.ctx, tac.coordinateURL, tac.dialOptions)
	if tac.isFirst {
		if res != nil && res.StatusCode == http.StatusConflict {
			err = codersdk.ReadBodyAsError(res)
			tac.connected <- err
			return nil, err
		}
		tac.isFirst = false
		close(tac.connected)
	}
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			tac.logger.Error(tac.ctx, "failed to dial tailnet v2+ API", slog.Error(err))
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

// coordinateAndDERPMap uses the provided client to coordinate and stream DERP Maps. It is combined
// into one function so that a problem with one tears down the other and triggers a retry (if
// appropriate). We multiplex both RPCs over the same websocket, so we want them to share the same
// fate.
func (tac *tailnetAPIConnector) coordinateAndDERPMap(client proto.DRPCTailnetClient) {
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
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		tac.coordinate(client)
	}()
	go func() {
		defer wg.Done()
		dErr := tac.derpMap(client)
		if dErr != nil && tac.ctx.Err() == nil {
			// The main context is still active, meaning that we want the tailnet data plane to stay
			// up, even though we hit some error getting DERP maps on the control plane.  That means
			// we do NOT want to gracefully disconnect on the coordinate() routine.  So, we'll just
			// close the underlying connection. This will trigger a retry of the control plane in
			// run().
			client.DRPCConn().Close()
			// Note that derpMap() logs it own errors, we don't bother here.
		}
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
		crdErr := coordination.Close()
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
			tac.logger.Error(tac.ctx, "error receiving DERP Map", slog.Error(err))
			return err
		}
		tac.logger.Debug(tac.ctx, "got new DERP Map", slog.F("derp_map", dmp))
		dm := tailnet.DERPMapFromProto(dmp)
		tac.conn.SetDERPMap(dm)
	}
}

// @typescript-ignore:WorkspaceAgentReconnectingPTYOpts
type WorkspaceAgentReconnectingPTYOpts struct {
	AgentID   uuid.UUID
	Reconnect uuid.UUID
	Width     uint16
	Height    uint16
	Command   string

	// SignedToken is an optional signed token from the
	// issue-reconnecting-pty-signed-token endpoint. If set, the session token
	// on the client will not be sent.
	SignedToken string
}

// AgentReconnectingPTY spawns a PTY that reconnects using the token provided.
// It communicates using `agent.ReconnectingPTYRequest` marshaled as JSON.
// Responses are PTY output that can be rendered.
func (c *Client) AgentReconnectingPTY(ctx context.Context, opts WorkspaceAgentReconnectingPTYOpts) (net.Conn, error) {
	serverURL, err := c.client.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/pty", opts.AgentID))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	q := serverURL.Query()
	q.Set("reconnect", opts.Reconnect.String())
	q.Set("width", strconv.Itoa(int(opts.Width)))
	q.Set("height", strconv.Itoa(int(opts.Height)))
	q.Set("command", opts.Command)
	// If we're using a signed token, set the query parameter.
	if opts.SignedToken != "" {
		q.Set(codersdk.SignedAppTokenQueryParameter, opts.SignedToken)
	}
	serverURL.RawQuery = q.Encode()

	// If we're not using a signed token, we need to set the session token as a
	// cookie.
	httpClient := c.client.HTTPClient
	if opts.SignedToken == "" {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, xerrors.Errorf("create cookie jar: %w", err)
		}
		jar.SetCookies(serverURL, []*http.Cookie{{
			Name:  codersdk.SessionTokenCookie,
			Value: c.client.SessionToken(),
		}})
		httpClient = &http.Client{
			Jar:       jar,
			Transport: c.client.HTTPClient.Transport,
		}
	}
	//nolint:bodyclose
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, codersdk.ReadBodyAsError(res)
	}
	return websocket.NetConn(context.Background(), conn, websocket.MessageBinary), nil
}
