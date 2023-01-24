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
	"net/url"
	"strconv"
	"time"

	"cloud.google.com/go/compute/metadata"
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
	Version              string                  `json:"version"`
	Apps                 []WorkspaceApp          `json:"apps"`
	// DERPLatency is mapped by region name (e.g. "New York City", "Seattle").
	DERPLatency              map[string]DERPRegion `json:"latency,omitempty"`
	ConnectionTimeoutSeconds int32                 `json:"connection_timeout_seconds"`
	TroubleshootingURL       string                `json:"troubleshooting_url"`
	// DelayLoginUntilReady if true, the agent will delay logins until it is ready (e.g. executing startup script has ended).
	DelayLoginUntilReady bool `db:"delay_login_until_ready" json:"delay_login_until_ready"`
	// StartupScriptTimeoutSeconds is the number of seconds to wait for the startup script to complete. If the script does not complete within this time, the agent lifecycle will be marked as start_timeout.
	StartupScriptTimeoutSeconds int32 `db:"startup_script_timeout_seconds" json:"startup_script_timeout_seconds"`
}

type WorkspaceAgentResourceMetadata struct {
	MemoryTotal uint64  `json:"memory_total"`
	DiskTotal   uint64  `json:"disk_total"`
	CPUCores    uint64  `json:"cpu_cores"`
	CPUModel    string  `json:"cpu_model"`
	CPUMhz      float64 `json:"cpu_mhz"`
}

type DERPRegion struct {
	Preferred           bool    `json:"preferred"`
	LatencyMilliseconds float64 `json:"latency_ms"`
}

type WorkspaceAgentInstanceMetadata struct {
	JailOrchestrator   string `json:"jail_orchestrator"`
	OperatingSystem    string `json:"operating_system"`
	Platform           string `json:"platform"`
	PlatformFamily     string `json:"platform_family"`
	KernelVersion      string `json:"kernel_version"`
	KernelArchitecture string `json:"kernel_architecture"`
	Cloud              string `json:"cloud"`
	Jail               string `json:"jail"`
	VNC                bool   `json:"vnc"`
}

// @typescript-ignore GoogleInstanceIdentityToken
type GoogleInstanceIdentityToken struct {
	JSONWebToken string `json:"json_web_token" validate:"required"`
}

// @typescript-ignore AWSInstanceIdentityToken
type AWSInstanceIdentityToken struct {
	Signature string `json:"signature" validate:"required"`
	Document  string `json:"document" validate:"required"`
}

// @typescript-ignore ReconnectingPTYRequest
type AzureInstanceIdentityToken struct {
	Signature string `json:"signature" validate:"required"`
	Encoding  string `json:"encoding" validate:"required"`
}

// WorkspaceAgentAuthenticateResponse is returned when an instance ID
// has been exchanged for a session token.
// @typescript-ignore WorkspaceAgentAuthenticateResponse
type WorkspaceAgentAuthenticateResponse struct {
	SessionToken string `json:"session_token"`
}

// WorkspaceAgentConnectionInfo returns required information for establishing
// a connection with a workspace.
// @typescript-ignore WorkspaceAgentConnectionInfo
type WorkspaceAgentConnectionInfo struct {
	DERPMap *tailcfg.DERPMap `json:"derp_map"`
}

// @typescript-ignore PostWorkspaceAgentVersionRequest
// @Description x-apidocgen:skip
type PostWorkspaceAgentVersionRequest struct {
	Version string `json:"version"`
}

// @typescript-ignore WorkspaceAgentMetadata
type WorkspaceAgentMetadata struct {
	// GitAuthConfigs stores the number of Git configurations
	// the Coder deployment has. If this number is >0, we
	// set up special configuration in the workspace.
	GitAuthConfigs       int               `json:"git_auth_configs"`
	VSCodePortProxyURI   string            `json:"vscode_port_proxy_uri"`
	Apps                 []WorkspaceApp    `json:"apps"`
	DERPMap              *tailcfg.DERPMap  `json:"derpmap"`
	EnvironmentVariables map[string]string `json:"environment_variables"`
	StartupScript        string            `json:"startup_script"`
	StartupScriptTimeout time.Duration     `json:"startup_script_timeout"`
	Directory            string            `json:"directory"`
	MOTDFile             string            `json:"motd_file"`
}

