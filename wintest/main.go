package main

import (
	"context"
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
	"golang.org/x/sys/windows"
)

func main() {
	state, err := MakeOutputRaw(os.Stdout.Fd())
	if err != nil {
		panic(err)
	}
	defer Restore(os.Stdout.Fd(), state)

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
	err = session.RequestPty("xterm-256color", 128, 128, ssh.TerminalModes{
		ssh.ECHO: 1,
	})
	require.NoError(t, err)
	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	err = session.Run("bash")
	require.NoError(t, err)
}

// State differs per-platform.
type State struct {
	mode uint32
}

// makeRaw sets the terminal in raw mode and returns the previous state so it can be restored.
func makeRaw(handle windows.Handle, input bool) (uint32, error) {
	var prevState uint32
	if err := windows.GetConsoleMode(handle, &prevState); err != nil {
		return 0, err
	}

	var raw uint32
	if input {
		raw = prevState &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_OUTPUT)
		raw |= windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	} else {
		raw = prevState | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	}

	if err := windows.SetConsoleMode(handle, raw); err != nil {
		return 0, err
	}
	return prevState, nil
}

// MakeOutputRaw sets an output terminal to raw and enables VT100 processing.
func MakeOutputRaw(handle uintptr) (*State, error) {
	prevState, err := makeRaw(windows.Handle(handle), false)
	if err != nil {
		return nil, err
	}

	return &State{mode: prevState}, nil
}

// Restore terminal back to original state.
func Restore(handle uintptr, state *State) error {
	return windows.SetConsoleMode(windows.Handle(handle), state.mode)
}
