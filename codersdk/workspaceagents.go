package codersdk

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
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/retry"
)

type WorkspaceAgentStatus string

// This is also in database/modelmethods.go and should be kept in sync.
const (
	WorkspaceAgentConnecting   WorkspaceAgentStatus = "connecting"
	WorkspaceAgentConnected    WorkspaceAgentStatus = "connected"
	WorkspaceAgentDisconnected WorkspaceAgentStatus = "disconnected"
	WorkspaceAgentTimeout      WorkspaceAgentStatus = "timeout"
)

// WorkspaceAgentLifecycle represents the lifecycle state of a workspace agent.
//
// The agent lifecycle starts in the "created" state, and transitions to
// "starting" when the agent reports it has begun preparing (e.g. started
// executing the startup script).
type WorkspaceAgentLifecycle string

// WorkspaceAgentLifecycle enums.
const (
	WorkspaceAgentLifecycleCreated         WorkspaceAgentLifecycle = "created"
	WorkspaceAgentLifecycleStarting        WorkspaceAgentLifecycle = "starting"
	WorkspaceAgentLifecycleStartTimeout    WorkspaceAgentLifecycle = "start_timeout"
	WorkspaceAgentLifecycleStartError      WorkspaceAgentLifecycle = "start_error"
	WorkspaceAgentLifecycleReady           WorkspaceAgentLifecycle = "ready"
	WorkspaceAgentLifecycleShuttingDown    WorkspaceAgentLifecycle = "shutting_down"
	WorkspaceAgentLifecycleShutdownTimeout WorkspaceAgentLifecycle = "shutdown_timeout"
	WorkspaceAgentLifecycleShutdownError   WorkspaceAgentLifecycle = "shutdown_error"
	WorkspaceAgentLifecycleOff             WorkspaceAgentLifecycle = "off"
)

// Starting returns true if the agent is in the process of starting.
func (l WorkspaceAgentLifecycle) Starting() bool {
	switch l {
	case WorkspaceAgentLifecycleCreated, WorkspaceAgentLifecycleStarting:
		return true
	default:
		return false
	}
}

// ShuttingDown returns true if the agent is in the process of shutting
// down or has shut down.
func (l WorkspaceAgentLifecycle) ShuttingDown() bool {
	switch l {
	case WorkspaceAgentLifecycleShuttingDown, WorkspaceAgentLifecycleShutdownTimeout, WorkspaceAgentLifecycleShutdownError, WorkspaceAgentLifecycleOff:
		return true
	default:
		return false
	}
}

// WorkspaceAgentLifecycleOrder is the order in which workspace agent
// lifecycle states are expected to be reported during the lifetime of
// the agent process. For instance, the agent can go from starting to
// ready without reporting timeout or error, but it should not go from
// ready to starting. This is merely a hint for the agent process, and
// is not enforced by the server.
var WorkspaceAgentLifecycleOrder = []WorkspaceAgentLifecycle{
	WorkspaceAgentLifecycleCreated,
	WorkspaceAgentLifecycleStarting,
	WorkspaceAgentLifecycleStartTimeout,
	WorkspaceAgentLifecycleStartError,
	WorkspaceAgentLifecycleReady,
	WorkspaceAgentLifecycleShuttingDown,
	WorkspaceAgentLifecycleShutdownTimeout,
	WorkspaceAgentLifecycleShutdownError,
	WorkspaceAgentLifecycleOff,
}

// WorkspaceAgentStartupScriptBehavior defines whether or not the startup script
// should be considered blocking or non-blocking. The blocking behavior means
// that the agent will not be considered ready until the startup script has
// completed and, for example, SSH connections will wait for the agent to be
// ready (can be overridden).
//
// Presently, non-blocking is the default, but this may change in the future.
// Deprecated: `coder_script` allows configuration on a per-script basis.
type WorkspaceAgentStartupScriptBehavior string

const (
	WorkspaceAgentStartupScriptBehaviorBlocking    WorkspaceAgentStartupScriptBehavior = "blocking"
	WorkspaceAgentStartupScriptBehaviorNonBlocking WorkspaceAgentStartupScriptBehavior = "non-blocking"
)