// AuthWorkspaceGoogleInstanceIdentity uses the Google Compute Engine Metadata API to
// fetch a signed JWT, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthWorkspaceGoogleInstanceIdentity(ctx context.Context, serviceAccount string, gcpClient *metadata.Client) (WorkspaceAgentAuthenticateResponse, error) {
	if serviceAccount == "" {
		// This is the default name specified by Google.
		serviceAccount = "default"
	}
	if gcpClient == nil {
		gcpClient = metadata.NewClient(c.HTTPClient)
	}
	// "format=full" is required, otherwise the responding payload will be missing "instance_id".
	jwt, err := gcpClient.Get(fmt.Sprintf("instance/service-accounts/%s/identity?audience=coder&format=full", serviceAccount))
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("get metadata identity: %w", err)
	}
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/google-instance-identity", GoogleInstanceIdentityToken{
		JSONWebToken: jwt,
	})
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// AuthWorkspaceAWSInstanceIdentity uses the Amazon Metadata API to
// fetch a signed payload, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthWorkspaceAWSInstanceIdentity(ctx context.Context) (WorkspaceAgentAuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	token, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/signature", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	signature, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/document", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	document, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	res, err = c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/aws-instance-identity", AWSInstanceIdentityToken{
		Signature: string(signature),
		Document:  string(document),
	})
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// AuthWorkspaceAzureInstanceIdentity uses the Azure Instance Metadata Service to
// fetch a signed payload, and exchange it for a session token for a workspace agent.
func (c *Client) AuthWorkspaceAzureInstanceIdentity(ctx context.Context) (WorkspaceAgentAuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/metadata/attested/document?api-version=2020-09-01", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("Metadata", "true")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()

	var token AzureInstanceIdentityToken
	err = json.NewDecoder(res.Body).Decode(&token)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}

	res, err = c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/azure-instance-identity", token)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// WorkspaceAgentMetadata fetches metadata for the currently authenticated workspace agent.
func (c *Client) WorkspaceAgentMetadata(ctx context.Context) (WorkspaceAgentMetadata, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/metadata", nil)
	if err != nil {
		return WorkspaceAgentMetadata{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentMetadata{}, readBodyAsError(res)
	}
	var agentMetadata WorkspaceAgentMetadata
	err = json.NewDecoder(res.Body).Decode(&agentMetadata)
	if err != nil {
		return WorkspaceAgentMetadata{}, err
	}
	accessingPort := c.URL.Port()
	if accessingPort == "" {
		accessingPort = "80"
		if c.URL.Scheme == "https" {
			accessingPort = "443"
		}
	}
	accessPort, err := strconv.Atoi(accessingPort)
	if err != nil {
		return WorkspaceAgentMetadata{}, xerrors.Errorf("convert accessing port %q: %w", accessingPort, err)
	}
	// Agents can provide an arbitrary access URL that may be different
	// that the globally configured one. This breaks the built-in DERP,
	// which would continue to reference the global access URL.
	//
	// This converts all built-in DERPs to use the access URL that the
	// metadata request was performed with.
	for _, region := range agentMetadata.DERPMap.Regions {
		if !region.EmbeddedRelay {
			continue
		}

		for _, node := range region.Nodes {
			if node.STUNOnly {
				continue
			}
			node.HostName = c.URL.Hostname()
			node.DERPPort = accessPort
			node.ForceHTTP = c.URL.Scheme == "http"
		}
	}
	return agentMetadata, nil
}

func (c *Client) ListenWorkspaceAgent(ctx context.Context) (net.Conn, error) {
	coordinateURL, err := c.URL.Parse("/api/v2/workspaceagents/me/coordinate")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(coordinateURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar:       jar,
		Transport: c.HTTPClient.Transport,
	}
	// nolint:bodyclose
	conn, res, err := websocket.Dial(ctx, coordinateURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, readBodyAsError(res)
	}

	// Ping once every 30 seconds to ensure that the websocket is alive. If we
	// don't get a response within 30s we kill the websocket and reconnect.
	// See: https://github.com/coder/coder/pull/5824
	go func() {
		tick := 30 * time.Second
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		defer func() {
			c.Logger.Debug(ctx, "coordinate pinger exited")
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case start := <-ticker.C:
				ctx, cancel := context.WithTimeout(ctx, tick)

				err := conn.Ping(ctx)
				if err != nil {
					c.Logger.Error(ctx, "workspace agent coordinate ping", slog.Error(err))

					err := conn.Close(websocket.StatusGoingAway, "Ping failed")
					if err != nil {
						c.Logger.Error(ctx, "close workspace agent coordinate websocket", slog.Error(err))
					}

					cancel()
					return
				}

				c.Logger.Debug(ctx, "got coordinate pong", slog.F("took", time.Since(start)))
				cancel()
			}
		}
	}()

	return websocket.NetConn(ctx, conn, websocket.MessageBinary), nil
}

