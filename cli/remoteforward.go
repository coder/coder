package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"github.com/coder/coder/agent/agentssh"
)

// cookieAddr is a special net.Addr accepted by sshRemoteForward() which includes a
// cookie which is written to the connection before forwarding.
type cookieAddr struct {
	net.Addr
	cookie []byte
}

// Format:
// remote_port:local_address:local_port
var remoteForwardRegex = regexp.MustCompile(`^(\d+):(.+):(\d+)$`)

func validateRemoteForward(flag string) bool {
	return remoteForwardRegex.MatchString(flag)
}

func parseRemoteForward(flag string) (net.Addr, net.Addr, error) {
	matches := remoteForwardRegex.FindStringSubmatch(flag)

	remotePort, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, nil, xerrors.Errorf("remote port is invalid: %w", err)
	}
	localAddress, err := net.ResolveIPAddr("ip", matches[2])
	if err != nil {
		return nil, nil, xerrors.Errorf("local address is invalid: %w", err)
	}
	localPort, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil, nil, xerrors.Errorf("local port is invalid: %w", err)
	}

	localAddr := &net.TCPAddr{
		IP:   localAddress.IP,
		Port: localPort,
	}

	remoteAddr := &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: remotePort,
	}
	return localAddr, remoteAddr, nil
}

// sshRemoteForward starts forwarding connections from a remote listener to a
// local address via SSH in a goroutine.
//
// Accepts a `cookieAddr` as the local address.
func sshRemoteForward(ctx context.Context, stderr io.Writer, sshClient *gossh.Client, localAddr, remoteAddr net.Addr) (io.Closer, error) {
	listener, err := sshClient.Listen(remoteAddr.Network(), remoteAddr.String())
	if err != nil {
		return nil, xerrors.Errorf("listen on remote SSH address %s: %w", remoteAddr.String(), err)
	}

	go func() {
		for {
			remoteConn, err := listener.Accept()
			if err != nil {
				if ctx.Err() == nil {
					_, _ = fmt.Fprintf(stderr, "Accept SSH listener connection: %+v\n", err)
				}
				return
			}

			go func() {
				defer remoteConn.Close()

				localConn, err := net.Dial(localAddr.Network(), localAddr.String())
				if err != nil {
					_, _ = fmt.Fprintf(stderr, "Dial local address %s: %+v\n", localAddr.String(), err)
					return
				}
				defer localConn.Close()

				if c, ok := localAddr.(cookieAddr); ok {
					_, err = localConn.Write(c.cookie)
					if err != nil {
						_, _ = fmt.Fprintf(stderr, "Write cookie to local connection: %+v\n", err)
						return
					}
				}

				agentssh.Bicopy(ctx, localConn, remoteConn)
			}()
		}
	}()

	return listener, nil
}
