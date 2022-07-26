package agent_test

import (
	"bufio"
	"context"
	"encoding/json"
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

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/google/uuid"
	"github.com/pion/udp"
	"github.com/pion/webrtc/v3"
	"github.com/pkg/sftp"
	"github.com/stretchr/testify/assert"
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
		session := setupSSHSession(t, agent.Metadata{})

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
		session := setupSSHSession(t, agent.Metadata{})
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
		session := setupSSHSession(t, agent.Metadata{})
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
			defer close(done)
			conn, err := local.Accept()
			if !assert.NoError(t, err) {
				return
			}
			_ = conn.Close()
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
		sshClient, err := setupAgent(t, agent.Metadata{}, 0).SSHClient()
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

	t.Run("SCP", func(t *testing.T) {
		t.Parallel()
		sshClient, err := setupAgent(t, agent.Metadata{}, 0).SSHClient()
		require.NoError(t, err)
		scpClient, err := scp.NewClientBySSH(sshClient)
		require.NoError(t, err)
		tempFile := filepath.Join(t.TempDir(), "scp")
		content := "hello world"
		err = scpClient.CopyFile(context.Background(), strings.NewReader(content), tempFile, "0755")
		require.NoError(t, err)
		_, err = os.Stat(tempFile)
		require.NoError(t, err)
	})

	t.Run("EnvironmentVariables", func(t *testing.T) {
		t.Parallel()
		key := "EXAMPLE"
		value := "value"
		session := setupSSHSession(t, agent.Metadata{
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

	t.Run("EnvironmentVariableExpansion", func(t *testing.T) {
		t.Parallel()
		key := "EXAMPLE"
		session := setupSSHSession(t, agent.Metadata{
			EnvironmentVariables: map[string]string{
				key: "$SOMETHINGNOTSET",
			},
		})
		command := "sh -c 'echo $" + key + "'"
		if runtime.GOOS == "windows" {
			command = "cmd.exe /c echo %" + key + "%"
		}
		output, err := session.Output(command)
		require.NoError(t, err)
		expect := ""
		if runtime.GOOS == "windows" {
			expect = "%EXAMPLE%"
		}
		// Output should be empty, because the variable is not set!
		require.Equal(t, expect, strings.TrimSpace(string(output)))
	})

	t.Run("StartupScript", func(t *testing.T) {
		t.Parallel()
		tempPath := filepath.Join(t.TempDir(), "content.txt")
		content := "somethingnice"
		setupAgent(t, agent.Metadata{
			StartupScript: fmt.Sprintf("echo %s > %s", content, tempPath),
		}, 0)

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
				if !assert.NoError(t, err) {
					return false
				}
			}
			gotContent = string(content)
			return true
		}, 15*time.Second, 100*time.Millisecond)
		require.Equal(t, content, strings.TrimSpace(gotContent))
	})

	t.Run("ReconnectingPTY", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			// This might be our implementation, or ConPTY itself.
			// It's difficult to find extensive tests for it, so
			// it seems like it could be either.
			t.Skip("ConPTY appears to be inconsistent on Windows.")
		}

		conn := setupAgent(t, agent.Metadata{}, 0)
		id := uuid.NewString()
		netConn, err := conn.ReconnectingPTY(id, 100, 100, "/bin/bash")
		require.NoError(t, err)
		bufRead := bufio.NewReader(netConn)

		// Brief pause to reduce the likelihood that we send keystrokes while
		// the shell is simultaneously sending a prompt.
		time.Sleep(100 * time.Millisecond)

		data, err := json.Marshal(agent.ReconnectingPTYRequest{
			Data: "echo test\r\n",
		})
		require.NoError(t, err)
		_, err = netConn.Write(data)
		require.NoError(t, err)

		expectLine := func(matcher func(string) bool) {
			for {
				line, err := bufRead.ReadString('\n')
				require.NoError(t, err)
				if matcher(line) {
					break
				}
			}
		}

		matchEchoCommand := func(line string) bool {
			return strings.Contains(line, "echo test")
		}
		matchEchoOutput := func(line string) bool {
			return strings.Contains(line, "test") && !strings.Contains(line, "echo")
		}

		// Once for typing the command...
		expectLine(matchEchoCommand)
		// And another time for the actual output.
		expectLine(matchEchoOutput)

		_ = netConn.Close()
		netConn, err = conn.ReconnectingPTY(id, 100, 100, "/bin/bash")
		require.NoError(t, err)
		bufRead = bufio.NewReader(netConn)

		// Same output again!
		expectLine(matchEchoCommand)
		expectLine(matchEchoOutput)
	})

	t.Run("Dial", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name  string
			setup func(t *testing.T) net.Listener
		}{
			{
				name: "TCP",
				setup: func(t *testing.T) net.Listener {
					l, err := net.Listen("tcp", "127.0.0.1:0")
					require.NoError(t, err, "create TCP listener")
					return l
				},
			},
			{
				name: "UDP",
				setup: func(t *testing.T) net.Listener {
					addr := net.UDPAddr{
						IP:   net.ParseIP("127.0.0.1"),
						Port: 0,
					}
					l, err := udp.Listen("udp", &addr)
					require.NoError(t, err, "create UDP listener")
					return l
				},
			},
			{
				name: "Unix",
				setup: func(t *testing.T) net.Listener {
					if runtime.GOOS == "windows" {
						t.Skip("Unix socket forwarding isn't supported on Windows")
					}

					tmpDir := t.TempDir()
					l, err := net.Listen("unix", filepath.Join(tmpDir, "test.sock"))
					require.NoError(t, err, "create UDP listener")
					return l
				},
			},
		}

		for _, c := range cases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()

				// Setup listener
				l := c.setup(t)
				defer l.Close()
				go func() {
					for {
						c, err := l.Accept()
						if err != nil {
							return
						}

						go testAccept(t, c)
					}
				}()

				// Dial the listener over WebRTC twice and test out of order
				conn := setupAgent(t, agent.Metadata{}, 0)
				conn1, err := conn.DialContext(context.Background(), l.Addr().Network(), l.Addr().String())
				require.NoError(t, err)
				defer conn1.Close()
				conn2, err := conn.DialContext(context.Background(), l.Addr().Network(), l.Addr().String())
				require.NoError(t, err)
				defer conn2.Close()
				testDial(t, conn2)
				testDial(t, conn1)
			})
		}
	})

	t.Run("DialError", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			// This test uses Unix listeners so we can very easily ensure that
			// no other tests decide to listen on the same random port we
			// picked.
			t.Skip("this test is unsupported on Windows")
			return
		}

		tmpDir, err := os.MkdirTemp("", "coderd_agent_test_")
		require.NoError(t, err, "create temp dir")
		t.Cleanup(func() {
			_ = os.RemoveAll(tmpDir)
		})

		// Try to dial the non-existent Unix socket over WebRTC
		conn := setupAgent(t, agent.Metadata{}, 0)
		netConn, err := conn.DialContext(context.Background(), "unix", filepath.Join(tmpDir, "test.sock"))
		require.Error(t, err)
		require.ErrorContains(t, err, "remote dial error")
		require.ErrorContains(t, err, "no such file")
		require.Nil(t, netConn)
	})
}