type WorkspaceAgentMetadataResult struct {
	CollectedAt time.Time `json:"collected_at" format:"date-time"`
	// Age is the number of seconds since the metadata was collected.
	// It is provided in addition to CollectedAt to protect against clock skew.
	Age   int64  `json:"age"`
	Value string `json:"value"`
	Error string `json:"error"`
}

// WorkspaceAgentMetadataDescription is a description of dynamic metadata the agent should report
// back to coderd. It is provided via the `metadata` list in the `coder_agent`
// block.
type WorkspaceAgentMetadataDescription struct {
	DisplayName string `json:"display_name"`
	Key         string `json:"key"`
	Script      string `json:"script"`
	Interval    int64  `json:"interval"`
	Timeout     int64  `json:"timeout"`
}

type WorkspaceAgentMetadata struct {
	Result      WorkspaceAgentMetadataResult      `json:"result"`
	Description WorkspaceAgentMetadataDescription `json:"description"`
}

type DisplayApp string

const (
	DisplayAppVSCodeDesktop  DisplayApp = "vscode"
	DisplayAppVSCodeInsiders DisplayApp = "vscode_insiders"
	DisplayAppWebTerminal    DisplayApp = "web_terminal"
	DisplayAppPortForward    DisplayApp = "port_forwarding_helper"
	DisplayAppSSH            DisplayApp = "ssh_helper"
)

type WorkspaceAgent struct {
	ID                   uuid.UUID               `json:"id" format:"uuid"`
	CreatedAt            time.Time               `json:"created_at" format:"date-time"`
	UpdatedAt            time.Time               `json:"updated_at" format:"date-time"`
	FirstConnectedAt     *time.Time              `json:"first_connected_at,omitempty" format:"date-time"`
	LastConnectedAt      *time.Time              `json:"last_connected_at,omitempty" format:"date-time"`
	DisconnectedAt       *time.Time              `json:"disconnected_at,omitempty" format:"date-time"`
	StartedAt            *time.Time              `json:"started_at,omitempty" format:"date-time"`
	ReadyAt              *time.Time              `json:"ready_at,omitempty" format:"date-time"`
	Status               WorkspaceAgentStatus    `json:"status"`
	LifecycleState       WorkspaceAgentLifecycle `json:"lifecycle_state"`
	Name                 string                  `json:"name"`
	ResourceID           uuid.UUID               `json:"resource_id" format:"uuid"`
	InstanceID           string                  `json:"instance_id,omitempty"`
	Architecture         string                  `json:"architecture"`
	EnvironmentVariables map[string]string       `json:"environment_variables"`
	OperatingSystem      string                  `json:"operating_system"`
	LogsLength           int32                   `json:"logs_length"`
	LogsOverflowed       bool                    `json:"logs_overflowed"`
	Directory            string                  `json:"directory,omitempty"`
	ExpandedDirectory    string                  `json:"expanded_directory,omitempty"`
	Version              string                  `json:"version"`
	APIVersion           string                  `json:"api_version"`
	Apps                 []WorkspaceApp          `json:"apps"`
	// DERPLatency is mapped by region name (e.g. "New York City", "Seattle").
	DERPLatency              map[string]DERPRegion     `json:"latency,omitempty"`
	ConnectionTimeoutSeconds int32                     `json:"connection_timeout_seconds"`
	TroubleshootingURL       string                    `json:"troubleshooting_url"`
	Subsystems               []AgentSubsystem          `json:"subsystems"`
	Health                   WorkspaceAgentHealth      `json:"health"` // Health reports the health of the agent.
	DisplayApps              []DisplayApp              `json:"display_apps"`
	LogSources               []WorkspaceAgentLogSource `json:"log_sources"`
	Scripts                  []WorkspaceAgentScript    `json:"scripts"`

	// StartupScriptBehavior is a legacy field that is deprecated in favor
	// of the `coder_script` resource. It's only referenced by old clients.
	// Deprecated: Remove in the future!
	StartupScriptBehavior WorkspaceAgentStartupScriptBehavior `json:"startup_script_behavior"`
}

