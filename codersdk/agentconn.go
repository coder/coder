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
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/speedtest"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/tailnet"
)

var (
	// TailnetIP is a static IPv6 address with the Tailscale prefix that is used to route
	// connections from clients to this node. A dynamic address is not required because a Tailnet
	// client only dials a single agent at a time.
	TailnetIP                  = netip.MustParseAddr("fd7a:115c:a1e0:49d6:b259:b7ac:b1b2:48f4")
	TailnetSSHPort             = 1
	TailnetReconnectingPTYPort = 2
	TailnetSpeedtestPort       = 3
	// TailnetStatisticsPort serves a HTTP server with endpoints for gathering
	// agent statistics.
	TailnetStatisticsPort = 4

	// MinimumListeningPort is the minimum port that the listening-ports
	// endpoint will return to the client, and the minimum port that is accepted
	// by the proxy applications endpoint. Coder consumes ports 1-4 at the
	// moment, and we reserve some extra ports for future use. Port 9 and up are
	// available for the user.
	//
	// This is not enforced in the CLI intentionally as we don't really care
	// *that* much. The user could bypass this in the CLI by using SSH instead
	// anyways.
	MinimumListeningPort = 9
)

// ReconnectingPTYRequest is sent from the client to the server
// to pipe data to a PTY.
// @typescript-ignore ReconnectingPTYRequest
type ReconnectingPTYRequest struct {
	Data   string `json:"data"`
	Height uint16 `json:"height"`
	Width  uint16 `json:"width"`
}

// @typescript-ignore AgentConn
type AgentConn struct {
	*tailnet.Conn
	CloseFunc func()
}

func (c *AgentConn) Ping() (time.Duration, error) {
	errCh := make(chan error, 1)
	durCh := make(chan time.Duration, 1)
	c.Conn.Ping(TailnetIP, tailcfg.PingDisco, func(pr *ipnstate.PingResult) {
		if pr.Err != "" {
			errCh <- xerrors.New(pr.Err)
			return
		}
		durCh <- time.Duration(pr.LatencySeconds * float64(time.Second))
	})
	select {
	case err := <-errCh:
		return 0, err
	case dur := <-durCh:
		return dur, nil
	}
}

func (c *AgentConn) CloseWithError(_ error) error {
	return c.Close()
}

func (c *AgentConn) Close() error {
	if c.CloseFunc != nil {
		c.CloseFunc()
	}
	return c.Conn.Close()
}

// @typescript-ignore ReconnectingPTYInit
type ReconnectingPTYInit struct {
	ID      string
	Height  uint16
	Width   uint16
	Command string
}

func (c *AgentConn) ReconnectingPTY(id string, height, width uint16, command string) (net.Conn, error) {
	conn, err := c.DialContextTCP(context.Background(), netip.AddrPortFrom(TailnetIP, uint16(TailnetReconnectingPTYPort)))
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(ReconnectingPTYInit{
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

func (c *AgentConn) SSH() (net.Conn, error) {
	return c.DialContextTCP(context.Background(), netip.AddrPortFrom(TailnetIP, uint16(TailnetSSHPort)))
}

// SSHClient calls SSH to create a client that uses a weak cipher
// for high throughput.
func (c *AgentConn) SSHClient() (*ssh.Client, error) {
	netConn, err := c.SSH()
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

func (c *AgentConn) Speedtest(direction speedtest.Direction, duration time.Duration) ([]speedtest.Result, error) {
	speedConn, err := c.DialContextTCP(context.Background(), netip.AddrPortFrom(TailnetIP, uint16(TailnetSpeedtestPort)))
	if err != nil {
		return nil, xerrors.Errorf("dial speedtest: %w", err)
	}
	results, err := speedtest.RunClientWithConn(direction, duration, speedConn)
	if err != nil {
		return nil, xerrors.Errorf("run speedtest: %w", err)
	}
	return results, err
}

func (c *AgentConn) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	if network == "unix" {
		return nil, xerrors.New("network must be tcp or udp")
	}
	_, rawPort, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(rawPort)
	ipp := netip.AddrPortFrom(TailnetIP, uint16(port))
	if network == "udp" {
		return c.Conn.DialContextUDP(ctx, ipp)
	}
	return c.Conn.DialContextTCP(ctx, ipp)
}

func (c *AgentConn) statisticsClient() *http.Client {
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
				// Verify that host is TailnetIP and port is
				// TailnetStatisticsPort.
				if host != TailnetIP.String() || port != strconv.Itoa(TailnetStatisticsPort) {
					return nil, xerrors.Errorf("request %q does not appear to be for statistics server", addr)
				}

				conn, err := c.DialContextTCP(context.Background(), netip.AddrPortFrom(TailnetIP, uint16(TailnetStatisticsPort)))
				if err != nil {
					return nil, xerrors.Errorf("dial statistics: %w", err)
				}

				return conn, nil
			},
		},
	}
}

func (c *AgentConn) doStatisticsRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	host := net.JoinHostPort(TailnetIP.String(), strconv.Itoa(TailnetStatisticsPort))
	url := fmt.Sprintf("http://%s%s", host, path)

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, xerrors.Errorf("new statistics server request to %q: %w", url, err)
	}

	return c.statisticsClient().Do(req)
}

type ListeningPortsResponse struct {
	// If there are no ports in the list, nothing should be displayed in the UI.
	// There must not be a "no ports available" message or anything similar, as
	// there will always be no ports displayed on platforms where our port
	// detection logic is unsupported.
	Ports []ListeningPort `json:"ports"`
}

type ListeningPortNetwork string

const (
	ListeningPortNetworkTCP ListeningPortNetwork = "tcp"
)

type ListeningPort struct {
	ProcessName string               `json:"process_name"` // may be empty
	Network     ListeningPortNetwork `json:"network"`      // only "tcp" at the moment
	Port        uint16               `json:"port"`
}

func (c *AgentConn) ListeningPorts(ctx context.Context) (ListeningPortsResponse, error) {
	res, err := c.doStatisticsRequest(ctx, http.MethodGet, "/api/v0/listening-ports", nil)
	if err != nil {
		return ListeningPortsResponse{}, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ListeningPortsResponse{}, readBodyAsError(res)
	}

	var resp ListeningPortsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
