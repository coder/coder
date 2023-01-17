//go:build !windows
// +build !windows

package cli

import (
	"context"
	"io"
	"net"
	"os"
	"os/signal"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"
)

func listenWindowSize(ctx context.Context) <-chan os.Signal {
	windowSize := make(chan os.Signal, 1)
	signal.Notify(windowSize, unix.SIGWINCH)
	go func() {
		<-ctx.Done()
		signal.Stop(windowSize)
	}()
	return windowSize
}

func forwardGPGAgent(ctx context.Context, stderr io.Writer, sshClient *gossh.Client) (io.Closer, error) {
	localSocket, err := localGPGExtraSocket(ctx)
	if err != nil {
		return nil, err
	}

	remoteSocket, err := remoteGPGAgentSocket(sshClient)
	if err != nil {
		return nil, err
	}

	localAddr := &net.UnixAddr{
		Name: localSocket,
		Net:  "unix",
	}
	remoteAddr := &net.UnixAddr{
		Name: remoteSocket,
		Net:  "unix",
	}

	return sshForwardRemote(ctx, stderr, sshClient, localAddr, remoteAddr)
}
