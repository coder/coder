package agentsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"storj.io/drpc"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/v3"

	"github.com/coder/retry"
	"github.com/coder/websocket"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

// ExternalLogSourceID is the statically-defined ID of a log-source that
// appears as "External" in the dashboard.
//
// This is to support legacy API-consumers that do not create their own
// log-source. This should be removed in the future.
var ExternalLogSourceID = uuid.MustParse("3b579bf4-1ed8-4b99-87a8-e9a1e3410410")

// SessionTokenSetup is a function that creates the token provider while setting up the workspace agent. We do it this
// way because cloud instance identity (AWS, Azure, Google, etc.) requires interacting with coderd to exchange tokens.
// This means that the token providers need a codersdk.Client. However, the SessionTokenProvider is itself used by
// the client to authenticate requests. Thus, the dependency is bidirectional. Functions of this type are used in
// New() to ensure that things are set up correctly so there is only one instance of the codersdk.Client created.
// @typescript-ignore SessionTokenSetup
type SessionTokenSetup func(client *codersdk.Client) RefreshableSessionTokenProvider

// New creates a new *Client which can be used by an agent to connect to Coderd. Use a SessionTokenSetup function
// to define the session token provider for the Client. This overrides the SessionTokenProvider on the underlying
// `*codersdk.Client`, so any `codersdk.ClientOptions` passed as `opts` should not set this property.
func New(serverURL *url.URL, setup SessionTokenSetup, opts ...codersdk.ClientOption) *Client {
	var provider RefreshableSessionTokenProvider
	opts = append(opts, func(c *codersdk.Client) {
		provider = setup(c)
		c.SessionTokenProvider = provider
	})
	c := codersdk.New(serverURL, opts...)
	return &Client{
		SDK:                             c,
		RefreshableSessionTokenProvider: provider,
	}
}

// Client wraps `codersdk.Client` with specific functions
// scoped to a workspace agent.
type Client struct {
	RefreshableSessionTokenProvider
	SDK *codersdk.Client
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
	ParentID  uuid.UUID `json:"parent_id"`
	AgentID   uuid.UUID `json:"agent_id"`
	AgentName string    `json:"agent_name"`
	// OwnerUsername and WorkspaceID are used by an open-source user to identify the workspace.
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
	Devcontainers            []codersdk.WorkspaceAgentDevcontainer        `json:"devcontainers"`
}

type LogSource struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Icon        string    `json:"icon"`
}

type Script struct {
	Script string `json:"script"`
}

