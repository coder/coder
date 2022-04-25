package agent_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/crypto/ssh"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

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
		session := setupSSHSession(t, nil)

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
		session := setupSSHSession(t, nil)
		command := "sh -c 'echo $GIT_SSH_COMMAND'"
		if runtime.GOOS == "windows" {
			command = "cmd.exe /c echo %GIT_SSH_COMMAND%"
		}
		output, err := session.Output(command)
		require.NoError(t, err)
		require.True(t, strings.HasSuffix(strings.TrimSpace(string(output)), "gitssh --"))
	})

	t.Run("SessionTTY", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			// This might be our implementation, or ConPTY itself.
			// It's difficult to find extensive tests for it, so
			// it seems like it could be either.
			t.Skip("ConPTY appears to be inconsistent on Windows.")
		}
		session := setupSSHSession(t, nil)
		command := "bash"
		if runtime.GOOS == "windows" {
			command = "cmd.exe"
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
		caret := "$"
		if runtime.GOOS == "windows" {
			caret = ">"
		}
		ptty.ExpectMatch(caret)
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
		defer local.Close()
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

	t.Run("SFTP", func(t *testing.T) {
		t.Parallel()
		sshClient, err := setupAgent(t, nil).SSHClient()
		require.NoError(t, err)
		client, err := sftp.NewClient(sshClient)
		require.NoError(t, err)
		tempFile := filepath.Join(t.TempDir(), "sftp")
		file, err := client.Create(tempFile)
		require.NoError(t, err)
		err = file.Close()
		require.NoError(t, err)
		_, err = os.Stat(tempFile)
		require.NoError(t, err)
	})

	t.Run("EnvironmentVariables", func(t *testing.T) {
		t.Parallel()
		key := "EXAMPLE"
		value := "value"
		session := setupSSHSession(t, &agent.Options{
			EnvironmentVariables: map[string]string{
				key: value,
			},
		})
		command := "sh -c 'echo $" + key + "'"
		if runtime.GOOS == "windows" {
			command = "cmd.exe /c echo %" + key + "%"
		}
		output, err := session.Output(command)
		require.NoError(t, err)
		require.Equal(t, value, strings.TrimSpace(string(output)))
	})

	t.Run("StartupScript", func(t *testing.T) {
		t.Parallel()
		tempPath := filepath.Join(os.TempDir(), "content.txt")
		content := "somethingnice"
		setupAgent(t, &agent.Options{
			StartupScript: "echo " + content + " > " + tempPath,
		})
		var gotContent string
		require.Eventually(t, func() bool {
			content, err := os.ReadFile(tempPath)
			if err != nil {
				return false
			}
			if len(content) == 0 {
				return false
			}
			if runtime.GOOS == "windows" {
				// Windows uses UTF16! 🪟🪟🪟
				content, _, err = transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder(), content)
				require.NoError(t, err)
			}
			gotContent = string(content)
			return true
		}, 15*time.Second, 100*time.Millisecond)
		require.Equal(t, content, strings.TrimSpace(gotContent))
	})
}

func setupSSHCommand(t *testing.T, beforeArgs []string, afterArgs []string) *exec.Cmd {
	agentConn := setupAgent(t, nil)
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

func setupSSHSession(t *testing.T, options *agent.Options) *ssh.Session {
	sshClient, err := setupAgent(t, options).SSHClient()
	require.NoError(t, err)
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	return session
}

func setupAgent(t *testing.T, options *agent.Options) *agent.Conn {
	if options == nil {
		options = &agent.Options{}
	}
	client, server := provisionersdk.TransportPipe()
	closer := agent.New(func(ctx context.Context, logger slog.Logger) (*agent.Options, *peerbroker.Listener, error) {
		listener, err := peerbroker.Listen(server, nil)
		return options, listener, err
	}, slogtest.Make(t, nil).Leveled(slog.LevelDebug))
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
