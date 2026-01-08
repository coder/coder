package workspacesdk

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
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

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/websocket"
)

// NewAgentConn creates a new WorkspaceAgentConn. `conn` may be unique
// to the WorkspaceAgentConn, or it may be shared in the case of coderd. If the
// conn is shared and closing it is undesirable, you may return ErrNoClose from
// opts.CloseFunc. This will ensure the underlying conn is not closed.
func NewAgentConn(conn *tailnet.Conn, opts AgentConnOptions) AgentConn {
	return &agentConn{
		Conn: conn,
		opts: opts,
	}
}

// AgentConn represents a connection to a workspace agent.
// @typescript-ignore AgentConn
type AgentConn interface {
	TailnetConn() *tailnet.Conn

	AwaitReachable(ctx context.Context) bool
	Close() error
	DebugLogs(ctx context.Context) ([]byte, error)
	DebugMagicsock(ctx context.Context) ([]byte, error)
	DebugManifest(ctx context.Context) ([]byte, error)
	DialContext(ctx context.Context, network string, addr string) (net.Conn, error)
	GetPeerDiagnostics() tailnet.PeerDiagnostics
	ListContainers(ctx context.Context) (codersdk.WorkspaceAgentListContainersResponse, error)
	ListeningPorts(ctx context.Context) (codersdk.WorkspaceAgentListeningPortsResponse, error)
	Netcheck(ctx context.Context) (healthsdk.AgentNetcheckReport, error)
	Ping(ctx context.Context) (time.Duration, bool, *ipnstate.PingResult, error)
	PrometheusMetrics(ctx context.Context) ([]byte, error)
	ReconnectingPTY(ctx context.Context, id uuid.UUID, height uint16, width uint16, command string, initOpts ...AgentReconnectingPTYInitOption) (net.Conn, error)
	DeleteDevcontainer(ctx context.Context, devcontainerID string) error
	RecreateDevcontainer(ctx context.Context, devcontainerID string) (codersdk.Response, error)
	LS(ctx context.Context, path string, req LSRequest) (LSResponse, error)
	ReadFile(ctx context.Context, path string, offset, limit int64) (io.ReadCloser, string, error)
	WriteFile(ctx context.Context, path string, reader io.Reader) error
	EditFiles(ctx context.Context, edits FileEditRequest) error
	SSH(ctx context.Context) (*gonet.TCPConn, error)
	SSHClient(ctx context.Context) (*ssh.Client, error)
	SSHClientOnPort(ctx context.Context, port uint16) (*ssh.Client, error)
	SSHOnPort(ctx context.Context, port uint16) (*gonet.TCPConn, error)
	Speedtest(ctx context.Context, direction speedtest.Direction, duration time.Duration) ([]speedtest.Result, error)
	WatchContainers(ctx context.Context, logger slog.Logger) (<-chan codersdk.WorkspaceAgentListContainersResponse, io.Closer, error)
}

// AgentConn represents a connection to a workspace agent.
// @typescript-ignore AgentConn
type agentConn struct {
	*tailnet.Conn
	opts AgentConnOptions
}

func (c *agentConn) TailnetConn() *tailnet.Conn {
	return c.Conn
}

// @typescript-ignore AgentConnOptions
type AgentConnOptions struct {
	AgentID   uuid.UUID
	CloseFunc func() error
}

func (c *agentConn) agentAddress() netip.Addr {
	return tailnet.TailscaleServicePrefix.AddrFromUUID(c.opts.AgentID)
}

// AwaitReachable waits for the agent to be reachable.
func (c *agentConn) AwaitReachable(ctx context.Context) bool {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	return c.Conn.AwaitReachable(ctx, c.agentAddress())
}

// Ping pings the agent and returns the round-trip time.
// The bool returns true if the ping was made P2P.
func (c *agentConn) Ping(ctx context.Context) (time.Duration, bool, *ipnstate.PingResult, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	return c.Conn.Ping(ctx, c.agentAddress())
}

