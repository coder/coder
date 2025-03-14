package workspacesdk

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/speedtest"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/coder/v2/tailnet"
)

// NewAgentConn creates a new WorkspaceAgentConn. `conn` may be unique
// to the WorkspaceAgentConn, or it may be shared in the case of coderd. If the
// conn is shared and closing it is undesirable, you may return ErrNoClose from
// opts.CloseFunc. This will ensure the underlying conn is not closed.
func NewAgentConn(conn *tailnet.Conn, opts AgentConnOptions) *AgentConn {
	return &AgentConn{
		Conn: conn,
		opts: opts,
	}
}

// AgentConn represents a connection to a workspace agent.
// @typescript-ignore AgentConn
type AgentConn struct {
	*tailnet.Conn
	opts AgentConnOptions
}

// @typescript-ignore AgentConnOptions
type AgentConnOptions struct {
	AgentID   uuid.UUID
	CloseFunc func() error
}

func (c *AgentConn) agentAddress() netip.Addr {
	return tailnet.TailscaleServicePrefix.AddrFromUUID(c.opts.AgentID)
}

// AwaitReachable waits for the agent to be reachable.
func (c *AgentConn) AwaitReachable(ctx context.Context) bool {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	return c.Conn.AwaitReachable(ctx, c.agentAddress())
}

// Ping pings the agent and returns the round-trip time.
// The bool returns true if the ping was made P2P.
func (c *AgentConn) Ping(ctx context.Context) (time.Duration, bool, *ipnstate.PingResult, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	return c.Conn.Ping(ctx, c.agentAddress())
}

// Close ends the connection to the workspace agent.
func (c *AgentConn) Close() error {
	var cerr error
	if c.opts.CloseFunc != nil {
		cerr = c.opts.CloseFunc()
		if errors.Is(cerr, ErrSkipClose) {
			return nil
		}
	}
	if cerr != nil {
		return multierror.Append(cerr, c.Conn.Close())
	}
	return c.Conn.Close()
}

// AgentReconnectingPTYInit initializes a new reconnecting PTY session.
// @typescript-ignore AgentReconnectingPTYInit
type AgentReconnectingPTYInit struct {
	ID      uuid.UUID
	Height  uint16
	Width   uint16
	Command string
	// Container, if set, will attempt to exec into a running container visible to the agent.
	// This should be a unique container ID (implementation-dependent).
	Container string
	// ContainerUser, if set, will set the target user when execing into a container.
	// This can be a username or UID, depending on the underlying implementation.
	// This is ignored if Container is not set.
	ContainerUser string
}

// AgentReconnectingPTYInitOption is a functional option for AgentReconnectingPTYInit.
type AgentReconnectingPTYInitOption func(*AgentReconnectingPTYInit)

