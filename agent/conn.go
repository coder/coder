package agent

import (
	"context"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"github.com/coder/coder/peer"
)

// Conn wraps a peer connection with helper functions to
// communicate with the agent.
type Conn struct {
	*peer.Conn
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
		Config: ssh.Config{
			Ciphers: []string{"arcfour"},
		},
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
