package agentsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

// ExternalLogSourceID is the statically-defined ID of a log-source that
// appears as "External" in the dashboard.
//
// This is to support legacy API-consumers that do not create their own
// log-source. This should be removed in the future.
var ExternalLogSourceID = uuid.MustParse("3b579bf4-1ed8-4b99-87a8-e9a1e3410410")

// New returns a client that is used to interact with the
// Coder API from a workspace agent.
func New(serverURL *url.URL) *Client {
	return &Client{
		SDK: codersdk.New(serverURL),
	}
}

// Client wraps `codersdk.Client` with specific functions
// scoped to a workspace agent.
type Client struct {
	SDK *codersdk.Client
}

func (c *Client) SetSessionToken(token string) {
	c.SDK.SetSessionToken(token)
}

type GitSSHKey struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// GitSSHKey will return the user's SSH key pair for the workspace.
func (c *Client) GitSSHKey(ctx context.Context) (GitSSHKey, error) {
	res, err := c.SDK.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/gitsshkey", nil)
	if err != nil {
		return GitSSHKey{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GitSSHKey{}, codersdk.ReadBodyAsError(res)
	}

	var gitSSHKey GitSSHKey
	return gitSSHKey, json.NewDecoder(res.Body).Decode(&gitSSHKey)
}

// In the future, we may want to support sending back multiple values for
// performance.
type PostMetadataRequest = codersdk.WorkspaceAgentMetadataResult

func (c *Client) PostMetadata(ctx context.Context, key string, req PostMetadataRequest) error {
	res, err := c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/metadata/"+key, req)
	if err != nil {
		return xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return codersdk.ReadBodyAsError(res)
	}

	return nil
}

type Manifest struct {
	AgentID uuid.UUID `json:"agent_id"`
	// GitAuthConfigs stores the number of Git configurations
	// the Coder deployment has. If this number is >0, we
	// set up special configuration in the workspace.
	GitAuthConfigs           int                                          `json:"git_auth_configs"`
	VSCodePortProxyURI       string                                       `json:"vscode_port_proxy_uri"`
	Apps                     []codersdk.WorkspaceApp                      `json:"apps"`
	DERPMap                  *tailcfg.DERPMap                             `json:"derpmap"`
	DERPForceWebSockets      bool                                         `json:"derp_force_websockets"`
	EnvironmentVariables     map[string]string                            `json:"environment_variables"`
	Directory                string                                       `json:"directory"`
	MOTDFile                 string                                       `json:"motd_file"`
	DisableDirectConnections bool                                         `json:"disable_direct_connections"`
	Metadata                 []codersdk.WorkspaceAgentMetadataDescription `json:"metadata"`
	Scripts                  []codersdk.WorkspaceAgentScript              `json:"scripts"`
}

type LogSource struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Icon        string    `json:"icon"`
}

type Script struct {
	Script string `json:"script"`
}

