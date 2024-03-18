package codersdk

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/speedtest"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/tailnet"
)

// WorkspaceAgentIP is a static IPv6 address with the Tailscale prefix that is used to route
// connections from clients to this node. A dynamic address is not required because a Tailnet
// client only dials a single agent at a time.
//
// Deprecated: use tailnet.IP() instead. This is kept for backwards
// compatibility with outdated CLI clients and Workspace Proxies that dial it.
// See: https://github.com/coder/coder/issues/11819
var WorkspaceAgentIP = netip.MustParseAddr("fd7a:115c:a1e0:49d6:b259:b7ac:b1b2:48f4")

var ErrSkipClose = xerrors.New("skip tailnet close")

const (
	WorkspaceAgentSSHPort             = tailnet.WorkspaceAgentSSHPort
	WorkspaceAgentReconnectingPTYPort = tailnet.WorkspaceAgentReconnectingPTYPort
	WorkspaceAgentSpeedtestPort       = tailnet.WorkspaceAgentSpeedtestPort
	// WorkspaceAgentHTTPAPIServerPort serves a HTTP server with endpoints for e.g.
	// gathering agent statistics.
	WorkspaceAgentHTTPAPIServerPort = 4

	// WorkspaceAgentMinimumListeningPort is the minimum port that the listening-ports
	// endpoint will return to the client, and the minimum port that is accepted
	// by the proxy applications endpoint. Coder consumes ports 1-4 at the
	// moment, and we reserve some extra ports for future use. Port 9 and up are
	// available for the user.
	//
	// This is not enforced in the CLI intentionally as we don't really care
	// *that* much. The user could bypass this in the CLI by using SSH instead
	// anyways.
	WorkspaceAgentMinimumListeningPort = 9
)

// WorkspaceAgentIgnoredListeningPorts contains a list of ports to ignore when looking for
// running applications inside a workspace. We want to ignore non-HTTP servers,
// so we pre-populate this list with common ports that are not HTTP servers.
//
// This is implemented as a map for fast lookup.
var WorkspaceAgentIgnoredListeningPorts = map[uint16]struct{}{
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
		WorkspaceAgentIgnoredListeningPorts[uint16(i)] = struct{}{}
	}
}

// NewWorkspaceAgentConn creates a new WorkspaceAgentConn. `conn` may be unique
// to the WorkspaceAgentConn, or it may be shared in the case of coderd. If the
// conn is shared and closing it is undesirable, you may return ErrNoClose from
// opts.CloseFunc. This will ensure the underlying conn is not closed.
func NewWorkspaceAgentConn(conn *tailnet.Conn, opts WorkspaceAgentConnOptions) *WorkspaceAgentConn {
	return &WorkspaceAgentConn{
		Conn: conn,
		opts: opts,
	}
}

// WorkspaceAgentConn represents a connection to a workspace agent.
// @typescript-ignore WorkspaceAgentConn
type WorkspaceAgentConn struct {
	*tailnet.Conn
	opts WorkspaceAgentConnOptions
}

// @typescript-ignore WorkspaceAgentConnOptions
type WorkspaceAgentConnOptions struct {
	AgentID   uuid.UUID
	CloseFunc func() error
}

func (c *WorkspaceAgentConn) agentAddress() netip.Addr {
	return tailnet.IPFromUUID(c.opts.AgentID)
}

// AwaitReachable waits for the agent to be reachable.
func (c *WorkspaceAgentConn) AwaitReachable(ctx context.Context) bool {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	return c.Conn.AwaitReachable(ctx, c.agentAddress())
}

// Ping pings the agent and returns the round-trip time.
// The bool returns true if the ping was made P2P.
func (c *WorkspaceAgentConn) Ping(ctx context.Context) (time.Duration, bool, *ipnstate.PingResult, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	return c.Conn.Ping(ctx, c.agentAddress())
}

// Close ends the connection to the workspace agent.
func (c *WorkspaceAgentConn) Close() error {
	var cerr error
	if c.opts.CloseFunc != nil {
		cerr = c.opts.CloseFunc()
		if xerrors.Is(cerr, ErrSkipClose) {
			return nil
		}
	}
	if cerr != nil {
		return multierror.Append(cerr, c.Conn.Close())
	}
	return c.Conn.Close()
}