type WorkspaceAgentLogSource struct {
	WorkspaceAgentID uuid.UUID `json:"workspace_agent_id" format:"uuid"`
	ID               uuid.UUID `json:"id" format:"uuid"`
	CreatedAt        time.Time `json:"created_at" format:"date-time"`
	DisplayName      string    `json:"display_name"`
	Icon             string    `json:"icon"`
}

type WorkspaceAgentScript struct {
	LogSourceID      uuid.UUID     `json:"log_source_id" format:"uuid"`
	LogPath          string        `json:"log_path"`
	Script           string        `json:"script"`
	Cron             string        `json:"cron"`
	RunOnStart       bool          `json:"run_on_start"`
	RunOnStop        bool          `json:"run_on_stop"`
	StartBlocksLogin bool          `json:"start_blocks_login"`
	Timeout          time.Duration `json:"timeout"`
}

type WorkspaceAgentHealth struct {
	Healthy bool   `json:"healthy" example:"false"`                              // Healthy is true if the agent is healthy.
	Reason  string `json:"reason,omitempty" example:"agent has lost connection"` // Reason is a human-readable explanation of the agent's health. It is empty if Healthy is true.
}

type DERPRegion struct {
	Preferred           bool    `json:"preferred"`
	LatencyMilliseconds float64 `json:"latency_ms"`
}

// WorkspaceAgentConnectionInfo returns required information for establishing
// a connection with a workspace.
// @typescript-ignore WorkspaceAgentConnectionInfo
type WorkspaceAgentConnectionInfo struct {
	DERPMap                  *tailcfg.DERPMap `json:"derp_map"`
	DERPForceWebSockets      bool             `json:"derp_force_websockets"`
	DisableDirectConnections bool             `json:"disable_direct_connections"`
}

