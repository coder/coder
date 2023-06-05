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
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/tailnet"
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
//
// Note that states are not guaranteed to be reported, for instance the agent
// may go from "created" to "ready" without reporting "starting", if it had
// trouble connecting on startup.
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

type WorkspaceAgent struct {
	ID                    uuid.UUID               `json:"id" format:"uuid"`
	CreatedAt             time.Time               `json:"created_at" format:"date-time"`
	UpdatedAt             time.Time               `json:"updated_at" format:"date-time"`
	FirstConnectedAt      *time.Time              `json:"first_connected_at,omitempty" format:"date-time"`
	LastConnectedAt       *time.Time              `json:"last_connected_at,omitempty" format:"date-time"`
	DisconnectedAt        *time.Time              `json:"disconnected_at,omitempty" format:"date-time"`
	Status                WorkspaceAgentStatus    `json:"status"`
	LifecycleState        WorkspaceAgentLifecycle `json:"lifecycle_state"`
	Name                  string                  `json:"name"`
	ResourceID            uuid.UUID               `json:"resource_id" format:"uuid"`
	InstanceID            string                  `json:"instance_id,omitempty"`
	Architecture          string                  `json:"architecture"`
	EnvironmentVariables  map[string]string       `json:"environment_variables"`
	OperatingSystem       string                  `json:"operating_system"`
	StartupScript         string                  `json:"startup_script,omitempty"`
	StartupLogsLength     int32                   `json:"startup_logs_length"`
	StartupLogsOverflowed bool                    `json:"startup_logs_overflowed"`
	Directory             string                  `json:"directory,omitempty"`
	ExpandedDirectory     string                  `json:"expanded_directory,omitempty"`
	Version               string                  `json:"version"`
	Apps                  []WorkspaceApp          `json:"apps"`
	// DERPLatency is mapped by region name (e.g. "New York City", "Seattle").
	DERPLatency              map[string]DERPRegion `json:"latency,omitempty"`
	ConnectionTimeoutSeconds int32                 `json:"connection_timeout_seconds"`
	TroubleshootingURL       string                `json:"troubleshooting_url"`
	// Deprecated: Use StartupScriptBehavior instead.
	LoginBeforeReady      bool                                `json:"login_before_ready"`
	StartupScriptBehavior WorkspaceAgentStartupScriptBehavior `json:"startup_script_behavior"`
	// StartupScriptTimeoutSeconds is the number of seconds to wait for the startup script to complete. If the script does not complete within this time, the agent lifecycle will be marked as start_timeout.
	StartupScriptTimeoutSeconds  int32          `json:"startup_script_timeout_seconds"`
	ShutdownScript               string         `json:"shutdown_script,omitempty"`
	ShutdownScriptTimeoutSeconds int32          `json:"shutdown_script_timeout_seconds"`
	Subsystem                    AgentSubsystem `json:"subsystem"`
}

type DERPRegion struct {
	Preferred           bool    `json:"preferred"`
	LatencyMilliseconds float64 `json:"latency_ms"`
}

// WorkspaceAgentConnectionInfo returns required information for establishing
// a connection with a workspace.
// @typescript-ignore WorkspaceAgentConnectionInfo
type WorkspaceAgentConnectionInfo struct {
	DERPMap *tailcfg.DERPMap `json:"derp_map"`
}

// @typescript-ignore DialWorkspaceAgentOptions
type DialWorkspaceAgentOptions struct {
	Logger slog.Logger
	// BlockEndpoints forced a direct connection through DERP.
	BlockEndpoints bool
}