func setupSSHCommand(t *testing.T, beforeArgs []string, afterArgs []string) *exec.Cmd {
	agentConn := setupAgent(t, agent.Metadata{}, 0)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			ssh, err := agentConn.SSH()
			if !assert.NoError(t, err) {
				_ = conn.Close()
				return
			}
			go agent.Bicopy(context.Background(), conn, ssh)
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

func setupSSHSession(t *testing.T, options agent.Metadata) *ssh.Session {
	sshClient, err := setupAgent(t, options, 0).SSHClient()
	require.NoError(t, err)
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	return session
}

func setupAgent(t *testing.T, metadata agent.Metadata, ptyTimeout time.Duration) *agent.Conn {
	client, server := provisionersdk.TransportPipe()
	closer := agent.New(func(ctx context.Context, logger slog.Logger) (agent.Metadata, *peerbroker.Listener, error) {
		listener, err := peerbroker.Listen(server, nil)
		return metadata, listener, err
	}, &agent.Options{
		Logger:                 slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		ReconnectingPTYTimeout: ptyTimeout,
	})
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		_ = closer.Close()
	})
	api := proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(client))
	stream, err := api.NegotiateConnection(context.Background())
	assert.NoError(t, err)
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

var dialTestPayload = []byte("dean-was-here123")

func testDial(t *testing.T, c net.Conn) {
	t.Helper()

	assertWritePayload(t, c, dialTestPayload)
	assertReadPayload(t, c, dialTestPayload)
}

func testAccept(t *testing.T, c net.Conn) {
	t.Helper()
	defer c.Close()

	assertReadPayload(t, c, dialTestPayload)
	assertWritePayload(t, c, dialTestPayload)
}

func assertReadPayload(t *testing.T, r io.Reader, payload []byte) {
	b := make([]byte, len(payload)+16)
	n, err := r.Read(b)
	assert.NoError(t, err, "read payload")
	assert.Equal(t, len(payload), n, "read payload length does not match")
	assert.Equal(t, payload, b[:n])
}

func assertWritePayload(t *testing.T, w io.Writer, payload []byte) {
	n, err := w.Write(payload)
	assert.NoError(t, err, "write payload")
	assert.Equal(t, len(payload), n, "payload length does not match")
}
