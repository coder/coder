package peerwg

import (
	"context"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
	"inet.af/netaddr"
)

func (n *Network) SSH(ctx context.Context, ip netaddr.IP) (net.Conn, error) {
	netConn, err := n.Netstack.DialContextTCP(ctx, netaddr.IPPortFrom(ip, 12212))
	if err != nil {
		return nil, xerrors.Errorf("dial agent ssh: %w", err)
	}

	return netConn, nil
}

func (n *Network) SSHClient(ctx context.Context, ip netaddr.IP) (*ssh.Client, error) {
	netConn, err := n.SSH(ctx, ip)
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
		return nil, xerrors.Errorf("new ssh client conn: %w", err)
	}

	return ssh.NewClient(sshConn, channels, requests), nil
}