// RewriteDERPMap rewrites the DERP map to use the configured access URL of the
// agent as the "embedded relay" access URL.
//
// See tailnet.RewriteDERPMapDefaultRelay for more details on why this is
// necessary.
func (c *Client) RewriteDERPMap(derpMap *tailcfg.DERPMap) {
	tailnet.RewriteDERPMapDefaultRelay(context.Background(), c.SDK.Logger(), derpMap, c.SDK.URL)
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

// ConnectRPC24 returns a dRPC client to the Agent API v2.4.  It is useful when you want to be
// maximally compatible with Coderd Release Versions from 2.20+
func (c *Client) ConnectRPC24(ctx context.Context) (
	proto.DRPCAgentClient24, tailnetproto.DRPCTailnetClient24, error,
) {
	conn, err := c.connectRPCVersion(ctx, apiversion.New(2, 4))
	if err != nil {
		return nil, nil, err
	}
	return proto.NewDRPCAgentClient(conn), tailnetproto.NewDRPCTailnetClient(conn), nil
}

// ConnectRPC25 returns a dRPC client to the Agent API v2.5.  It is useful when you want to be
// maximally compatible with Coderd Release Versions from 2.23+
func (c *Client) ConnectRPC25(ctx context.Context) (
	proto.DRPCAgentClient25, tailnetproto.DRPCTailnetClient25, error,
) {
	conn, err := c.connectRPCVersion(ctx, apiversion.New(2, 5))
	if err != nil {
		return nil, nil, err
	}
	return proto.NewDRPCAgentClient(conn), tailnetproto.NewDRPCTailnetClient(conn), nil
}

// ConnectRPC26 returns a dRPC client to the Agent API v2.6.  It is useful when you want to be
// maximally compatible with Coderd Release Versions from 2.24+
func (c *Client) ConnectRPC26(ctx context.Context) (
	proto.DRPCAgentClient26, tailnetproto.DRPCTailnetClient26, error,
) {
	conn, err := c.connectRPCVersion(ctx, apiversion.New(2, 6))
	if err != nil {
		return nil, nil, err
	}
	return proto.NewDRPCAgentClient(conn), tailnetproto.NewDRPCTailnetClient(conn), nil
}

// ConnectRPC27 returns a dRPC client to the Agent API v2.7.  It is useful when you want to be
// maximally compatible with Coderd Release Versions from 2.30+
func (c *Client) ConnectRPC27(ctx context.Context) (
	proto.DRPCAgentClient27, tailnetproto.DRPCTailnetClient27, error,
) {
	conn, err := c.connectRPCVersion(ctx, apiversion.New(2, 7))
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

// RefreshableSessionTokenProvider is a SessionTokenProvider that can be refreshed, for example, via token exchange.
// @typescript-ignore RefreshableSessionTokenProvider
type RefreshableSessionTokenProvider interface {
	codersdk.SessionTokenProvider
	RefreshToken(ctx context.Context) error
}

// InstanceIdentitySessionTokenProvider implements RefreshableSessionTokenProvider via token exchange for a cloud
// compute instance identity.
// @typescript-ignore InstanceIdentitySessionTokenProvider
type InstanceIdentitySessionTokenProvider struct {
	TokenExchanger TokenExchanger
	logger         slog.Logger

	// cache so we don't request each time
	mu           sync.Mutex
	sessionToken string
}

// TokenExchanger obtains a session token by exchanging a cloud instance identity credential for a Coder session token.
// @typescript-ignore TokenExchanger
type TokenExchanger interface {
	exchange(ctx context.Context) (AuthenticateResponse, error)
}

func (i *InstanceIdentitySessionTokenProvider) AsRequestOption() codersdk.RequestOption {
	t := i.GetSessionToken()
	return func(req *http.Request) {
		req.Header.Set(codersdk.SessionTokenHeader, t)
	}
}

func (i *InstanceIdentitySessionTokenProvider) SetDialOption(opts *websocket.DialOptions) {
	t := i.GetSessionToken()
	if opts.HTTPHeader == nil {
		opts.HTTPHeader = http.Header{}
	}
	if opts.HTTPHeader.Get(codersdk.SessionTokenHeader) == "" {
		opts.HTTPHeader.Set(codersdk.SessionTokenHeader, t)
	}
}

func (i *InstanceIdentitySessionTokenProvider) GetSessionToken() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.sessionToken != "" {
		return i.sessionToken
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := i.TokenExchanger.exchange(ctx)
	if err != nil {
		i.logger.Error(ctx, "failed to exchange session token", slog.Error(err))
		return ""
	}
	i.sessionToken = resp.SessionToken
	return i.sessionToken
}

func (i *InstanceIdentitySessionTokenProvider) RefreshToken(ctx context.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	resp, err := i.TokenExchanger.exchange(ctx)
	if err != nil {
		return err
	}
	i.sessionToken = resp.SessionToken
	return nil
}

// FixedSessionTokenProvider wraps the codersdk variant to add a no-op RefreshToken method to satisfy the
// RefreshableSessionTokenProvider interface.
// @typescript-ignore FixedSessionTokenProvider
type FixedSessionTokenProvider struct {
	codersdk.FixedSessionTokenProvider
}

func (FixedSessionTokenProvider) RefreshToken(_ context.Context) error {
	return nil
}

func WithFixedToken(token string) SessionTokenSetup {
	return func(_ *codersdk.Client) RefreshableSessionTokenProvider {
		return FixedSessionTokenProvider{FixedSessionTokenProvider: codersdk.FixedSessionTokenProvider{SessionToken: token}}
	}
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

// PatchAppStatus updates the status of a workspace app.
type PatchAppStatus struct {
	AppSlug string                           `json:"app_slug"`
	State   codersdk.WorkspaceAppStatusState `json:"state"`
	Message string                           `json:"message"`
	URI     string                           `json:"uri"`
	// Deprecated: this field is unused and will be removed in a future version.
	Icon string `json:"icon"`
	// Deprecated: this field is unused and will be removed in a future version.
	NeedsUserAttention bool `json:"needs_user_attention"`
}

func (c *Client) PatchAppStatus(ctx context.Context, req PatchAppStatus) error {
	res, err := c.SDK.Request(ctx, http.MethodPatch, "/api/v2/workspaceagents/me/app-status", req)
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

type ReinitializationReason string

const (
	ReinitializeReasonPrebuildClaimed ReinitializationReason = "prebuild_claimed"
)

type ReinitializationEvent struct {
	WorkspaceID uuid.UUID
	Reason      ReinitializationReason `json:"reason"`
}

func PrebuildClaimedChannel(id uuid.UUID) string {
	return fmt.Sprintf("prebuild_claimed_%s", id)
}

// WaitForReinit polls a SSE endpoint, and receives an event back under the following conditions:
// - ping: ignored, keepalive
// - prebuild claimed: a prebuilt workspace is claimed, so the agent must reinitialize.
func (c *Client) WaitForReinit(ctx context.Context) (*ReinitializationEvent, error) {
	rpcURL, err := c.SDK.URL.Parse("/api/v2/workspaceagents/me/reinit")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}

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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rpcURL.String(), nil)
	if err != nil {
		return nil, xerrors.Errorf("build request: %w", err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, codersdk.ReadBodyAsError(res)
	}

	reinitEvent, err := NewSSEAgentReinitReceiver(res.Body).Receive(ctx)
	if err != nil {
		return nil, xerrors.Errorf("listening for reinitialization events: %w", err)
	}
	return reinitEvent, nil
}

func WaitForReinitLoop(ctx context.Context, logger slog.Logger, client *Client) <-chan ReinitializationEvent {
	reinitEvents := make(chan ReinitializationEvent)

	go func() {
		for retrier := retry.New(100*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
			logger.Debug(ctx, "waiting for agent reinitialization instructions")
			reinitEvent, err := client.WaitForReinit(ctx)
			if err != nil {
				logger.Error(ctx, "failed to wait for agent reinitialization instructions", slog.Error(err))
				continue
			}
			retrier.Reset()
			select {
			case <-ctx.Done():
				close(reinitEvents)
				return
			case reinitEvents <- *reinitEvent:
			}
		}
	}()

	return reinitEvents
}

func NewSSEAgentReinitTransmitter(logger slog.Logger, rw http.ResponseWriter, r *http.Request) *SSEAgentReinitTransmitter {
	return &SSEAgentReinitTransmitter{logger: logger, rw: rw, r: r}
}

type SSEAgentReinitTransmitter struct {
	rw     http.ResponseWriter
	r      *http.Request
	logger slog.Logger
}

var (
	ErrTransmissionSourceClosed = xerrors.New("transmission source closed")
	ErrTransmissionTargetClosed = xerrors.New("transmission target closed")
)

// Transmit will read from the given chan and send events for as long as:
// * the chan remains open
// * the context has not been canceled
// * not timed out
// * the connection to the receiver remains open
func (s *SSEAgentReinitTransmitter) Transmit(ctx context.Context, reinitEvents <-chan ReinitializationEvent) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	sseSendEvent, sseSenderClosed, err := httpapi.ServerSentEventSender(s.rw, s.r)
	if err != nil {
		return xerrors.Errorf("failed to create sse transmitter: %w", err)
	}

	defer func() {
		// Block returning until the ServerSentEventSender is closed
		// to avoid a race condition where we might write or flush to rw after the handler returns.
		<-sseSenderClosed
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sseSenderClosed:
			return ErrTransmissionTargetClosed
		case reinitEvent, ok := <-reinitEvents:
			if !ok {
				return ErrTransmissionSourceClosed
			}
			err := sseSendEvent(codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeData,
				Data: reinitEvent,
			})
			if err != nil {
				return err
			}
		}
	}
}

