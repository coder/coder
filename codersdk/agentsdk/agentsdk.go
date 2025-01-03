package agentsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"storj.io/drpc"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/codersdk"
	drpcsdk "github.com/coder/coder/v2/codersdk/drpc"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/websocket"
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

type Metadata struct {
	Key string `json:"key"`
	codersdk.WorkspaceAgentMetadataResult
}

type PostMetadataRequest struct {
	Metadata []Metadata `json:"metadata"`
}

// In the future, we may want to support sending back multiple values for
// performance.
type PostMetadataRequestDeprecated = codersdk.WorkspaceAgentMetadataResult

type Manifest struct {
	AgentID   uuid.UUID `json:"agent_id"`
	AgentName string    `json:"agent_name"`
	// OwnerName and WorkspaceID are used by an open-source user to identify the workspace.
	// We do not provide insurance that this will not be removed in the future,
	// but if it's easy to persist lets keep it around.
	OwnerName     string    `json:"owner_name"`
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
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

// RewriteDERPMap rewrites the DERP map to use the access URL of the SDK as the
// "embedded relay" access URL. The passed derp map is modified in place.
//
// Agents can provide an arbitrary access URL that may be different that the
// globally configured one. This breaks the built-in DERP, which would continue
// to reference the global access URL.
func (c *Client) RewriteDERPMap(derpMap *tailcfg.DERPMap) {
	accessingPort := c.SDK.URL.Port()
	if accessingPort == "" {
		accessingPort = "80"
		if c.SDK.URL.Scheme == "https" {
			accessingPort = "443"
		}
	}
	accessPort, err := strconv.Atoi(accessingPort)
	if err != nil {
		// this should never happen because URL.Port() returns the empty string if the port is not
		// valid.
		c.SDK.Logger().Critical(context.Background(), "failed to parse URL port", slog.F("port", accessingPort))
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
}

// ConnectRPC20 returns a dRPC client to the Agent API v2.0.  Notably, it is missing
// GetAnnouncementBanners, but is useful when you want to be maximally compatible with Coderd
// Release Versions from 2.9+
// Deprecated: use ConnectRPC20WithTailnet
func (c *Client) ConnectRPC20(ctx context.Context) (proto.DRPCAgentClient20, error) {
	conn, err := c.connectRPCVersion(ctx, apiversion.New(2, 0))
	if err != nil {
		return nil, err
	}
	return proto.NewDRPCAgentClient(conn), nil
}

// ConnectRPC20WithTailnet returns a dRPC client to the Agent API v2.0.  Notably, it is missing
// GetAnnouncementBanners, but is useful when you want to be maximally compatible with Coderd
// Release Versions from 2.9+
func (c *Client) ConnectRPC20WithTailnet(ctx context.Context) (
	proto.DRPCAgentClient20, tailnetproto.DRPCTailnetClient20, error,
) {
	conn, err := c.connectRPCVersion(ctx, apiversion.New(2, 0))
	if err != nil {
		return nil, nil, err
	}
	return proto.NewDRPCAgentClient(conn), tailnetproto.NewDRPCTailnetClient(conn), nil
}

// ConnectRPC21 returns a dRPC client to the Agent API v2.1.  It is useful when you want to be
// maximally compatible with Coderd Release Versions from 2.12+
// Deprecated: use ConnectRPC21WithTailnet
func (c *Client) ConnectRPC21(ctx context.Context) (proto.DRPCAgentClient21, error) {
	conn, err := c.connectRPCVersion(ctx, apiversion.New(2, 1))
	if err != nil {
		return nil, err
	}
	return proto.NewDRPCAgentClient(conn), nil
}

// ConnectRPC21WithTailnet returns a dRPC client to the Agent API v2.1.  It is useful when you want to be
// maximally compatible with Coderd Release Versions from 2.12+
func (c *Client) ConnectRPC21WithTailnet(ctx context.Context) (
	proto.DRPCAgentClient21, tailnetproto.DRPCTailnetClient21, error,
) {
	conn, err := c.connectRPCVersion(ctx, apiversion.New(2, 1))
	if err != nil {
		return nil, nil, err
	}
	return proto.NewDRPCAgentClient(conn), tailnetproto.NewDRPCTailnetClient(conn), nil
}

// ConnectRPC22 returns a dRPC client to the Agent API v2.2.  It is useful when you want to be
// maximally compatible with Coderd Release Versions from 2.13+
func (c *Client) ConnectRPC22(ctx context.Context) (
	proto.DRPCAgentClient22, tailnetproto.DRPCTailnetClient22, error,
) {
	conn, err := c.connectRPCVersion(ctx, apiversion.New(2, 2))
	if err != nil {
		return nil, nil, err
	}
	return proto.NewDRPCAgentClient(conn), tailnetproto.NewDRPCTailnetClient(conn), nil
}

// ConnectRPC23 returns a dRPC client to the Agent API v2.3.  It is useful when you want to be
// maximally compatible with Coderd Release Versions from 2.18+
func (c *Client) ConnectRPC23(ctx context.Context) (
	proto.DRPCAgentClient23, tailnetproto.DRPCTailnetClient23, error,
) {
	conn, err := c.connectRPCVersion(ctx, apiversion.New(2, 3))
	if err != nil {
		return nil, nil, err
	}
	return proto.NewDRPCAgentClient(conn), tailnetproto.NewDRPCTailnetClient(conn), nil
}

// ConnectRPC connects to the workspace agent API and tailnet API
func (c *Client) ConnectRPC(ctx context.Context) (drpc.Conn, error) {
	return c.connectRPCVersion(ctx, proto.CurrentVersion)
}

func (c *Client) connectRPCVersion(ctx context.Context, version *apiversion.APIVersion) (drpc.Conn, error) {
	rpcURL, err := c.SDK.URL.Parse("/api/v2/workspaceagents/me/rpc")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	q := rpcURL.Query()
	q.Add("version", version.String())
	rpcURL.RawQuery = q.Encode()

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(rpcURL, []*http.Cookie{{
		Name:  codersdk.SessionTokenCookie,
		Value: c.SDK.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar:       jar,
		Transport: c.SDK.HTTPClient.Transport,
	}
	// nolint:bodyclose
	conn, res, err := websocket.Dial(ctx, rpcURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, codersdk.ReadBodyAsError(res)
	}

	// Set the read limit to 4 MiB -- about the limit for protobufs.  This needs to be larger than
	// the default because some of our protocols can include large messages like startup scripts.
	conn.SetReadLimit(1 << 22)
	netConn := websocket.NetConn(ctx, conn, websocket.MessageBinary)

	config := yamux.DefaultConfig()
	config.LogOutput = nil
	config.Logger = slog.Stdlib(ctx, c.SDK.Logger(), slog.LevelInfo)
	session, err := yamux.Client(netConn, config)
	if err != nil {
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	return drpcsdk.MultiplexedConn(session), nil
}

type PostAppHealthsRequest struct {
	// Healths is a map of the workspace app name and the health of the app.
	Healths map[uuid.UUID]codersdk.WorkspaceAppHealth
}

// BatchUpdateAppHealthsClient is a partial interface of proto.DRPCAgentClient.
type BatchUpdateAppHealthsClient interface {
	BatchUpdateAppHealths(ctx context.Context, req *proto.BatchUpdateAppHealthRequest) (*proto.BatchUpdateAppHealthResponse, error)
}

func AppHealthPoster(aAPI BatchUpdateAppHealthsClient) func(ctx context.Context, req PostAppHealthsRequest) error {
	return func(ctx context.Context, req PostAppHealthsRequest) error {
		pReq, err := ProtoFromAppHealthsRequest(req)
		if err != nil {
			return xerrors.Errorf("convert AppHealthsRequest: %w", err)
		}
		_, err = aAPI.BatchUpdateAppHealths(ctx, pReq)
		if err != nil {
			return xerrors.Errorf("batch update app healths: %w", err)
		}
		return nil
	}
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

func (s Stats) SessionCount() int64 {
	return s.SessionCountVSCode + s.SessionCountJetBrains + s.SessionCountReconnectingPTY + s.SessionCountSSH
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

type PostLifecycleRequest struct {
	State     codersdk.WorkspaceAgentLifecycle `json:"state"`
	ChangedAt time.Time                        `json:"changed_at"`
}

type PostStartupRequest struct {
	Version           string                    `json:"version"`
	ExpandedDirectory string                    `json:"expanded_directory"`
	Subsystems        []codersdk.AgentSubsystem `json:"subsystems"`
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
//
// Deprecated: use the DRPCAgentClient.BatchCreateLogs instead
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

type PostLogSourceRequest struct {
	// ID is a unique identifier for the log source.
	// It is scoped to a workspace agent, and can be statically
	// defined inside code to prevent duplicate sources from being
	// created for the same agent.
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Icon        string    `json:"icon"`
}

func (c *Client) PostLogSource(ctx context.Context, req PostLogSourceRequest) (codersdk.WorkspaceAgentLogSource, error) {
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

type ExternalAuthResponse struct {
	AccessToken string                 `json:"access_token"`
	TokenExtra  map[string]interface{} `json:"token_extra"`
	URL         string                 `json:"url"`
	Type        string                 `json:"type"`

	// Deprecated: Only supported on `/workspaceagents/me/gitauth`
	// for backwards compatibility.
	Username string `json:"username"`
	Password string `json:"password"`
}

// ExternalAuthRequest is used to request an access token for a provider.
// Either ID or Match must be specified, but not both.
type ExternalAuthRequest struct {
	// ID is the ID of a provider to request authentication for.
	ID string
	// Match is an arbitrary string matched against the regex of the provider.
	Match string
	// Listen indicates that the request should be long-lived and listen for
	// a new token to be requested.
	Listen bool
}

// ExternalAuth submits a URL or provider ID to fetch an access token for.
// nolint:revive
func (c *Client) ExternalAuth(ctx context.Context, req ExternalAuthRequest) (ExternalAuthResponse, error) {
	q := url.Values{
		"id":    []string{req.ID},
		"match": []string{req.Match},
	}
	if req.Listen {
		q.Set("listen", "true")
	}
	reqURL := "/api/v2/workspaceagents/me/external-auth?" + q.Encode()
	res, err := c.SDK.Request(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return ExternalAuthResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ExternalAuthResponse{}, codersdk.ReadBodyAsError(res)
	}

	var authResp ExternalAuthResponse
	return authResp, json.NewDecoder(res.Body).Decode(&authResp)
}

// LogsNotifyChannel returns the channel name responsible for notifying
// of new logs.
func LogsNotifyChannel(agentID uuid.UUID) string {
	return fmt.Sprintf("agent-logs:%s", agentID)
}

type LogsNotifyMessage struct {
	CreatedAfter int64 `json:"created_after"`
}
