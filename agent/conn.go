package agent

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
	"tailscale.com/tailcfg"

	"github.com/coder/coder/tailnet"
)

// ReconnectingPTYRequest is sent from the client to the server
// to pipe data to a PTY.
type ReconnectingPTYRequest struct {
	Data   string `json:"data"`
	Height uint16 `json:"height"`
	Width  uint16 `json:"width"`
}

type Conn struct {
	*tailnet.Conn
	CloseFunc func()
}

func (c *Conn) Ping() (time.Duration, error) {
	errCh := make(chan error, 1)
	durCh := make(chan time.Duration, 1)
	c.Conn.Ping(tailnetIP, tailcfg.PingICMP, func(pr *ipnstate.PingResult) {
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

func (c *Conn) CloseWithError(_ error) error {
	return c.Close()
}

func (c *Conn) Close() error {
	if c.CloseFunc != nil {
		c.CloseFunc()
	}
	return c.Conn.Close()
}

type reconnectingPTYInit struct {
	ID      string
	Height  uint16
	Width   uint16
	Command string
}

func (c *Conn) ReconnectingPTY(id string, height, width uint16, command string) (net.Conn, error) {
	conn, err := c.DialContextTCP(context.Background(), netip.AddrPortFrom(tailnetIP, uint16(tailnetReconnectingPTYPort)))
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(reconnectingPTYInit{
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

func (c *Conn) SSH() (net.Conn, error) {
	return c.DialContextTCP(context.Background(), netip.AddrPortFrom(tailnetIP, uint16(tailnetSSHPort)))
}

// SSHClient calls SSH to create a client that uses a weak cipher
// for high throughput.
func (c *Conn) SSHClient() (*ssh.Client, error) {
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

func (c *Conn) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	if network == "unix" {
		return nil, xerrors.New("network must be tcp or udp")
	}
	_, rawPort, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(rawPort)
	ipp := netip.AddrPortFrom(tailnetIP, uint16(port))
	if network == "udp" {
		return c.Conn.DialContextUDP(ctx, ipp)
	}
	return c.Conn.DialContextTCP(ctx, ipp)
}