func NewSSEAgentReinitReceiver(r io.ReadCloser) *SSEAgentReinitReceiver {
	return &SSEAgentReinitReceiver{r: r}
}

type SSEAgentReinitReceiver struct {
	r io.ReadCloser
}

func (s *SSEAgentReinitReceiver) Receive(ctx context.Context) (*ReinitializationEvent, error) {
	nextEvent := codersdk.ServerSentEventReader(ctx, s.r)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		sse, err := nextEvent()
		switch {
		case err != nil:
			return nil, xerrors.Errorf("failed to read server-sent event: %w", err)
		case sse.Type == codersdk.ServerSentEventTypeError:
			return nil, xerrors.Errorf("unexpected server sent event type error")
		case sse.Type == codersdk.ServerSentEventTypePing:
			continue
		case sse.Type != codersdk.ServerSentEventTypeData:
			return nil, xerrors.Errorf("unexpected server sent event type: %s", sse.Type)
		}

		// At this point we know that the sent event is of type codersdk.ServerSentEventTypeData
		var reinitEvent ReinitializationEvent
		b, ok := sse.Data.([]byte)
		if !ok {
			return nil, xerrors.Errorf("expected data as []byte, got %T", sse.Data)
		}
		err = json.Unmarshal(b, &reinitEvent)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal reinit response: %w", err)
		}
		return &reinitEvent, nil
	}
}
