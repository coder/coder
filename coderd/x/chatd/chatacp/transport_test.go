package chatacp_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"

	"github.com/coder/coder/v2/coderd/x/chatd/chatacp"
	"github.com/coder/coder/v2/testutil"
)

// TestSSHTransportStartCancel verifies that Start returns as soon as
// the turn is canceled while session setup waits on an unresponsive
// agent, and that a session which completes afterwards is closed
// rather than leaked.
func TestSSHTransportStartCancel(t *testing.T) {
	t.Parallel()
	testCtx := testutil.Context(t, testutil.WaitShort)

	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(hostKey)
	require.NoError(t, err)
	serverConfig := &gossh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	// The server completes the handshake but holds the session channel
	// open request until released, then serves the exec request and
	// reports when the client closes the session.
	opened := make(chan struct{}, 1)
	release := make(chan struct{})
	closed := make(chan struct{}, 1)
	serverDone := make(chan struct{}, 1)
	go func() {
		defer func() { serverDone <- struct{}{} }()
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		serverConn, chans, reqs, err := gossh.NewServerConn(conn, serverConfig)
		if err != nil {
			_ = conn.Close()
			return
		}
		defer serverConn.Close()
		go gossh.DiscardRequests(reqs)
		newChannel, ok := <-chans
		if !ok {
			return
		}
		opened <- struct{}{}
		<-release
		channel, requests, err := newChannel.Accept()
		if err != nil {
			return
		}
		defer channel.Close()
		for req := range requests {
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		}
		closed <- struct{}{}
	}()

	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec // Test server with a throwaway host key.
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	transport := &chatacp.SSHTransport{Client: client, Command: "adapter"}
	ctx, cancel := context.WithCancel(testCtx)
	defer cancel()
	type result struct {
		process chatacp.Process
		err     error
	}
	started := make(chan result, 1)
	go func() {
		process, err := transport.Start(ctx)
		started <- result{process: process, err: err}
	}()

	testutil.RequireReceive(testCtx, t, opened)
	cancel()
	res := testutil.RequireReceive(testCtx, t, started)
	require.ErrorIs(t, res.err, context.Canceled)
	require.Nil(t, res.process)

	close(release)
	testutil.RequireReceive(testCtx, t, closed)
	// The server may close the connection first, so the client close
	// result carries no signal here.
	_ = client.Close()
	testutil.RequireReceive(testCtx, t, serverDone)
}
