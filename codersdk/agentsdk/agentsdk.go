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
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"

	"github.com/coder/retry"

	"cdr.dev/slog"

	"github.com/google/uuid"

	"github.com/coder/coder/codersdk"
)

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

type Metadata struct {
	// GitAuthConfigs stores the number of Git configurations
	// the Coder deployment has. If this number is >0, we
	// set up special configuration in the workspace.
	GitAuthConfigs       int                     `json:"git_auth_configs"`
	VSCodePortProxyURI   string                  `json:"vscode_port_proxy_uri"`
	Apps                 []codersdk.WorkspaceApp `json:"apps"`
	DERPMap              *tailcfg.DERPMap        `json:"derpmap"`
	EnvironmentVariables map[string]string       `json:"environment_variables"`
	StartupScript        string                  `json:"startup_script"`
	StartupScriptTimeout time.Duration           `json:"startup_script_timeout"`
	Directory            string                  `json:"directory"`
	MOTDFile             string                  `json:"motd_file"`
}

// Metadata fetches metadata for the currently authenticated workspace agent.
func (c *Client) Metadata(ctx context.Context) (Metadata, error) {
	res, err := c.SDK.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/metadata", nil)
	if err != nil {
		return Metadata{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Metadata{}, codersdk.ReadBodyAsError(res)
	}
	var agentMeta Metadata
	err = json.NewDecoder(res.Body).Decode(&agentMeta)
	if err != nil {
		return Metadata{}, err
	}
	accessingPort := c.SDK.URL.Port()
	if accessingPort == "" {
		accessingPort = "80"
		if c.SDK.URL.Scheme == "https" {
			accessingPort = "443"
		}
	}
	accessPort, err := strconv.Atoi(accessingPort)
	if err != nil {
		return Metadata{}, xerrors.Errorf("convert accessing port %q: %w", accessingPort, err)
	}
	// Agents can provide an arbitrary access URL that may be different
	// that the globally configured one. This breaks the built-in DERP,
	// which would continue to reference the global access URL.
	//
	// This converts all built-in DERPs to use the access URL that the
	// metadata request was performed with.
	for _, region := range agentMeta.DERPMap.Regions {
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
	return agentMeta, nil
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

	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageBinary)

	// Ping once every 30 seconds to ensure that the websocket is alive. If we
	// don't get a response within 30s we kill the websocket and reconnect.
	// See: https://github.com/coder/coder/pull/5824
	go func() {
		tick := 30 * time.Second
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		defer func() {
			c.SDK.Logger.Debug(ctx, "coordinate pinger exited")
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case start := <-ticker.C:
				ctx, cancel := context.WithTimeout(ctx, tick)

				err := conn.Ping(ctx)
				if err != nil {
					c.SDK.Logger.Error(ctx, "workspace agent coordinate ping", slog.Error(err))

					err := conn.Close(websocket.StatusGoingAway, "Ping failed")
					if err != nil {
						c.SDK.Logger.Error(ctx, "close workspace agent coordinate websocket", slog.Error(err))
					}

					cancel()
					return
				}

				c.SDK.Logger.Debug(ctx, "got coordinate pong", slog.F("took", time.Since(start)))
				cancel()
			}
		}
	}()

	return wsNetConn, nil
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
	postStat(&Stats{ConnectionsByProto: map[string]int64{}})

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
	ConnectionsByProto map[string]int64 `json:"conns_by_proto"`
	// ConnectionCount is the number of connections received by an agent.
	ConnectionCount int64 `json:"num_comms"`
	// RxPackets is the number of received packets.
	RxPackets int64 `json:"rx_packets"`
	// RxBytes is the number of received bytes.
	RxBytes int64 `json:"rx_bytes"`
	// TxPackets is the number of transmitted bytes.
	TxPackets int64 `json:"tx_packets"`
	// TxBytes is the number of transmitted bytes.
	TxBytes int64 `json:"tx_bytes"`
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
	State codersdk.WorkspaceAgentLifecycle `json:"state"`
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
	Version           string `json:"version"`
	ExpandedDirectory string `json:"expanded_directory"`
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