// @typescript-ignore DialWorkspaceAgentOptions
type DialWorkspaceAgentOptions struct {
	Logger slog.Logger
	// BlockEndpoints forced a direct connection through DERP.
	BlockEndpoints     bool
	EnableTrafficStats bool
}

func (c *Client) DialWorkspaceAgent(ctx context.Context, agentID uuid.UUID, options *DialWorkspaceAgentOptions) (*AgentConn, error) {
	if options == nil {
		options = &DialWorkspaceAgentOptions{}
	}
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/connection", agentID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var connInfo WorkspaceAgentConnectionInfo
	err = json.NewDecoder(res.Body).Decode(&connInfo)
	if err != nil {
		return nil, xerrors.Errorf("decode conn info: %w", err)
	}

	ip := tailnet.IP()
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:          []netip.Prefix{netip.PrefixFrom(ip, 128)},
		DERPMap:            connInfo.DERPMap,
		Logger:             options.Logger,
		BlockEndpoints:     options.BlockEndpoints,
		EnableTrafficStats: options.EnableTrafficStats,
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet: %w", err)
	}

	coordinateURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/coordinate", agentID))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(coordinateURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar:       jar,
		Transport: c.HTTPClient.Transport,
	}
	ctx, cancelFunc := context.WithCancel(ctx)
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
					first <- readBodyAsError(res)
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
				return conn.UpdateNodes(node)
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
		cancelFunc()
		_ = conn.Close()
		return nil, err
	}

	return &AgentConn{
		Conn: conn,
		CloseFunc: func() {
			cancelFunc()
			<-closed
		},
	}, nil
}

// WorkspaceAgent returns an agent by ID.
func (c *Client) WorkspaceAgent(ctx context.Context, id uuid.UUID) (WorkspaceAgent, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s", id), nil)
	if err != nil {
		return WorkspaceAgent{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgent{}, readBodyAsError(res)
	}
	var workspaceAgent WorkspaceAgent
	return workspaceAgent, json.NewDecoder(res.Body).Decode(&workspaceAgent)
}

// PostWorkspaceAgentAppHealth updates the workspace agent app health status.
func (c *Client) PostWorkspaceAgentAppHealth(ctx context.Context, req PostWorkspaceAppHealthsRequest) error {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/app-health", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}

	return nil
}

func (c *Client) PostWorkspaceAgentVersion(ctx context.Context, version string) error {
	versionReq := PostWorkspaceAgentVersionRequest{Version: version}
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/version", versionReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
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
		Name:  SessionTokenKey,
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
		return nil, readBodyAsError(res)
	}
	return websocket.NetConn(ctx, conn, websocket.MessageBinary), nil
}

// WorkspaceAgentListeningPorts returns a list of ports that are currently being
// listened on inside the workspace agent's network namespace.
func (c *Client) WorkspaceAgentListeningPorts(ctx context.Context, agentID uuid.UUID) (ListeningPortsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/listening-ports", agentID), nil)
	if err != nil {
		return ListeningPortsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ListeningPortsResponse{}, readBodyAsError(res)
	}
	var listeningPorts ListeningPortsResponse
	return listeningPorts, json.NewDecoder(res.Body).Decode(&listeningPorts)
}

