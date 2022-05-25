package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

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
	channel, err := c.CreateChannel(context.Background(), fmt.Sprintf("%s:%d:%d", id, height, width), &peer.ChannelOptions{
		Protocol: ProtocolReconnectingPTY,
	})
	if err != nil {
		return nil, xerrors.Errorf("pty: %w", err)
	}
	return channel.NetConn(), nil
}

// SSH dials the built-in SSH server.
func (c *Conn) SSH() (net.Conn, error) {
	channel, err := c.CreateChannel(context.Background(), "ssh", &peer.ChannelOptions{
		Protocol: ProtocolSSH,
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

// DialContext dials an arbitrary protocol+address from inside the workspace and
// proxies it through the provided net.Conn.
func (c *Conn) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	u := &url.URL{
		Scheme: network,
	}
	if strings.HasPrefix(network, "unix") {
		u.Path = addr
	} else {
		u.Host = addr
	}

	channel, err := c.CreateChannel(ctx, u.String(), &peer.ChannelOptions{
		Protocol:  ProtocolDial,
		Unordered: strings.HasPrefix(network, "udp"),
	})
	if err != nil {
		return nil, xerrors.Errorf("create datachannel: %w", err)
	}

	// The first message written from the other side is a JSON payload
	// containing the dial error.
	dec := json.NewDecoder(channel)
	var res dialResponse
	err = dec.Decode(&res)
	if err != nil {
		return nil, xerrors.Errorf("failed to decode initial packet: %w", err)
	}
	if res.Error != "" {
		_ = channel.Close()
		return nil, xerrors.Errorf("remote dial error: %v", res.Error)
	}

	return channel.NetConn(), nil
}

func (c *Conn) Close() error {
	_ = c.Negotiator.DRPCConn().Close()
	return c.Conn.Close()
}
