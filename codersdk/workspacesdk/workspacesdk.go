package workspacesdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"
	"tailscale.com/wgengine/capture"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
)

// AgentIP is a static IPv6 address with the Tailscale prefix that is used to route
// connections from clients to this node. A dynamic address is not required because a Tailnet
// client only dials a single agent at a time.
//
// Deprecated: use tailnet.IP() instead. This is kept for backwards
// compatibility with outdated CLI clients and Workspace Proxies that dial it.
// See: https://github.com/coder/coder/issues/11819
var AgentIP = netip.MustParseAddr("fd7a:115c:a1e0:49d6:b259:b7ac:b1b2:48f4")

var ErrSkipClose = xerrors.New("skip tailnet close")

const (
	AgentSSHPort             = tailnet.WorkspaceAgentSSHPort
	AgentReconnectingPTYPort = tailnet.WorkspaceAgentReconnectingPTYPort
	AgentSpeedtestPort       = tailnet.WorkspaceAgentSpeedtestPort
	// AgentHTTPAPIServerPort serves a HTTP server with endpoints for e.g.
	// gathering agent statistics.
	AgentHTTPAPIServerPort = 4

	// AgentMinimumListeningPort is the minimum port that the listening-ports
	// endpoint will return to the client, and the minimum port that is accepted
	// by the proxy applications endpoint. Coder consumes ports 1-4 at the
	// moment, and we reserve some extra ports for future use. Port 9 and up are
	// available for the user.
	//
	// This is not enforced in the CLI intentionally as we don't really care
	// *that* much. The user could bypass this in the CLI by using SSH instead
	// anyways.
	AgentMinimumListeningPort = 9
)

// AgentIgnoredListeningPorts contains a list of ports to ignore when looking for
// running applications inside a workspace. We want to ignore non-HTTP servers,
// so we pre-populate this list with common ports that are not HTTP servers.
//
// This is implemented as a map for fast lookup.
var AgentIgnoredListeningPorts = map[uint16]struct{}{
	0: {},
	// Ports 1-8 are reserved for future use by the Coder agent.
	1: {},
	2: {},
	3: {},
	4: {},
	5: {},
	6: {},
	7: {},
	8: {},
	// ftp
	20: {},
	21: {},
	// ssh
	22: {},
	// telnet
	23: {},
	// smtp
	25: {},
	// dns over TCP
	53: {},
	// pop3
	110: {},
	// imap
	143: {},
	// bgp
	179: {},
	// ldap
	389: {},
	636: {},
	// smtps
	465: {},
	// smtp
	587: {},
	// ftps
	989: {},
	990: {},
	// imaps
	993: {},
	// pop3s
	995: {},
	// mysql
	3306: {},
	// rdp
	3389: {},
	// postgres
	5432: {},
	// mongodb
	27017: {},
	27018: {},
	27019: {},
	28017: {},
}

func init() {
	if !strings.HasSuffix(os.Args[0], ".test") {
		return
	}
	// Add a thousand more ports to the ignore list during tests so it's easier
	// to find an available port.
	for i := 63000; i < 64000; i++ {
		AgentIgnoredListeningPorts[uint16(i)] = struct{}{}
	}
}

type Client struct {
	client *codersdk.Client
}

func New(c *codersdk.Client) *Client {
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
	// CaptureHook is a callback that captures Disco packets and packets sent
	// into the tailnet tunnel.
	CaptureHook capture.Callback
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
		CaptureHook:         options.CaptureHook,
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
