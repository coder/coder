package main

import (
	"context"
	"log"
	"os"
	"testing"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("Could not put terminal in raw mode: %v\n", err)
	}
	defer terminal.Restore(0, oldState)

	// if true {
	// 	pty, err := pty.Run(exec.Command("powershell.exe"))
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	go func() {
	// 		_, _ = io.Copy(pty.Input(), os.Stdin)

	// 	}()
	// 	_, _ = io.Copy(os.Stdout, pty.Output())
	// 	return
	// }

	t := &testing.T{}
	ctx := context.Background()
	client, server := provisionersdk.TransportPipe()
	defer client.Close()
	defer server.Close()
	closer := agent.Server(func(ctx context.Context) (*peerbroker.Listener, error) {
		return peerbroker.Listen(server, &peer.ConnOptions{
			Logger: slogtest.Make(t, nil),
		})
	}, &agent.Options{
		Logger: slogtest.Make(t, nil),
	})
	defer closer.Close()
	api := proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(client))
	stream, err := api.NegotiateConnection(ctx)
	require.NoError(t, err)
	conn, err := peerbroker.Dial(stream, []webrtc.ICEServer{}, &peer.ConnOptions{
		Logger: slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer conn.Close()
	channel, err := conn.Dial(ctx, "example", &peer.ChannelOptions{
		Protocol: "ssh",
	})
	require.NoError(t, err)
	sshConn, channels, requests, err := ssh.NewClientConn(channel.NetConn(), "localhost:22", &ssh.ClientConfig{
		User: "kyle",
		Config: ssh.Config{
			Ciphers: []string{"arcfour"},
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	require.NoError(t, err)
	sshClient := ssh.NewClient(sshConn, channels, requests)
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	err = session.RequestPty("", 128, 128, ssh.TerminalModes{})
	require.NoError(t, err)
	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	err = session.Run("C:\\WINDOWS\\System32\\WindowsPowerShell\\v1.0\\powershell.exe")
	require.NoError(t, err)
}
