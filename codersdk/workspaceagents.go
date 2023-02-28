package codersdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/coder/coder/tailnet"
	"github.com/coder/retry"
)

type WorkspaceAgentStatus string

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
	WorkspaceAgentLifecycleCreated      WorkspaceAgentLifecycle = "created"
	WorkspaceAgentLifecycleStarting     WorkspaceAgentLifecycle = "starting"
	WorkspaceAgentLifecycleStartTimeout WorkspaceAgentLifecycle = "start_timeout"
	WorkspaceAgentLifecycleStartError   WorkspaceAgentLifecycle = "start_error"
	WorkspaceAgentLifecycleReady        WorkspaceAgentLifecycle = "ready"
)

type WorkspaceAgent struct {
	ID                   uuid.UUID               `json:"id" format:"uuid"`
	CreatedAt            time.Time               `json:"created_at" format:"date-time"`
	UpdatedAt            time.Time               `json:"updated_at" format:"date-time"`
	FirstConnectedAt     *time.Time              `json:"first_connected_at,omitempty" format:"date-time"`
	LastConnectedAt      *time.Time              `json:"last_connected_at,omitempty" format:"date-time"`
	DisconnectedAt       *time.Time              `json:"disconnected_at,omitempty" format:"date-time"`
	Status               WorkspaceAgentStatus    `json:"status"`
	LifecycleState       WorkspaceAgentLifecycle `json:"lifecycle_state"`
	Name                 string                  `json:"name"`
	ResourceID           uuid.UUID               `json:"resource_id" format:"uuid"`
	InstanceID           string                  `json:"instance_id,omitempty"`
	Architecture         string                  `json:"architecture"`
	EnvironmentVariables map[string]string       `json:"environment_variables"`
	OperatingSystem      string                  `json:"operating_system"`
	StartupScript        string                  `json:"startup_script,omitempty"`
	Directory            string                  `json:"directory,omitempty"`
	ExpandedDirectory    string                  `json:"expanded_directory,omitempty"`
	Version              string                  `json:"version"`
	Apps                 []WorkspaceApp          `json:"apps"`
	// DERPLatency is mapped by region name (e.g. "New York City", "Seattle").
	DERPLatency              map[string]DERPRegion `json:"latency,omitempty"`
	ConnectionTimeoutSeconds int32                 `json:"connection_timeout_seconds"`
	TroubleshootingURL       string                `json:"troubleshooting_url"`
	// LoginBeforeReady if true, the agent will delay logins until it is ready (e.g. executing startup script has ended).
	LoginBeforeReady bool `db:"login_before_ready" json:"login_before_ready"`
	// StartupScriptTimeoutSeconds is the number of seconds to wait for the startup script to complete. If the script does not complete within this time, the agent lifecycle will be marked as start_timeout.
	StartupScriptTimeoutSeconds int32 `db:"startup_script_timeout_seconds" json:"startup_script_timeout_seconds"`
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
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:      []netip.Prefix{netip.PrefixFrom(ip, 128)},
		DERPMap:        connInfo.DERPMap,
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
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(coordinateURL, []*http.Cookie{{
		Name:  SessionTokenCookie,
		Value: c.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar:       jar,
		Transport: c.HTTPClient.Transport,
	}
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
				HTTPClient: httpClient,
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

// WorkspaceAgentReconnectingPTY spawns a PTY that reconnects using the token provided.
// It communicates using `agent.ReconnectingPTYRequest` marshaled as JSON.
// Responses are PTY output that can be rendered.
func (c *Client) WorkspaceAgentReconnectingPTY(ctx context.Context, agentID, reconnect uuid.UUID, height, width uint16, command string) (net.Conn, error) {
	serverURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/pty", agentID))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	q := serverURL.Query()
	q.Set("reconnect", reconnect.String())
	q.Set("height", strconv.Itoa(int(height)))
	q.Set("width", strconv.Itoa(int(width)))
	q.Set("command", command)
	serverURL.RawQuery = q.Encode()

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  SessionTokenCookie,
		Value: c.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar: jar,
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