// Stats records the Agent's network connection statistics for use in
// user-facing metrics and debugging.
// @typescript-ignore AgentStats
type AgentStats struct {
	// ConnsByProto is a count of connections by protocol.
	ConnsByProto map[string]int64 `json:"conns_by_proto"`
	// NumConns is the number of connections received by an agent.
	NumConns int64 `json:"num_comms"`
	// RxPackets is the number of received packets.
	RxPackets int64 `json:"rx_packets"`
	// RxBytes is the number of received bytes.
	RxBytes int64 `json:"rx_bytes"`
	// TxPackets is the number of transmitted bytes.
	TxPackets int64 `json:"tx_packets"`
	// TxBytes is the number of transmitted bytes.
	TxBytes int64 `json:"tx_bytes"`
}

// @typescript-ignore AgentStatsResponse
type AgentStatsResponse struct {
	// ReportInterval is the duration after which the agent should send stats
	// again.
	ReportInterval time.Duration `json:"report_interval"`
}

func (c *Client) PostAgentStats(ctx context.Context, stats *AgentStats) (AgentStatsResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/report-stats", stats)
	if err != nil {
		return AgentStatsResponse{}, xerrors.Errorf("send request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AgentStatsResponse{}, readBodyAsError(res)
	}

	var interval AgentStatsResponse
	err = json.NewDecoder(res.Body).Decode(&interval)
	if err != nil {
		return AgentStatsResponse{}, xerrors.Errorf("decode stats response: %w", err)
	}

	return interval, nil
}

// AgentReportStats begins a stat streaming connection with the Coder server.
// It is resilient to network failures and intermittent coderd issues.
func (c *Client) AgentReportStats(
	ctx context.Context,
	log slog.Logger,
	getStats func() *AgentStats,
) (io.Closer, error) {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		// Immediately trigger a stats push to get the correct interval.
		timer := time.NewTimer(time.Nanosecond)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}

			var nextInterval time.Duration
			for r := retry.New(100*time.Millisecond, time.Minute); r.Wait(ctx); {
				resp, err := c.PostAgentStats(ctx, getStats())
				if err != nil {
					if !xerrors.Is(err, context.Canceled) {
						log.Error(ctx, "report stats", slog.Error(err))
					}
					continue
				}

				nextInterval = resp.ReportInterval
				break
			}
			timer.Reset(nextInterval)
		}
	}()

	return closeFunc(func() error {
		cancel()
		return nil
	}), nil
}

// GitProvider is a constant that represents the
// type of providers that are supported within Coder.
// @typescript-ignore GitProvider
type GitProvider string

const (
	GitProviderAzureDevops = "azure-devops"
	GitProviderGitHub      = "github"
	GitProviderGitLab      = "gitlab"
	GitProviderBitBucket   = "bitbucket"
)

type WorkspaceAgentGitAuthResponse struct {
	Username string `json:"username"`
	Password string `json:"password"`
	URL      string `json:"url"`
}

// WorkspaceAgentGitAuth submits a URL to fetch a GIT_ASKPASS username
// and password for.
// nolint:revive
func (c *Client) WorkspaceAgentGitAuth(ctx context.Context, gitURL string, listen bool) (WorkspaceAgentGitAuthResponse, error) {
	reqURL := "/api/v2/workspaceagents/me/gitauth?url=" + url.QueryEscape(gitURL)
	if listen {
		reqURL += "&listen"
	}
	res, err := c.Request(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return WorkspaceAgentGitAuthResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentGitAuthResponse{}, readBodyAsError(res)
	}

	var authResp WorkspaceAgentGitAuthResponse
	return authResp, json.NewDecoder(res.Body).Decode(&authResp)
}

// @typescript-ignore PostWorkspaceAgentLifecycleRequest
type PostWorkspaceAgentLifecycleRequest struct {
	State WorkspaceAgentLifecycle `json:"state"`
}

func (c *Client) PostWorkspaceAgentLifecycle(ctx context.Context, req PostWorkspaceAgentLifecycleRequest) error {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/report-lifecycle", req)
	if err != nil {
		return xerrors.Errorf("agent state post request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return readBodyAsError(res)
	}

	return nil
}