// AgentReconnectingPTYInitWithContainer sets the container and container user for the reconnecting PTY session.
func AgentReconnectingPTYInitWithContainer(container, containerUser string) AgentReconnectingPTYInitOption {
	return func(init *AgentReconnectingPTYInit) {
		init.Container = container
		init.ContainerUser = containerUser
	}
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
func (c *AgentConn) ReconnectingPTY(ctx context.Context, id uuid.UUID, height, width uint16, command string, initOpts ...AgentReconnectingPTYInitOption) (net.Conn, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	if !c.AwaitReachable(ctx) {
		return nil, xerrors.Errorf("workspace agent not reachable in time: %v", ctx.Err())
	}

	conn, err := c.Conn.DialContextTCP(ctx, netip.AddrPortFrom(c.agentAddress(), AgentReconnectingPTYPort))
	if err != nil {
		return nil, err
	}
	rptyInit := AgentReconnectingPTYInit{
		ID:      id,
		Height:  height,
		Width:   width,
		Command: command,
	}
	for _, o := range initOpts {
		o(&rptyInit)
	}
	data, err := json.Marshal(rptyInit)
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
func (c *AgentConn) SSH(ctx context.Context) (*gonet.TCPConn, error) {
	return c.SSHOnPort(ctx, AgentSSHPort)
}

// SSHOnPort pipes the SSH protocol over the returned net.Conn.
// This connects to the built-in SSH server in the workspace agent on the specified port.
func (c *AgentConn) SSHOnPort(ctx context.Context, port uint16) (*gonet.TCPConn, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	if !c.AwaitReachable(ctx) {
		return nil, xerrors.Errorf("workspace agent not reachable in time: %v", ctx.Err())
	}

	c.SendConnectedTelemetry(c.agentAddress(), tailnet.TelemetryApplicationSSH)
	return c.DialContextTCP(ctx, netip.AddrPortFrom(c.agentAddress(), port))
}

// SSHClient calls SSH to create a client that uses a weak cipher
// to improve throughput.
func (c *AgentConn) SSHClient(ctx context.Context) (*ssh.Client, error) {
	return c.SSHClientOnPort(ctx, AgentSSHPort)
}

// SSHClientOnPort calls SSH to create a client on a specific port
// that uses a weak cipher to improve throughput.
func (c *AgentConn) SSHClientOnPort(ctx context.Context, port uint16) (*ssh.Client, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	netConn, err := c.SSHOnPort(ctx, port)
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
func (c *AgentConn) Speedtest(ctx context.Context, direction speedtest.Direction, duration time.Duration) ([]speedtest.Result, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	if !c.AwaitReachable(ctx) {
		return nil, xerrors.Errorf("workspace agent not reachable in time: %v", ctx.Err())
	}

	c.Conn.SendConnectedTelemetry(c.agentAddress(), tailnet.TelemetryApplicationSpeedtest)
	speedConn, err := c.Conn.DialContextTCP(ctx, netip.AddrPortFrom(c.agentAddress(), AgentSpeedtestPort))
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
func (c *AgentConn) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
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

// ListeningPorts lists the ports that are currently in use by the workspace.
func (c *AgentConn) ListeningPorts(ctx context.Context) (codersdk.WorkspaceAgentListeningPortsResponse, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/api/v0/listening-ports", nil)
	if err != nil {
		return codersdk.WorkspaceAgentListeningPortsResponse{}, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.WorkspaceAgentListeningPortsResponse{}, codersdk.ReadBodyAsError(res)
	}

	var resp codersdk.WorkspaceAgentListeningPortsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// Netcheck returns a network check report from the workspace agent.
func (c *AgentConn) Netcheck(ctx context.Context) (healthsdk.AgentNetcheckReport, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/api/v0/netcheck", nil)
	if err != nil {
		return healthsdk.AgentNetcheckReport{}, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return healthsdk.AgentNetcheckReport{}, codersdk.ReadBodyAsError(res)
	}

	var resp healthsdk.AgentNetcheckReport
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DebugMagicsock makes a request to the workspace agent's magicsock debug endpoint.
func (c *AgentConn) DebugMagicsock(ctx context.Context) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/debug/magicsock", nil)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, codersdk.ReadBodyAsError(res)
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
func (c *AgentConn) DebugManifest(ctx context.Context) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/debug/manifest", nil)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, codersdk.ReadBodyAsError(res)
	}
	bs, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, xerrors.Errorf("read response body: %w", err)
	}
	return bs, nil
}

// DebugLogs returns up to the last 10MB of `/tmp/coder-agent.log`
func (c *AgentConn) DebugLogs(ctx context.Context) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/debug/logs", nil)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, codersdk.ReadBodyAsError(res)
	}
	bs, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, xerrors.Errorf("read response body: %w", err)
	}
	return bs, nil
}

// PrometheusMetrics returns a response from the agent's prometheus metrics endpoint
func (c *AgentConn) PrometheusMetrics(ctx context.Context) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/debug/prometheus", nil)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, codersdk.ReadBodyAsError(res)
	}
	bs, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, xerrors.Errorf("read response body: %w", err)
	}
	return bs, nil
}

// ListContainers returns a response from the agent's containers endpoint
func (c *AgentConn) ListContainers(ctx context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodGet, "/api/v0/containers", nil)
	if err != nil {
		return codersdk.WorkspaceAgentListContainersResponse{}, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.WorkspaceAgentListContainersResponse{}, codersdk.ReadBodyAsError(res)
	}
	var resp codersdk.WorkspaceAgentListContainersResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// apiRequest makes a request to the workspace agent's HTTP API server.
func (c *AgentConn) apiRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	host := net.JoinHostPort(c.agentAddress().String(), strconv.Itoa(AgentHTTPAPIServerPort))
	url := fmt.Sprintf("http://%s%s", host, path)

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, xerrors.Errorf("new http api request to %q: %w", url, err)
	}

	return c.apiClient().Do(req)
}

// apiClient returns an HTTP client that can be used to make
// requests to the workspace agent's HTTP API server.
func (c *AgentConn) apiClient() *http.Client {
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
				if port != strconv.Itoa(AgentHTTPAPIServerPort) {
					return nil, xerrors.Errorf("request %q does not appear to be for http api", addr)
				}

				if !c.AwaitReachable(ctx) {
					return nil, xerrors.Errorf("workspace agent not reachable in time: %v", ctx.Err())
				}

				ipAddr, err := netip.ParseAddr(host)
				if err != nil {
					return nil, xerrors.Errorf("parse host addr: %w", err)
				}

				conn, err := c.Conn.DialContextTCP(ctx, netip.AddrPortFrom(ipAddr, AgentHTTPAPIServerPort))
				if err != nil {
					return nil, xerrors.Errorf("dial http api: %w", err)
				}

				return conn, nil
			},
		},
	}
}

func (c *AgentConn) GetPeerDiagnostics() tailnet.PeerDiagnostics {
	return c.Conn.GetPeerDiagnostics(c.opts.AgentID)
}