func (c *Client) DialWorkspaceAgent(ctx context.Context, agentID uuid.UUID, options *DialWorkspaceAgentOptions) (agentConn *WorkspaceAgentConn, err error) {
	if options == nil {
		options = &DialWorkspaceAgentOptions{}
	}
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/connection", agentID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var connInfo WorkspaceAgentConnectionInfo
	err = json.NewDecoder(res.Body).Decode(&connInfo)
	if err != nil {
		return nil, xerrors.Errorf("decode conn info: %w", err)
	}

	ip := tailnet.IP()
	var header http.Header
	headerTransport, ok := c.HTTPClient.Transport.(interface {
		Header() http.Header
	})
	if ok {
		header = headerTransport.Header()
	}
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:      []netip.Prefix{netip.PrefixFrom(ip, 128)},
		DERPMap:        connInfo.DERPMap,
		DERPHeader:     &header,
		Logger:         options.Logger,
		BlockEndpoints: options.BlockEndpoints,
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet: %w", err)
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()

	coordinateURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/coordinate", agentID))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	coordinateHeaders := make(http.Header)
	tokenHeader := SessionTokenHeader
	if c.SessionTokenHeader != "" {
		tokenHeader = c.SessionTokenHeader
	}
	coordinateHeaders.Set(tokenHeader, c.SessionToken())
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if err != nil {
			cancel()
		}
	}()
	closed := make(chan struct{})
	first := make(chan error)
	go func() {
		defer close(closed)
		isFirst := true
		for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
			options.Logger.Debug(ctx, "connecting")
			// nolint:bodyclose
			ws, res, err := websocket.Dial(ctx, coordinateURL.String(), &websocket.DialOptions{
				HTTPClient: c.HTTPClient,
				HTTPHeader: coordinateHeaders,
				// Need to disable compression to avoid a data-race.
				CompressionMode: websocket.CompressionDisabled,
			})
			if isFirst {
				if res != nil && res.StatusCode == http.StatusConflict {
					first <- ReadBodyAsError(res)
					return
				}
				isFirst = false
				close(first)
			}
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				options.Logger.Debug(ctx, "failed to dial", slog.Error(err))
				continue
			}
			sendNode, errChan := tailnet.ServeCoordinator(websocket.NetConn(ctx, ws, websocket.MessageBinary), func(node []*tailnet.Node) error {
				return conn.UpdateNodes(node, false)
			})
			conn.SetNodeCallback(sendNode)
			options.Logger.Debug(ctx, "serving coordinator")
			err = <-errChan
			if errors.Is(err, context.Canceled) {
				_ = ws.Close(websocket.StatusGoingAway, "")
				return
			}
			if err != nil {
				options.Logger.Debug(ctx, "error serving coordinator", slog.Error(err))
				_ = ws.Close(websocket.StatusGoingAway, "")
				continue
			}
			_ = ws.Close(websocket.StatusGoingAway, "")
		}
	}()
	err = <-first
	if err != nil {
		return nil, err
	}

	agentConn = &WorkspaceAgentConn{
		Conn: conn,
		CloseFunc: func() {
			cancel()
			<-closed
		},
	}
	if !agentConn.AwaitReachable(ctx) {
		_ = agentConn.Close()
		return nil, xerrors.Errorf("timed out waiting for agent to become reachable: %w", ctx.Err())
	}

	return agentConn, nil
}

// WatchWorkspaceAgentMetadata watches the metadata of a workspace agent.
// The returned channel will be closed when the context is canceled. Exactly
// one error will be sent on the error channel. The metadata channel is never closed.
func (c *Client) WatchWorkspaceAgentMetadata(ctx context.Context, id uuid.UUID) (<-chan []WorkspaceAgentMetadata, <-chan error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	metadataChan := make(chan []WorkspaceAgentMetadata, 256)

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

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				break
			}

			sse, err := nextEvent()
			if err != nil {
				return err
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
		errorChan <- watch()
	}()

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
	return workspaceAgent, json.NewDecoder(res.Body).Decode(&workspaceAgent)
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

func (c *Client) WorkspaceAgentStartupLogsAfter(ctx context.Context, agentID uuid.UUID, after int64) (<-chan []WorkspaceAgentStartupLog, io.Closer, error) {
	afterQuery := ""
	if after != 0 {
		afterQuery = fmt.Sprintf("&after=%d", after)
	}
	followURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/startup-logs?follow%s", agentID, afterQuery))
	if err != nil {
		return nil, nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(followURL, []*http.Cookie{{
		Name:  SessionTokenCookie,
		Value: c.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar:       jar,
		Transport: c.HTTPClient.Transport,
	}
	conn, res, err := websocket.Dial(ctx, followURL.String(), &websocket.DialOptions{
		HTTPClient:      httpClient,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if res == nil {
			return nil, nil, err
		}
		return nil, nil, ReadBodyAsError(res)
	}
	logChunks := make(chan []WorkspaceAgentStartupLog)
	closed := make(chan struct{})
	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageText)
	decoder := json.NewDecoder(wsNetConn)
	go func() {
		defer close(closed)
		defer close(logChunks)
		defer conn.Close(websocket.StatusGoingAway, "")
		var logs []WorkspaceAgentStartupLog
		for {
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

// GitProvider is a constant that represents the
// type of providers that are supported within Coder.
type GitProvider string

func (g GitProvider) Pretty() string {
	switch g {
	case GitProviderAzureDevops:
		return "Azure DevOps"
	case GitProviderGitHub:
		return "GitHub"
	case GitProviderGitLab:
		return "GitLab"
	case GitProviderBitBucket:
		return "Bitbucket"
	default:
		return string(g)
	}
}

const (
	GitProviderAzureDevops GitProvider = "azure-devops"
	GitProviderGitHub      GitProvider = "github"
	GitProviderGitLab      GitProvider = "gitlab"
	GitProviderBitBucket   GitProvider = "bitbucket"
)

type WorkspaceAgentStartupLog struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	Output    string    `json:"output"`
	Level     LogLevel  `json:"level"`
}

type AgentSubsystem string

const (
	AgentSubsystemEnvbox AgentSubsystem = "envbox"
)