// Manifest fetches manifest for the currently authenticated workspace agent.
func (c *Client) Manifest(ctx context.Context) (Manifest, error) {
	res, err := c.SDK.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/manifest", nil)
	if err != nil {
		return Manifest{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Manifest{}, codersdk.ReadBodyAsError(res)
	}
	var agentMeta Manifest
	err = json.NewDecoder(res.Body).Decode(&agentMeta)
	if err != nil {
		return Manifest{}, err
	}
	err = c.rewriteDerpMap(agentMeta.DERPMap)
	if err != nil {
		return Manifest{}, err
	}
	return agentMeta, nil
}

// rewriteDerpMap rewrites the DERP map to use the access URL of the SDK as the
// "embedded relay" access URL. The passed derp map is modified in place.
//
// Agents can provide an arbitrary access URL that may be different that the
// globally configured one. This breaks the built-in DERP, which would continue
// to reference the global access URL.
func (c *Client) rewriteDerpMap(derpMap *tailcfg.DERPMap) error {
	accessingPort := c.SDK.URL.Port()
	if accessingPort == "" {
		accessingPort = "80"
		if c.SDK.URL.Scheme == "https" {
			accessingPort = "443"
		}
	}
	accessPort, err := strconv.Atoi(accessingPort)
	if err != nil {
		return xerrors.Errorf("convert accessing port %q: %w", accessingPort, err)
	}
	for _, region := range derpMap.Regions {
		if !region.EmbeddedRelay {
			continue
		}

		for _, node := range region.Nodes {
			if node.STUNOnly {
				continue
			}
			node.HostName = c.SDK.URL.Hostname()
			node.DERPPort = accessPort
			node.ForceHTTP = c.SDK.URL.Scheme == "http"
		}
	}
	return nil
}

type DERPMapUpdate struct {
	Err     error
	DERPMap *tailcfg.DERPMap
}

// DERPMapUpdates connects to the DERP map updates WebSocket.
func (c *Client) DERPMapUpdates(ctx context.Context) (<-chan DERPMapUpdate, io.Closer, error) {
	derpMapURL, err := c.SDK.URL.Parse("/api/v2/derp-map")
	if err != nil {
		return nil, nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(derpMapURL, []*http.Cookie{{
		Name:  codersdk.SessionTokenCookie,
		Value: c.SDK.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar:       jar,
		Transport: c.SDK.HTTPClient.Transport,
	}
	// nolint:bodyclose
	conn, res, err := websocket.Dial(ctx, derpMapURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		if res == nil {
			return nil, nil, err
		}
		return nil, nil, codersdk.ReadBodyAsError(res)
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageBinary)
	pingClosed := pingWebSocket(ctx, c.SDK.Logger(), conn, "derp map")

	var (
		updates       = make(chan DERPMapUpdate)
		updatesClosed = make(chan struct{})
		dec           = json.NewDecoder(wsNetConn)
	)
	go func() {
		defer close(updates)
		defer close(updatesClosed)
		defer cancelFunc()
		defer conn.Close(websocket.StatusGoingAway, "DERPMapUpdates closed")
		for {
			var update DERPMapUpdate
			err := dec.Decode(&update.DERPMap)
			if err != nil {
				update.Err = err
				update.DERPMap = nil
			}
			if update.DERPMap != nil {
				err = c.rewriteDerpMap(update.DERPMap)
				if err != nil {
					update.Err = err
					update.DERPMap = nil
				}
			}

			select {
			case updates <- update:
			case <-ctx.Done():
				// Unblock the caller if they're waiting for an update.
				select {
				case updates <- DERPMapUpdate{Err: ctx.Err()}:
				default:
				}
				return
			}
			if update.Err != nil {
				return
			}
		}
	}()

	return updates, &closer{
		closeFunc: func() error {
			cancelFunc()
			<-pingClosed
			_ = conn.Close(websocket.StatusGoingAway, "DERPMapUpdates closed")
			<-updatesClosed
			return nil
		},
	}, nil
}

// Listen connects to the workspace agent coordinate WebSocket
// that handles connection negotiation.
func (c *Client) Listen(ctx context.Context) (net.Conn, error) {
	coordinateURL, err := c.SDK.URL.Parse("/api/v2/workspaceagents/me/coordinate")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(coordinateURL, []*http.Cookie{{
		Name:  codersdk.SessionTokenCookie,
		Value: c.SDK.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar:       jar,
		Transport: c.SDK.HTTPClient.Transport,
	}
	// nolint:bodyclose
	conn, res, err := websocket.Dial(ctx, coordinateURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, codersdk.ReadBodyAsError(res)
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageBinary)
	pingClosed := pingWebSocket(ctx, c.SDK.Logger(), conn, "coordinate")

	return &closeNetConn{
		Conn: wsNetConn,
		closeFunc: func() {
			cancelFunc()
			_ = conn.Close(websocket.StatusGoingAway, "Listen closed")
			<-pingClosed
		},
	}, nil
}

type PostAppHealthsRequest struct {
	// Healths is a map of the workspace app name and the health of the app.
	Healths map[uuid.UUID]codersdk.WorkspaceAppHealth
}

// PostAppHealth updates the workspace agent app health status.
func (c *Client) PostAppHealth(ctx context.Context, req PostAppHealthsRequest) error {
	res, err := c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/app-health", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.ReadBodyAsError(res)
	}

	return nil
}

// AuthenticateResponse is returned when an instance ID
// has been exchanged for a session token.
// @typescript-ignore AuthenticateResponse
type AuthenticateResponse struct {
	SessionToken string `json:"session_token"`
}

type GoogleInstanceIdentityToken struct {
	JSONWebToken string `json:"json_web_token" validate:"required"`
}

// AuthWorkspaceGoogleInstanceIdentity uses the Google Compute Engine Metadata API to
// fetch a signed JWT, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthGoogleInstanceIdentity(ctx context.Context, serviceAccount string, gcpClient *metadata.Client) (AuthenticateResponse, error) {
	if serviceAccount == "" {
		// This is the default name specified by Google.
		serviceAccount = "default"
	}
	if gcpClient == nil {
		gcpClient = metadata.NewClient(c.SDK.HTTPClient)
	}
	// "format=full" is required, otherwise the responding payload will be missing "instance_id".
	jwt, err := gcpClient.Get(fmt.Sprintf("instance/service-accounts/%s/identity?audience=coder&format=full", serviceAccount))
	if err != nil {
		return AuthenticateResponse{}, xerrors.Errorf("get metadata identity: %w", err)
	}
	res, err := c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/google-instance-identity", GoogleInstanceIdentityToken{
		JSONWebToken: jwt,
	})
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AuthenticateResponse{}, codersdk.ReadBodyAsError(res)
	}
	var resp AuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

type AWSInstanceIdentityToken struct {
	Signature string `json:"signature" validate:"required"`
	Document  string `json:"document" validate:"required"`
}

// AuthWorkspaceAWSInstanceIdentity uses the Amazon Metadata API to
// fetch a signed payload, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthAWSInstanceIdentity(ctx context.Context) (AuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return AuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
	res, err := c.SDK.HTTPClient.Do(req)
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()
	token, err := io.ReadAll(res.Body)
	if err != nil {
		return AuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/signature", nil)
	if err != nil {
		return AuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = c.SDK.HTTPClient.Do(req)
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()
	signature, err := io.ReadAll(res.Body)
	if err != nil {
		return AuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/document", nil)
	if err != nil {
		return AuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = c.SDK.HTTPClient.Do(req)
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()
	document, err := io.ReadAll(res.Body)
	if err != nil {
		return AuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	res, err = c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/aws-instance-identity", AWSInstanceIdentityToken{
		Signature: string(signature),
		Document:  string(document),
	})
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AuthenticateResponse{}, codersdk.ReadBodyAsError(res)
	}
	var resp AuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

type AzureInstanceIdentityToken struct {
	Signature string `json:"signature" validate:"required"`
	Encoding  string `json:"encoding" validate:"required"`
}

// AuthWorkspaceAzureInstanceIdentity uses the Azure Instance Metadata Service to
// fetch a signed payload, and exchange it for a session token for a workspace agent.
func (c *Client) AuthAzureInstanceIdentity(ctx context.Context) (AuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/metadata/attested/document?api-version=2020-09-01", nil)
	if err != nil {
		return AuthenticateResponse{}, nil
	}
	req.Header.Set("Metadata", "true")
	res, err := c.SDK.HTTPClient.Do(req)
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()

	var token AzureInstanceIdentityToken
	err = json.NewDecoder(res.Body).Decode(&token)
	if err != nil {
		return AuthenticateResponse{}, err
	}

	res, err = c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/azure-instance-identity", token)
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AuthenticateResponse{}, codersdk.ReadBodyAsError(res)
	}
	var resp AuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ReportStats begins a stat streaming connection with the Coder server.
// It is resilient to network failures and intermittent coderd issues.
func (c *Client) ReportStats(ctx context.Context, log slog.Logger, statsChan <-chan *Stats, setInterval func(time.Duration)) (io.Closer, error) {
	var interval time.Duration
	ctx, cancel := context.WithCancel(ctx)
	exited := make(chan struct{})

	postStat := func(stat *Stats) {
		var nextInterval time.Duration
		for r := retry.New(100*time.Millisecond, time.Minute); r.Wait(ctx); {
			resp, err := c.PostStats(ctx, stat)
			if err != nil {
				if !xerrors.Is(err, context.Canceled) {
					log.Error(ctx, "report stats", slog.Error(err))
				}
				continue
			}

			nextInterval = resp.ReportInterval
			break
		}

		if nextInterval != 0 && interval != nextInterval {
			setInterval(nextInterval)
		}
		interval = nextInterval
	}

	// Send an empty stat to get the interval.
	postStat(&Stats{})

	go func() {
		defer close(exited)

		for {
			select {
			case <-ctx.Done():
				return
			case stat, ok := <-statsChan:
				if !ok {
					return
				}

				postStat(stat)
			}
		}
	}()

	return closeFunc(func() error {
		cancel()
		<-exited
		return nil
	}), nil
}

// Stats records the Agent's network connection statistics for use in
// user-facing metrics and debugging.
type Stats struct {
	// ConnectionsByProto is a count of connections by protocol.
	ConnectionsByProto map[string]int64 `json:"connections_by_proto"`
	// ConnectionCount is the number of connections received by an agent.
	ConnectionCount int64 `json:"connection_count"`
	// ConnectionMedianLatencyMS is the median latency of all connections in milliseconds.
	ConnectionMedianLatencyMS float64 `json:"connection_median_latency_ms"`
	// RxPackets is the number of received packets.
	RxPackets int64 `json:"rx_packets"`
	// RxBytes is the number of received bytes.
	RxBytes int64 `json:"rx_bytes"`
	// TxPackets is the number of transmitted bytes.
	TxPackets int64 `json:"tx_packets"`
	// TxBytes is the number of transmitted bytes.
	TxBytes int64 `json:"tx_bytes"`

	// SessionCountVSCode is the number of connections received by an agent
	// that are from our VS Code extension.
	SessionCountVSCode int64 `json:"session_count_vscode"`
	// SessionCountJetBrains is the number of connections received by an agent
	// that are from our JetBrains extension.
	SessionCountJetBrains int64 `json:"session_count_jetbrains"`
	// SessionCountReconnectingPTY is the number of connections received by an agent
	// that are from the reconnecting web terminal.
	SessionCountReconnectingPTY int64 `json:"session_count_reconnecting_pty"`
	// SessionCountSSH is the number of connections received by an agent
	// that are normal, non-tagged SSH sessions.
	SessionCountSSH int64 `json:"session_count_ssh"`

	// Metrics collected by the agent
	Metrics []AgentMetric `json:"metrics"`
}

type AgentMetricType string

const (
	AgentMetricTypeCounter AgentMetricType = "counter"
	AgentMetricTypeGauge   AgentMetricType = "gauge"
)

type AgentMetric struct {
	Name   string             `json:"name" validate:"required"`
	Type   AgentMetricType    `json:"type" validate:"required" enums:"counter,gauge"`
	Value  float64            `json:"value" validate:"required"`
	Labels []AgentMetricLabel `json:"labels,omitempty"`
}

type AgentMetricLabel struct {
	Name  string `json:"name" validate:"required"`
	Value string `json:"value" validate:"required"`
}

type StatsResponse struct {
	// ReportInterval is the duration after which the agent should send stats
	// again.
	ReportInterval time.Duration `json:"report_interval"`
}

func (c *Client) PostStats(ctx context.Context, stats *Stats) (StatsResponse, error) {
	res, err := c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/report-stats", stats)
	if err != nil {
		return StatsResponse{}, xerrors.Errorf("send request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return StatsResponse{}, codersdk.ReadBodyAsError(res)
	}

	var interval StatsResponse
	err = json.NewDecoder(res.Body).Decode(&interval)
	if err != nil {
		return StatsResponse{}, xerrors.Errorf("decode stats response: %w", err)
	}

	return interval, nil
}

type PostLifecycleRequest struct {
	State     codersdk.WorkspaceAgentLifecycle `json:"state"`
	ChangedAt time.Time                        `json:"changed_at"`
}

func (c *Client) PostLifecycle(ctx context.Context, req PostLifecycleRequest) error {
	res, err := c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/report-lifecycle", req)
	if err != nil {
		return xerrors.Errorf("agent state post request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return codersdk.ReadBodyAsError(res)
	}

	return nil
}

type PostStartupRequest struct {
	Version           string                    `json:"version"`
	ExpandedDirectory string                    `json:"expanded_directory"`
	Subsystems        []codersdk.AgentSubsystem `json:"subsystems"`
}

func (c *Client) PostStartup(ctx context.Context, req PostStartupRequest) error {
	res, err := c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/startup", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}

type Log struct {
	CreatedAt time.Time         `json:"created_at"`
	Output    string            `json:"output"`
	Level     codersdk.LogLevel `json:"level"`
}

type PatchLogs struct {
	LogSourceID uuid.UUID `json:"log_source_id"`
	Logs        []Log     `json:"logs"`
}

// PatchLogs writes log messages to the agent startup script.
// Log messages are limited to 1MB in total.
func (c *Client) PatchLogs(ctx context.Context, req PatchLogs) error {
	res, err := c.SDK.Request(ctx, http.MethodPatch, "/api/v2/workspaceagents/me/logs", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}

type PostLogSource struct {
	// ID is a unique identifier for the log source.
	// It is scoped to a workspace agent, and can be statically
	// defined inside code to prevent duplicate sources from being
	// created for the same agent.
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Icon        string    `json:"icon"`
}

func (c *Client) PostLogSource(ctx context.Context, req PostLogSource) (codersdk.WorkspaceAgentLogSource, error) {
	res, err := c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/log-source", req)
	if err != nil {
		return codersdk.WorkspaceAgentLogSource{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return codersdk.WorkspaceAgentLogSource{}, codersdk.ReadBodyAsError(res)
	}
	var logSource codersdk.WorkspaceAgentLogSource
	return logSource, json.NewDecoder(res.Body).Decode(&logSource)
}

// GetServiceBanner relays the service banner config.
func (c *Client) GetServiceBanner(ctx context.Context) (codersdk.ServiceBannerConfig, error) {
	res, err := c.SDK.Request(ctx, http.MethodGet, "/api/v2/appearance", nil)
	if err != nil {
		return codersdk.ServiceBannerConfig{}, err
	}
	defer res.Body.Close()
	// If the route does not exist then Enterprise code is not enabled.
	if res.StatusCode == http.StatusNotFound {
		return codersdk.ServiceBannerConfig{}, nil
	}
	if res.StatusCode != http.StatusOK {
		return codersdk.ServiceBannerConfig{}, codersdk.ReadBodyAsError(res)
	}
	var cfg codersdk.AppearanceConfig
	return cfg.ServiceBanner, json.NewDecoder(res.Body).Decode(&cfg)
}

type GitAuthResponse struct {
	Username string `json:"username"`
	Password string `json:"password"`
	URL      string `json:"url"`
}

// GitAuth submits a URL to fetch a GIT_ASKPASS username and password for.
// nolint:revive
func (c *Client) GitAuth(ctx context.Context, gitURL string, listen bool) (GitAuthResponse, error) {
	reqURL := "/api/v2/workspaceagents/me/gitauth?url=" + url.QueryEscape(gitURL)
	if listen {
		reqURL += "&listen"
	}
	res, err := c.SDK.Request(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return GitAuthResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GitAuthResponse{}, codersdk.ReadBodyAsError(res)
	}

	var authResp GitAuthResponse
	return authResp, json.NewDecoder(res.Body).Decode(&authResp)
}

type closeFunc func() error

func (c closeFunc) Close() error {
	return c()
}

// wsNetConn wraps net.Conn created by websocket.NetConn(). Cancel func
// is called if a read or write error is encountered.
type wsNetConn struct {
	cancel context.CancelFunc
	net.Conn
}

func (c *wsNetConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Close() error {
	defer c.cancel()
	return c.Conn.Close()
}

// websocketNetConn wraps websocket.NetConn and returns a context that
// is tied to the parent context and the lifetime of the conn. Any error
// during read or write will cancel the context, but not close the
// conn. Close should be called to release context resources.
func websocketNetConn(ctx context.Context, conn *websocket.Conn, msgType websocket.MessageType) (context.Context, net.Conn) {
	ctx, cancel := context.WithCancel(ctx)
	nc := websocket.NetConn(ctx, conn, msgType)
	return ctx, &wsNetConn{
		cancel: cancel,
		Conn:   nc,
	}
}

// LogsNotifyChannel returns the channel name responsible for notifying
// of new logs.
func LogsNotifyChannel(agentID uuid.UUID) string {
	return fmt.Sprintf("agent-logs:%s", agentID)
}

type LogsNotifyMessage struct {
	CreatedAfter int64 `json:"created_after"`
}

type closeNetConn struct {
	net.Conn
	closeFunc func()
}

func (c *closeNetConn) Close() error {
	c.closeFunc()
	return c.Conn.Close()
}

func pingWebSocket(ctx context.Context, logger slog.Logger, conn *websocket.Conn, name string) <-chan struct{} {
	// Ping once every 30 seconds to ensure that the websocket is alive. If we
	// don't get a response within 30s we kill the websocket and reconnect.
	// See: https://github.com/coder/coder/pull/5824
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		tick := 30 * time.Second
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		defer func() {
			logger.Debug(ctx, fmt.Sprintf("%s pinger exited", name))
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case start := <-ticker.C:
				ctx, cancel := context.WithTimeout(ctx, tick)

				err := conn.Ping(ctx)
				if err != nil {
					logger.Error(ctx, fmt.Sprintf("workspace agent %s ping", name), slog.Error(err))

					err := conn.Close(websocket.StatusGoingAway, "Ping failed")
					if err != nil {
						logger.Error(ctx, fmt.Sprintf("close workspace agent %s websocket", name), slog.Error(err))
					}

					cancel()
					return
				}

				logger.Debug(ctx, fmt.Sprintf("got %s ping", name), slog.F("took", time.Since(start)))
				cancel()
			}
		}
	}()

	return closed
}

type closer struct {
	closeFunc func() error
}

func (c *closer) Close() error {
	return c.closeFunc()
}