// WorkspaceAgentReconnectingPTYInit initializes a new reconnecting PTY session.
// @typescript-ignore WorkspaceAgentReconnectingPTYInit
type WorkspaceAgentReconnectingPTYInit struct {
	ID      uuid.UUID
	Height  uint16
	Width   uint16
	Command string
}

// ReconnectingPTYRequest is sent from the client to the server
// to pipe data to a PTY.
// @typescript-ignore ReconnectingPTYRequest
type ReconnectingPTYRequest struct {
	Data   string `json:"data,omitempty"`
	Height uint16 `json:"height,omitempty"`
	Width  uint16 `json:"width,omitempty"`
}

// ReconnectingPTY spawns a new reconnecting terminal session.
// `ReconnectingPTYRequest` should be JSON marshaled and written to the returned net.Conn.
// Raw terminal output will be read from the returned net.Conn.
func (c *WorkspaceAgentConn) ReconnectingPTY(ctx context.Context, id uuid.UUID, height, width uint16, command string) (net.Conn, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	if !c.AwaitReachable(ctx) {
		return nil, xerrors.Errorf("workspace agent not reachable in time: %v", ctx.Err())
	}

	conn, err := c.Conn.DialContextTCP(ctx, netip.AddrPortFrom(c.agentAddress(), WorkspaceAgentReconnectingPTYPort))
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(WorkspaceAgentReconnectingPTYInit{
		ID:      id,
		Height:  height,
		Width:   width,
		Command: command,
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	data = append(make([]byte, 2), data...)
	binary.LittleEndian.PutUint16(data, uint16(len(data)-2))

	_, err = conn.Write(data)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// SSH pipes the SSH protocol over the returned net.Conn.
// This connects to the built-in SSH server in the workspace agent.
func (c *WorkspaceAgentConn) SSH(ctx context.Context) (*gonet.TCPConn, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	if !c.AwaitReachable(ctx) {
		return nil, xerrors.Errorf("workspace agent not reachable in time: %v", ctx.Err())
	}

	return c.Conn.DialContextTCP(ctx, netip.AddrPortFrom(c.agentAddress(), WorkspaceAgentSSHPort))
}

// SSHClient calls SSH to create a client that uses a weak cipher
// to improve throughput.
func (c *WorkspaceAgentConn) SSHClient(ctx context.Context) (*ssh.Client, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	netConn, err := c.SSH(ctx)
	if err != nil {
		return nil, xerrors.Errorf("ssh: %w", err)
	}

	sshConn, channels, requests, err := ssh.NewClientConn(netConn, "localhost:22", &ssh.ClientConfig{
		// SSH host validation isn't helpful, because obtaining a peer
		// connection already signifies user-intent to dial a workspace.
		// #nosec
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return nil, xerrors.Errorf("ssh conn: %w", err)
	}

	return ssh.NewClient(sshConn, channels, requests), nil
}

// Speedtest runs a speedtest against the workspace agent.
func (c *WorkspaceAgentConn) Speedtest(ctx context.Context, direction speedtest.Direction, duration time.Duration) ([]speedtest.Result, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	if !c.AwaitReachable(ctx) {
		return nil, xerrors.Errorf("workspace agent not reachable in time: %v", ctx.Err())
	}

	speedConn, err := c.Conn.DialContextTCP(ctx, netip.AddrPortFrom(c.agentAddress(), WorkspaceAgentSpeedtestPort))
	if err != nil {
		return nil, xerrors.Errorf("dial speedtest: %w", err)
	}

	results, err := speedtest.RunClientWithConn(direction, duration, speedConn)
	if err != nil {
		return nil, xerrors.Errorf("run speedtest: %w", err)
	}

	return results, err
}

// DialContext dials the address provided in the workspace agent.
// The network must be "tcp" or "udp".
func (c *WorkspaceAgentConn) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	if !c.AwaitReachable(ctx) {
		return nil, xerrors.Errorf("workspace agent not reachable in time: %v", ctx.Err())
	}

	_, rawPort, _ := net.SplitHostPort(addr)
	port, _ := strconv.ParseUint(rawPort, 10, 16)
	ipp := netip.AddrPortFrom(c.agentAddress(), uint16(port))

	switch network {
	case "tcp":
		return c.Conn.DialContextTCP(ctx, ipp)
	case "udp":
		return c.Conn.DialContextUDP(ctx, ipp)
	default:
		return nil, xerrors.Errorf("unknown network %q", network)
	}
}

type WorkspaceAgentListeningPortsResponse struct {
	// If there are no ports in the list, nothing should be displayed in the UI.
	// There must not be a "no ports available" message or anything similar, as
	// there will always be no ports displayed on platforms where our port
	// detection logic is unsupported.
	Ports []WorkspaceAgentListeningPort `json:"ports"`
}

type WorkspaceAgentListeningPort struct {
	ProcessName string `json:"process_name"` // may be empty
	Network     string `json:"network"`      // only "tcp" at the moment
	Port        uint16 `json:"port"`
}

// ListeningPorts lists the ports that are currently in use by the workspace.
func (c *WorkspaceAgentConn) ListeningPorts(ctx context.Context) (WorkspaceAgentListeningPortsResponse, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/api/v0/listening-ports", nil)
	if err != nil {
		return WorkspaceAgentListeningPortsResponse{}, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentListeningPortsResponse{}, ReadBodyAsError(res)
	}

	var resp WorkspaceAgentListeningPortsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DebugMagicsock makes a request to the workspace agent's magicsock debug endpoint.
func (c *WorkspaceAgentConn) DebugMagicsock(ctx context.Context) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/debug/magicsock", nil)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	defer res.Body.Close()
	bs, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, xerrors.Errorf("read response body: %w", err)
	}
	return bs, nil
}

// DebugManifest returns the agent's in-memory manifest. Unfortunately this must
// be returns as a []byte to avoid an import cycle.
func (c *WorkspaceAgentConn) DebugManifest(ctx context.Context) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/debug/manifest", nil)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	bs, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, xerrors.Errorf("read response body: %w", err)
	}
	return bs, nil
}

