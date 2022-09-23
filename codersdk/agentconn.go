package codersdk

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
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
	c.Conn.Ping(TailnetIP, tailcfg.PingICMP, func(pr *ipnstate.PingResult) {
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