func (c *Client) WorkspaceAgentConnectionInfoGeneric(ctx context.Context) (WorkspaceAgentConnectionInfo, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/connection", nil)
	if err != nil {
		return WorkspaceAgentConnectionInfo{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentConnectionInfo{}, ReadBodyAsError(res)
	}

	var connInfo WorkspaceAgentConnectionInfo
	return connInfo, json.NewDecoder(res.Body).Decode(&connInfo)
}

func (c *Client) WorkspaceAgentConnectionInfo(ctx context.Context, agentID uuid.UUID) (WorkspaceAgentConnectionInfo, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/connection", agentID), nil)
	if err != nil {
		return WorkspaceAgentConnectionInfo{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentConnectionInfo{}, ReadBodyAsError(res)
	}

	var connInfo WorkspaceAgentConnectionInfo
	return connInfo, json.NewDecoder(res.Body).Decode(&connInfo)
}

// @typescript-ignore DialWorkspaceAgentOptions
type DialWorkspaceAgentOptions struct {
	Logger slog.Logger
	// BlockEndpoints forced a direct connection through DERP. The Client may
	// have DisableDirect set which will override this value.
	BlockEndpoints bool
}

func (c *Client) DialWorkspaceAgent(dialCtx context.Context, agentID uuid.UUID, options *DialWorkspaceAgentOptions) (agentConn *WorkspaceAgentConn, err error) {
	if options == nil {
		options = &DialWorkspaceAgentOptions{}
	}

	connInfo, err := c.WorkspaceAgentConnectionInfo(dialCtx, agentID)
	if err != nil {
		return nil, xerrors.Errorf("get connection info: %w", err)
	}
	if connInfo.DisableDirectConnections {
		options.BlockEndpoints = true
	}

	ip := tailnet.IP()
	var header http.Header
	if headerTransport, ok := c.HTTPClient.Transport.(*HeaderTransport); ok {
		header = headerTransport.Header
	}
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:           []netip.Prefix{netip.PrefixFrom(ip, 128)},
		DERPMap:             connInfo.DERPMap,
		DERPHeader:          &header,
		DERPForceWebSockets: connInfo.DERPForceWebSockets,
		Logger:              options.Logger,
		BlockEndpoints:      c.DisableDirectConnections || options.BlockEndpoints,
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
	tokenHeader := SessionTokenHeader
	if c.SessionTokenHeader != "" {
		tokenHeader = c.SessionTokenHeader
	}
	headers.Set(tokenHeader, c.SessionToken())

	// New context, separate from dialCtx. We don't want to cancel the
	// connection if dialCtx is canceled.
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	coordinateURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/coordinate", agentID))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	q := coordinateURL.Query()
	q.Add("version", proto.CurrentVersion.String())
	coordinateURL.RawQuery = q.Encode()

	connector := runTailnetAPIConnector(ctx, options.Logger,
		agentID, coordinateURL.String(),
		&websocket.DialOptions{
			HTTPClient: c.HTTPClient,
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

	agentConn = NewWorkspaceAgentConn(conn, WorkspaceAgentConnOptions{
		AgentID: agentID,
		// Newer agents will listen on two IPs: WorkspaceAgentIP and an IP
		// derived from the agents UUID. We need to use the legacy
		// WorkspaceAgentIP here since we don't know if the agent is listening
		// on the new IP.
		AgentIP: WorkspaceAgentIP,
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
	ctx    context.Context
	logger slog.Logger

	agentID       uuid.UUID
	coordinateURL string
	dialOptions   *websocket.DialOptions
	conn          *tailnet.Conn

	connected chan error
	isFirst   bool
	closed    chan struct{}
}

// runTailnetAPIConnector creates and runs a tailnetAPIConnector
func runTailnetAPIConnector(
	ctx context.Context, logger slog.Logger,
	agentID uuid.UUID, coordinateURL string, dialOptions *websocket.DialOptions,
	conn *tailnet.Conn,
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
	go tac.run()
	return tac
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
			err = ReadBodyAsError(res)
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
	client, err := tailnet.NewDRPCClient(websocket.NetConn(tac.ctx, ws, websocket.MessageBinary))
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
	eg, egCtx := errgroup.WithContext(tac.ctx)
	eg.Go(func() error {
		return tac.coordinate(egCtx, client)
	})
	eg.Go(func() error {
		return tac.derpMap(egCtx, client)
	})
	err := eg.Wait()
	if err != nil &&
		!xerrors.Is(err, io.EOF) &&
		!xerrors.Is(err, context.Canceled) &&
		!xerrors.Is(err, context.DeadlineExceeded) {
		tac.logger.Error(tac.ctx, "error while connected to tailnet v2+ API")
	}
}

func (tac *tailnetAPIConnector) coordinate(ctx context.Context, client proto.DRPCTailnetClient) error {
	coord, err := client.Coordinate(ctx)
	if err != nil {
		return xerrors.Errorf("failed to connect to Coordinate RPC: %w", err)
	}
	defer func() {
		cErr := coord.Close()
		if cErr != nil {
			tac.logger.Debug(ctx, "error closing Coordinate RPC", slog.Error(cErr))
		}
	}()
	coordination := tailnet.NewRemoteCoordination(tac.logger, coord, tac.conn, tac.agentID)
	tac.logger.Debug(ctx, "serving coordinator")
	err = <-coordination.Error()
	if err != nil &&
		!xerrors.Is(err, io.EOF) &&
		!xerrors.Is(err, context.Canceled) &&
		!xerrors.Is(err, context.DeadlineExceeded) {
		return xerrors.Errorf("remote coordination error: %w", err)
	}
	return nil
}

func (tac *tailnetAPIConnector) derpMap(ctx context.Context, client proto.DRPCTailnetClient) error {
	s, err := client.StreamDERPMaps(ctx, &proto.StreamDERPMapsRequest{})
	if err != nil {
		return xerrors.Errorf("failed to connect to StreamDERPMaps RPC: %w", err)
	}
	defer func() {
		cErr := s.Close()
		if cErr != nil {
			tac.logger.Debug(ctx, "error closing StreamDERPMaps RPC", slog.Error(cErr))
		}
	}()
	for {
		dmp, err := s.Recv()
		if err != nil {
			if xerrors.Is(err, io.EOF) || xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return xerrors.Errorf("error receiving DERP Map: %w", err)
		}
		tac.logger.Debug(ctx, "got new DERP Map", slog.F("derp_map", dmp))
		dm := tailnet.DERPMapFromProto(dmp)
		tac.conn.SetDERPMap(dm)
	}
}

// WatchWorkspaceAgentMetadata watches the metadata of a workspace agent.
// The returned channel will be closed when the context is canceled. Exactly
// one error will be sent on the error channel. The metadata channel is never closed.
func (c *Client) WatchWorkspaceAgentMetadata(ctx context.Context, id uuid.UUID) (<-chan []WorkspaceAgentMetadata, <-chan error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	metadataChan := make(chan []WorkspaceAgentMetadata, 256)

	ready := make(chan struct{})
	watch := func() error {
		res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/watch-metadata", id), nil)
		if err != nil {
			return err
		}
		if res.StatusCode != http.StatusOK {
			return ReadBodyAsError(res)
		}

		nextEvent := ServerSentEventReader(ctx, res.Body)
		defer res.Body.Close()

		firstEvent := true
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			sse, err := nextEvent()
			if err != nil {
				return err
			}

			if firstEvent {
				close(ready) // Only close ready after the first event is received.
				firstEvent = false
			}

			// Ignore pings.
			if sse.Type == ServerSentEventTypePing {
				continue
			}

			b, ok := sse.Data.([]byte)
			if !ok {
				return xerrors.Errorf("unexpected data type: %T", sse.Data)
			}

			switch sse.Type {
			case ServerSentEventTypeData:
				var met []WorkspaceAgentMetadata
				err = json.Unmarshal(b, &met)
				if err != nil {
					return xerrors.Errorf("unmarshal metadata: %w", err)
				}
				metadataChan <- met
			case ServerSentEventTypeError:
				var r Response
				err = json.Unmarshal(b, &r)
				if err != nil {
					return xerrors.Errorf("unmarshal error: %w", err)
				}
				return xerrors.Errorf("%+v", r)
			default:
				return xerrors.Errorf("unexpected event type: %s", sse.Type)
			}
		}
	}

	errorChan := make(chan error, 1)
	go func() {
		defer close(errorChan)
		err := watch()
		select {
		case <-ready:
		default:
			close(ready) // Error before first event.
		}
		errorChan <- err
	}()

	// Wait until first event is received and the subscription is registered.
	<-ready

	return metadataChan, errorChan
}

// WorkspaceAgent returns an agent by ID.
func (c *Client) WorkspaceAgent(ctx context.Context, id uuid.UUID) (WorkspaceAgent, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s", id), nil)
	if err != nil {
		return WorkspaceAgent{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgent{}, ReadBodyAsError(res)
	}
	var workspaceAgent WorkspaceAgent
	err = json.NewDecoder(res.Body).Decode(&workspaceAgent)
	if err != nil {
		return WorkspaceAgent{}, err
	}
	return workspaceAgent, nil
}

type IssueReconnectingPTYSignedTokenRequest struct {
	// URL is the URL of the reconnecting-pty endpoint you are connecting to.
	URL     string    `json:"url" validate:"required"`
	AgentID uuid.UUID `json:"agentID" format:"uuid" validate:"required"`
}

type IssueReconnectingPTYSignedTokenResponse struct {
	SignedToken string `json:"signed_token"`
}

func (c *Client) IssueReconnectingPTYSignedToken(ctx context.Context, req IssueReconnectingPTYSignedTokenRequest) (IssueReconnectingPTYSignedTokenResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/applications/reconnecting-pty-signed-token", req)
	if err != nil {
		return IssueReconnectingPTYSignedTokenResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return IssueReconnectingPTYSignedTokenResponse{}, ReadBodyAsError(res)
	}
	var resp IssueReconnectingPTYSignedTokenResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
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

// WorkspaceAgentReconnectingPTY spawns a PTY that reconnects using the token provided.
// It communicates using `agent.ReconnectingPTYRequest` marshaled as JSON.
// Responses are PTY output that can be rendered.
func (c *Client) WorkspaceAgentReconnectingPTY(ctx context.Context, opts WorkspaceAgentReconnectingPTYOpts) (net.Conn, error) {
	serverURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/pty", opts.AgentID))
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
		q.Set(SignedAppTokenQueryParameter, opts.SignedToken)
	}
	serverURL.RawQuery = q.Encode()

	// If we're not using a signed token, we need to set the session token as a
	// cookie.
	httpClient := c.HTTPClient
	if opts.SignedToken == "" {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, xerrors.Errorf("create cookie jar: %w", err)
		}
		jar.SetCookies(serverURL, []*http.Cookie{{
			Name:  SessionTokenCookie,
			Value: c.SessionToken(),
		}})
		httpClient = &http.Client{
			Jar:       jar,
			Transport: c.HTTPClient.Transport,
		}
	}
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, ReadBodyAsError(res)
	}
	return websocket.NetConn(context.Background(), conn, websocket.MessageBinary), nil
}

// WorkspaceAgentListeningPorts returns a list of ports that are currently being
// listened on inside the workspace agent's network namespace.
func (c *Client) WorkspaceAgentListeningPorts(ctx context.Context, agentID uuid.UUID) (WorkspaceAgentListeningPortsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/listening-ports", agentID), nil)
	if err != nil {
		return WorkspaceAgentListeningPortsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentListeningPortsResponse{}, ReadBodyAsError(res)
	}
	var listeningPorts WorkspaceAgentListeningPortsResponse
	return listeningPorts, json.NewDecoder(res.Body).Decode(&listeningPorts)
}

//nolint:revive // Follow is a control flag on the server as well.
func (c *Client) WorkspaceAgentLogsAfter(ctx context.Context, agentID uuid.UUID, after int64, follow bool) (<-chan []WorkspaceAgentLog, io.Closer, error) {
	var queryParams []string
	if after != 0 {
		queryParams = append(queryParams, fmt.Sprintf("after=%d", after))
	}
	if follow {
		queryParams = append(queryParams, "follow")
	}
	var query string
	if len(queryParams) > 0 {
		query = "?" + strings.Join(queryParams, "&")
	}
	reqURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/logs%s", agentID, query))
	if err != nil {
		return nil, nil, err
	}

	if !follow {
		resp, err := c.Request(ctx, http.MethodGet, reqURL.String(), nil)
		if err != nil {
			return nil, nil, xerrors.Errorf("execute request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, nil, ReadBodyAsError(resp)
		}

		var logs []WorkspaceAgentLog
		err = json.NewDecoder(resp.Body).Decode(&logs)
		if err != nil {
			return nil, nil, xerrors.Errorf("decode startup logs: %w", err)
		}

		ch := make(chan []WorkspaceAgentLog, 1)
		ch <- logs
		close(ch)
		return ch, closeFunc(func() error { return nil }), nil
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(reqURL, []*http.Cookie{{
		Name:  SessionTokenCookie,
		Value: c.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar:       jar,
		Transport: c.HTTPClient.Transport,
	}
	conn, res, err := websocket.Dial(ctx, reqURL.String(), &websocket.DialOptions{
		HTTPClient:      httpClient,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if res == nil {
			return nil, nil, err
		}
		return nil, nil, ReadBodyAsError(res)
	}
	logChunks := make(chan []WorkspaceAgentLog, 1)
	closed := make(chan struct{})
	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageText)
	decoder := json.NewDecoder(wsNetConn)
	go func() {
		defer close(closed)
		defer close(logChunks)
		defer conn.Close(websocket.StatusGoingAway, "")
		for {
			var logs []WorkspaceAgentLog
			err = decoder.Decode(&logs)
			if err != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case logChunks <- logs:
			}
		}
	}()
	return logChunks, closeFunc(func() error {
		_ = wsNetConn.Close()
		<-closed
		return nil
	}), nil
}

type WorkspaceAgentLog struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	Output    string    `json:"output"`
	Level     LogLevel  `json:"level"`
	SourceID  uuid.UUID `json:"source_id" format:"uuid"`
}

type AgentSubsystem string

const (
	AgentSubsystemEnvbox     AgentSubsystem = "envbox"
	AgentSubsystemEnvbuilder AgentSubsystem = "envbuilder"
	AgentSubsystemExectrace  AgentSubsystem = "exectrace"
)

func (s AgentSubsystem) Valid() bool {
	switch s {
	case AgentSubsystemEnvbox, AgentSubsystemEnvbuilder, AgentSubsystemExectrace:
		return true
	default:
		return false
	}
}