// DebugLogs returns up to the last 10MB of `/tmp/coder-agent.log`
func (c *WorkspaceAgentConn) DebugLogs(ctx context.Context) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/debug/logs", nil)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	bs, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, xerrors.Errorf("read response body: %w", err)
	}
	return bs, nil
}

// PrometheusMetrics returns a response from the agent's prometheus metrics endpoint
func (c *WorkspaceAgentConn) PrometheusMetrics(ctx context.Context) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/debug/prometheus", nil)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	bs, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, xerrors.Errorf("read response body: %w", err)
	}
	return bs, nil
}

// apiRequest makes a request to the workspace agent's HTTP API server.
func (c *WorkspaceAgentConn) apiRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	host := net.JoinHostPort(c.agentAddress().String(), strconv.Itoa(WorkspaceAgentHTTPAPIServerPort))
	url := fmt.Sprintf("http://%s%s", host, path)

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, xerrors.Errorf("new http api request to %q: %w", url, err)
	}

	return c.apiClient().Do(req)
}

// apiClient returns an HTTP client that can be used to make
// requests to the workspace agent's HTTP API server.
func (c *WorkspaceAgentConn) apiClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			// Disable keep alives as we're usually only making a single
			// request, and this triggers goleak in tests
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if network != "tcp" {
					return nil, xerrors.Errorf("network must be tcp")
				}

				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, xerrors.Errorf("split host port %q: %w", addr, err)
				}

				// Verify that the port is TailnetStatisticsPort.
				if port != strconv.Itoa(WorkspaceAgentHTTPAPIServerPort) {
					return nil, xerrors.Errorf("request %q does not appear to be for http api", addr)
				}

				if !c.AwaitReachable(ctx) {
					return nil, xerrors.Errorf("workspace agent not reachable in time: %v", ctx.Err())
				}

				ipAddr, err := netip.ParseAddr(host)
				if err != nil {
					return nil, xerrors.Errorf("parse host addr: %w", err)
				}

				conn, err := c.Conn.DialContextTCP(ctx, netip.AddrPortFrom(ipAddr, WorkspaceAgentHTTPAPIServerPort))
				if err != nil {
					return nil, xerrors.Errorf("dial http api: %w", err)
				}

				return conn, nil
			},
		},
	}
}

func (c *WorkspaceAgentConn) GetPeerDiagnostics() tailnet.PeerDiagnostics {
	return c.Conn.GetPeerDiagnostics(c.opts.AgentID)
}
