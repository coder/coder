package agent

import (
	"context"
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker/proto"
)

// ReconnectingPTYRequest is sent from the client to the server
// to pipe data to a PTY.
type ReconnectingPTYRequest struct {
	Data   string `json:"data"`
	Height uint16 `json:"height"`
	Width  uint16 `json:"width"`
}

// Conn wraps a peer connection with helper functions to
// communicate with the agent.
type Conn struct {
	// Negotiator is responsible for exchanging messages.
	Negotiator proto.DRPCPeerBrokerClient

	*peer.Conn
}

// ReconnectingPTY returns a connection serving a TTY that can
// be reconnected to via ID.
func (c *Conn) ReconnectingPTY(id string, height, width uint16) (net.Conn, error) {
	channel, err := c.Dial(context.Background(), fmt.Sprintf("%s:%d:%d", id, height, width), &peer.ChannelOptions{
		Protocol: "reconnecting-pty",
	})
	if err != nil {
		return nil, xerrors.Errorf("pty: %w", err)
	}
	return channel.NetConn(), nil
}

// SSH dials the built-in SSH server.
func (c *Conn) SSH() (net.Conn, error) {
	channel, err := c.Dial(context.Background(), "ssh", &peer.ChannelOptions{
		Protocol: "ssh",
	})
	if err != nil {
		return nil, xerrors.Errorf("dial: %w", err)
	}
	return channel.NetConn(), nil
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

func (c *Conn) Close() error {
	_ = c.Negotiator.DRPCConn().Close()
	return c.Conn.Close()
}
