package agent_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/crypto/ssh"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/pty/ptytest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestAgent(t *testing.T) {
	t.Parallel()
	t.Run("SessionExec", func(t *testing.T) {
		t.Parallel()
		session := setupSSHSession(t)

		command := "echo test"
		if runtime.GOOS == "windows" {
			command = "cmd.exe /c echo test"
		}
		output, err := session.Output(command)
		require.NoError(t, err)
		require.Equal(t, "test", strings.TrimSpace(string(output)))
	})

	t.Run("GitSSH", func(t *testing.T) {
		t.Parallel()
		session := setupSSHSession(t)
		command := "sh -c 'echo $GIT_SSH_COMMAND'"
		if runtime.GOOS == "windows" {
			command = "cmd.exe /c echo %GIT_SSH_COMMAND%"
		}
		output, err := session.Output(command)
		require.NoError(t, err)
		require.Contains(t, string(output), "gitssh --")
	})

	t.Run("SessionTTY", func(t *testing.T) {
		t.Parallel()
		session := setupSSHSession(t)
		prompt := "$"
		command := "bash"
		if runtime.GOOS == "windows" {
			command = "cmd.exe"
			prompt = ">"
		}
		err := session.RequestPty("xterm", 128, 128, ssh.TerminalModes{})
		require.NoError(t, err)
		ptty := ptytest.New(t)
		require.NoError(t, err)
		session.Stdout = ptty.Output()
		session.Stderr = ptty.Output()
		session.Stdin = ptty.Input()
		err = session.Start(command)
		require.NoError(t, err)
		ptty.ExpectMatch(prompt)
		ptty.WriteLine("echo test")
		ptty.ExpectMatch("test")
		ptty.WriteLine("exit")
		err = session.Wait()
		require.NoError(t, err)
	})

	t.Run("LocalForwarding", func(t *testing.T) {
		t.Parallel()
		random, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		_ = random.Close()
		tcpAddr, valid := random.Addr().(*net.TCPAddr)
		require.True(t, valid)
		randomPort := tcpAddr.Port

		local, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		tcpAddr, valid = local.Addr().(*net.TCPAddr)
		require.True(t, valid)
		localPort := tcpAddr.Port
		done := make(chan struct{})
		go func() {
			conn, err := local.Accept()
			require.NoError(t, err)
			_ = conn.Close()
			close(done)
		}()

		err = setupSSHCommand(t, []string{"-L", fmt.Sprintf("%d:127.0.0.1:%d", randomPort, localPort)}, []string{"echo", "test"}).Start()
		require.NoError(t, err)

		conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(localPort))
		require.NoError(t, err)
		conn.Close()
		<-done
	})
}

func setupSSHCommand(t *testing.T, beforeArgs []string, afterArgs []string) *exec.Cmd {
	agentConn := setupAgent(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			ssh, err := agentConn.SSH()
			require.NoError(t, err)
			go io.Copy(conn, ssh)
			go io.Copy(ssh, conn)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
	})
	tcpAddr, valid := listener.Addr().(*net.TCPAddr)
	require.True(t, valid)
	args := append(beforeArgs,
		"-o", "HostName "+tcpAddr.IP.String(),
		"-o", "Port "+strconv.Itoa(tcpAddr.Port),
		"-o", "StrictHostKeyChecking=no", "host")
	args = append(args, afterArgs...)
	return exec.Command("ssh", args...)
}

func setupSSHSession(t *testing.T) *ssh.Session {
	sshClient, err := setupAgent(t).SSHClient()
	require.NoError(t, err)
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	return session
}

func setupAgent(t *testing.T) *agent.Conn {
	client, server := provisionersdk.TransportPipe()
	closer := agent.New(func(ctx context.Context, opts *peer.ConnOptions) (*peerbroker.Listener, error) {
		return peerbroker.Listen(server, nil, opts)
	}, &peer.ConnOptions{
		Logger: slogtest.Make(t, nil).Leveled(slog.LevelDebug),
	})
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		_ = closer.Close()
	})
	api := proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(client))
	stream, err := api.NegotiateConnection(context.Background())
	require.NoError(t, err)
	conn, err := peerbroker.Dial(stream, []webrtc.ICEServer{}, &peer.ConnOptions{
		Logger: slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	return &agent.Conn{
		Negotiator: api,
		Conn:       conn,
	}
}