// Close ends the connection to the workspace agent.
func (c *agentConn) Close() error {
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

	BackendType string
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
func (c *agentConn) ReconnectingPTY(ctx context.Context, id uuid.UUID, height, width uint16, command string, initOpts ...AgentReconnectingPTYInitOption) (net.Conn, error) {
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
	// #nosec G115 - Safe conversion as the data length is expected to be within uint16 range for PTY initialization
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
func (c *agentConn) SSH(ctx context.Context) (*gonet.TCPConn, error) {
	return c.SSHOnPort(ctx, AgentSSHPort)
}

// SSHOnPort pipes the SSH protocol over the returned net.Conn.
// This connects to the built-in SSH server in the workspace agent on the specified port.
func (c *agentConn) SSHOnPort(ctx context.Context, port uint16) (*gonet.TCPConn, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	if !c.AwaitReachable(ctx) {
		return nil, xerrors.Errorf("workspace agent not reachable in time: %v", ctx.Err())
	}

	c.SendConnectedTelemetry(c.agentAddress(), tailnet.TelemetryApplicationSSH)
	return c.DialContextTCP(ctx, netip.AddrPortFrom(c.agentAddress(), port))
}

// SSHClient calls SSH to create a client
func (c *agentConn) SSHClient(ctx context.Context) (*ssh.Client, error) {
	return c.SSHClientOnPort(ctx, AgentSSHPort)
}

// SSHClientOnPort calls SSH to create a client on a specific port
func (c *agentConn) SSHClientOnPort(ctx context.Context, port uint16) (*ssh.Client, error) {
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
func (c *agentConn) Speedtest(ctx context.Context, direction speedtest.Direction, duration time.Duration) ([]speedtest.Result, error) {
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
func (c *agentConn) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
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
func (c *agentConn) ListeningPorts(ctx context.Context) (codersdk.WorkspaceAgentListeningPortsResponse, error) {
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
func (c *agentConn) Netcheck(ctx context.Context) (healthsdk.AgentNetcheckReport, error) {
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
func (c *agentConn) DebugMagicsock(ctx context.Context) ([]byte, error) {
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
func (c *agentConn) DebugManifest(ctx context.Context) ([]byte, error) {
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
func (c *agentConn) DebugLogs(ctx context.Context) ([]byte, error) {
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
func (c *agentConn) PrometheusMetrics(ctx context.Context) ([]byte, error) {
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
func (c *agentConn) ListContainers(ctx context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
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

func (c *agentConn) WatchContainers(ctx context.Context, logger slog.Logger) (<-chan codersdk.WorkspaceAgentListContainersResponse, io.Closer, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	host := net.JoinHostPort(c.agentAddress().String(), strconv.Itoa(AgentHTTPAPIServerPort))
	url := fmt.Sprintf("http://%s%s", host, "/api/v0/containers/watch")

	conn, res, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPClient: c.apiClient(),

		// We want `NoContextTakeover` compression to balance improving
		// bandwidth cost/latency with minimal memory usage overhead.
		CompressionMode: websocket.CompressionNoContextTakeover,
	})
	if err != nil {
		if res == nil {
			return nil, nil, err
		}
		return nil, nil, codersdk.ReadBodyAsError(res)
	}
	if res != nil && res.Body != nil {
		defer res.Body.Close()
	}

	// When a workspace has a few devcontainers running, or a single devcontainer
	// has a large amount of apps, then each payload can easily exceed 32KiB.
	// We up the limit to 4MiB to give us plenty of headroom for workspaces that
	// have lots of dev containers with lots of apps.
	conn.SetReadLimit(1 << 22) // 4MiB

	d := wsjson.NewDecoder[codersdk.WorkspaceAgentListContainersResponse](conn, websocket.MessageText, logger)
	return d.Chan(), d, nil
}

// DeleteDevcontainer deletes the provided devcontainer.
// This is a blocking call and will wait for the container to be deleted.
func (c *agentConn) DeleteDevcontainer(ctx context.Context, devcontainerID string) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodDelete, "/api/v0/containers/devcontainers/"+devcontainerID, nil)
	if err != nil {
		return xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}

// RecreateDevcontainer recreates a devcontainer with the given container.
// This is a blocking call and will wait for the container to be recreated.
func (c *agentConn) RecreateDevcontainer(ctx context.Context, devcontainerID string) (codersdk.Response, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	res, err := c.apiRequest(ctx, http.MethodPost, "/api/v0/containers/devcontainers/"+devcontainerID+"/recreate", nil)
	if err != nil {
		return codersdk.Response{}, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		return codersdk.Response{}, codersdk.ReadBodyAsError(res)
	}
	var m codersdk.Response
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		return codersdk.Response{}, xerrors.Errorf("decode response body: %w", err)
	}
	return m, nil
}

type LSRequest struct {
	// e.g. [], ["repos", "coder"],
	Path []string `json:"path"`
	// Whether the supplied path is relative to the user's home directory,
	// or the root directory.
	Relativity LSRelativity `json:"relativity"`
}

type LSRelativity string

const (
	LSRelativityRoot LSRelativity = "root"
	LSRelativityHome LSRelativity = "home"
)

type LSResponse struct {
	AbsolutePath []string `json:"absolute_path"`
	// Returned so clients can display the full path to the user, and
	// copy it to configure file sync
	// e.g. Windows: "C:\\Users\\coder"
	//      Linux: "/home/coder"
	AbsolutePathString string   `json:"absolute_path_string"`
	Contents           []LSFile `json:"contents"`
}

type LSFile struct {
	Name string `json:"name"`
	// e.g. "C:\\Users\\coder\\hello.txt"
	//      "/home/coder/hello.txt"
	AbsolutePathString string `json:"absolute_path_string"`
	IsDir              bool   `json:"is_dir"`
}

// LS lists a directory.
func (c *agentConn) LS(ctx context.Context, path string, req LSRequest) (LSResponse, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	res, err := c.apiRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v0/list-directory?path=%s", path), req)
	if err != nil {
		return LSResponse{}, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return LSResponse{}, codersdk.ReadBodyAsError(res)
	}

	var m LSResponse
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		return LSResponse{}, xerrors.Errorf("decode response body: %w", err)
	}
	return m, nil
}

// ReadFile reads from a file from the workspace, returning a file reader and
// the mime type.
func (c *agentConn) ReadFile(ctx context.Context, path string, offset, limit int64) (io.ReadCloser, string, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	//nolint:bodyclose // we want to return the body so the caller can stream.
	res, err := c.apiRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v0/read-file?path=%s&offset=%d&limit=%d", path, offset, limit), nil)
	if err != nil {
		return nil, "", xerrors.Errorf("do request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		// codersdk.ReadBodyAsError will close the body.
		return nil, "", codersdk.ReadBodyAsError(res)
	}

	mimeType := res.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return res.Body, mimeType, nil
}

// WriteFile writes to a file in the workspace.
func (c *agentConn) WriteFile(ctx context.Context, path string, reader io.Reader) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	res, err := c.apiRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v0/write-file?path=%s", path), reader)
	if err != nil {
		return xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.ReadBodyAsError(res)
	}

	var m codersdk.Response
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		return xerrors.Errorf("decode response body: %w", err)
	}
	return nil
}

type FileEdit struct {
	Search  string `json:"search"`
	Replace string `json:"replace"`
}

type FileEdits struct {
	Path  string     `json:"path"`
	Edits []FileEdit `json:"edits"`
}

type FileEditRequest struct {
	Files []FileEdits `json:"files"`
}

// EditFiles performs search and replace edits on one or more files.
func (c *agentConn) EditFiles(ctx context.Context, edits FileEditRequest) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	res, err := c.apiRequest(ctx, http.MethodPost, "/api/v0/edit-files", edits)
	if err != nil {
		return xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.ReadBodyAsError(res)
	}

	var m codersdk.Response
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		return xerrors.Errorf("decode response body: %w", err)
	}
	return nil
}

// apiRequest makes a request to the workspace agent's HTTP API server.
func (c *agentConn) apiRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	host := net.JoinHostPort(c.agentAddress().String(), strconv.Itoa(AgentHTTPAPIServerPort))
	url := fmt.Sprintf("http://%s%s", host, path)

	var r io.Reader
	if body != nil {
		switch data := body.(type) {
		case io.Reader:
			r = data
		case []byte:
			r = bytes.NewReader(data)
		default:
			// Assume JSON in all other cases.
			buf := bytes.NewBuffer(nil)
			enc := json.NewEncoder(buf)
			enc.SetEscapeHTML(false)
			err := enc.Encode(body)
			if err != nil {
				return nil, xerrors.Errorf("encode body: %w", err)
			}
			r = buf
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return nil, xerrors.Errorf("new http api request to %q: %w", url, err)
	}

	return c.apiClient().Do(req)
}

// apiClient returns an HTTP client that can be used to make
// requests to the workspace agent's HTTP API server.
func (c *agentConn) apiClient() *http.Client {
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

func (c *agentConn) GetPeerDiagnostics() tailnet.PeerDiagnostics {
	return c.Conn.GetPeerDiagnostics(c.opts.AgentID)
}
