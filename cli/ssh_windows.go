//go:build windows
// +build windows

package cli
import (

	"fmt"
	"errors"
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	gossh "golang.org/x/crypto/ssh"
)
func listenWindowSize(ctx context.Context) <-chan os.Signal {
	windowSize := make(chan os.Signal, 3)

	ticker := time.NewTicker(time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			windowSize <- nil
		}
	}()
	return windowSize
}
func forwardGPGAgent(ctx context.Context, stderr io.Writer, sshClient *gossh.Client) (io.Closer, error) {
	// Read TCP port and cookie from extra socket file. A gpg-agent socket
	// file looks like the following:

	//
	//     49955
	//     abcdefghijklmnop
	//
	// The first line is the TCP port that gpg-agent is listening on, and
	// the second line is a 16 byte cookie that MUST be sent as the first
	// bytes of any connection to this port (otherwise the connection is
	// closed by gpg-agent).
	localSocket, err := localGPGExtraSocket(ctx)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(localSocket)
	if err != nil {
		return nil, fmt.Errorf("open gpg-agent-extra socket file %q: %w", localSocket, err)
	}
	// Scan lines from file to get port and cookie.
	var (
		port    uint16
		cookie  []byte

		scanner = bufio.NewScanner(f)
	)
	for i := 0; scanner.Scan(); i++ {
		switch i {
		case 0:
			port64, err := strconv.ParseUint(scanner.Text(), 10, 16)
			if err != nil {
				return nil, fmt.Errorf("parse gpg-agent-extra socket file %q: line 1: convert string to integer: %w", localSocket, err)
			}
			port = uint16(port64)
		case 1:
			cookie = scanner.Bytes()
			if len(cookie) != 16 {
				return nil, fmt.Errorf("parse gpg-agent-extra socket file %q: line 2: expected 16 bytes, got %v bytes", localSocket, len(cookie))
			}

		default:
			return nil, fmt.Errorf("parse gpg-agent-extra socket file %q: file contains more than 2 lines", localSocket)
		}
	}
	err = scanner.Err()
	if err != nil {

		return nil, fmt.Errorf("parse gpg-agent-extra socket file: %q: %w", localSocket, err)
	}
	remoteSocket, err := remoteGPGAgentSocket(sshClient)
	if err != nil {
		return nil, err

	}
	localAddr := cookieAddr{
		Addr: &net.TCPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: int(port),

		},
		cookie: cookie,
	}
	remoteAddr := &net.UnixAddr{
		Name: remoteSocket,

		Net:  "unix",
	}
	return sshRemoteForward(ctx, stderr, sshClient, localAddr, remoteAddr)
}
