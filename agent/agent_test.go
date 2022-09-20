package agent_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/net/speedtest"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/google/uuid"
	"github.com/pion/udp"
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
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/tailnet/tailnettest"
	"github.com/coder/coder/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestAgent(t *testing.T) {
	t.Parallel()
	t.Run("Stats", func(t *testing.T) {
		t.Parallel()

		t.Run("SSH", func(t *testing.T) {
			t.Parallel()
			conn, stats := setupAgent(t, codersdk.WorkspaceAgentMetadata{}, 0)

			sshClient, err := conn.SSHClient()
			require.NoError(t, err)
			defer sshClient.Close()
			session, err := sshClient.NewSession()
			require.NoError(t, err)
			defer session.Close()

			assert.EqualValues(t, 1, (<-stats).NumConns)
			assert.Greater(t, (<-stats).RxBytes, int64(0))
			assert.Greater(t, (<-stats).TxBytes, int64(0))
		})

		t.Run("ReconnectingPTY", func(t *testing.T) {
			t.Parallel()

			conn, stats := setupAgent(t, codersdk.WorkspaceAgentMetadata{}, 0)

			ptyConn, err := conn.ReconnectingPTY(uuid.NewString(), 128, 128, "/bin/bash")
			require.NoError(t, err)
			defer ptyConn.Close()

			data, err := json.Marshal(codersdk.ReconnectingPTYRequest{
				Data: "echo test\r\n",
			})
			require.NoError(t, err)
			_, err = ptyConn.Write(data)
			require.NoError(t, err)

			var s *codersdk.AgentStats
			require.Eventuallyf(t, func() bool {
				var ok bool
				s, ok = (<-stats)
				return ok && s.NumConns > 0 && s.RxBytes > 0 && s.TxBytes > 0
			}, testutil.WaitLong, testutil.IntervalFast,
				"never saw stats: %+v", s,
			)
		})
	})

	t.Run("SessionExec", func(t *testing.T) {
		t.Parallel()
		session := setupSSHSession(t, codersdk.WorkspaceAgentMetadata{})

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
		session := setupSSHSession(t, codersdk.WorkspaceAgentMetadata{})
		command := "sh -c 'echo $GIT_SSH_COMMAND'"
		if runtime.GOOS == "windows" {
			command = "cmd.exe /c echo %GIT_SSH_COMMAND%"
		}
		output, err := session.Output(command)
		require.NoError(t, err)
		require.True(t, strings.HasSuffix(strings.TrimSpace(string(output)), "gitssh --"))
	})

	t.Run("SessionTTYShell", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			// This might be our implementation, or ConPTY itself.
			// It's difficult to find extensive tests for it, so
			// it seems like it could be either.
			t.Skip("ConPTY appears to be inconsistent on Windows.")
		}
		session := setupSSHSession(t, codersdk.WorkspaceAgentMetadata{})
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

	t.Run("SessionTTYExitCode", func(t *testing.T) {
		t.Parallel()
		session := setupSSHSession(t, codersdk.WorkspaceAgentMetadata{})
		command := "areallynotrealcommand"
		err := session.RequestPty("xterm", 128, 128, ssh.TerminalModes{})
		require.NoError(t, err)
		ptty := ptytest.New(t)
		require.NoError(t, err)
		session.Stdout = ptty.Output()
		session.Stderr = ptty.Output()
		session.Stdin = ptty.Input()
		err = session.Start(command)
		require.NoError(t, err)
		err = session.Wait()
		exitErr := &ssh.ExitError{}
		require.True(t, xerrors.As(err, &exitErr))
		if runtime.GOOS == "windows" {
			assert.Equal(t, 1, exitErr.ExitStatus())
		} else {
			assert.Equal(t, 127, exitErr.ExitStatus())
		}
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
		conn, _ := setupAgent(t, codersdk.WorkspaceAgentMetadata{}, 0)
		sshClient, err := conn.SSHClient()
		require.NoError(t, err)
		defer sshClient.Close()
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

		conn, _ := setupAgent(t, codersdk.WorkspaceAgentMetadata{}, 0)
		sshClient, err := conn.SSHClient()
		require.NoError(t, err)
		defer sshClient.Close()
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
		session := setupSSHSession(t, codersdk.WorkspaceAgentMetadata{
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
		session := setupSSHSession(t, codersdk.WorkspaceAgentMetadata{
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

	t.Run("Coder env vars", func(t *testing.T) {
		t.Parallel()

		for _, key := range []string{"CODER"} {
			key := key
			t.Run(key, func(t *testing.T) {
				t.Parallel()

				session := setupSSHSession(t, codersdk.WorkspaceAgentMetadata{})
				command := "sh -c 'echo $" + key + "'"
				if runtime.GOOS == "windows" {
					command = "cmd.exe /c echo %" + key + "%"
				}
				output, err := session.Output(command)
				require.NoError(t, err)
				require.NotEmpty(t, strings.TrimSpace(string(output)))
			})
		}
	})

	t.Run("SSH connection env vars", func(t *testing.T) {
		t.Parallel()

		// Note: the SSH_TTY environment variable should only be set for TTYs.
		// For some reason this test produces a TTY locally and a non-TTY in CI
		// so we don't test for the absence of SSH_TTY.
		for _, key := range []string{"SSH_CONNECTION", "SSH_CLIENT"} {
			key := key
			t.Run(key, func(t *testing.T) {
				t.Parallel()

				session := setupSSHSession(t, codersdk.WorkspaceAgentMetadata{})
				command := "sh -c 'echo $" + key + "'"
				if runtime.GOOS == "windows" {
					command = "cmd.exe /c echo %" + key + "%"
				}
				output, err := session.Output(command)
				require.NoError(t, err)
				require.NotEmpty(t, strings.TrimSpace(string(output)))
			})
		}
	})

	t.Run("StartupScript", func(t *testing.T) {
		t.Parallel()
		tempPath := filepath.Join(t.TempDir(), "content.txt")
		content := "somethingnice"
		setupAgent(t, codersdk.WorkspaceAgentMetadata{
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
		}, testutil.WaitMedium, testutil.IntervalMedium)
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

		conn, _ := setupAgent(t, codersdk.WorkspaceAgentMetadata{}, 0)
		id := uuid.NewString()
		netConn, err := conn.ReconnectingPTY(id, 100, 100, "/bin/bash")
		require.NoError(t, err)
		bufRead := bufio.NewReader(netConn)

		// Brief pause to reduce the likelihood that we send keystrokes while
		// the shell is simultaneously sending a prompt.
		time.Sleep(100 * time.Millisecond)

		data, err := json.Marshal(codersdk.ReconnectingPTYRequest{
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

				conn, _ := setupAgent(t, codersdk.WorkspaceAgentMetadata{}, 0)
				require.Eventually(t, func() bool {
					_, err := conn.Ping()
					return err == nil
				}, testutil.WaitMedium, testutil.IntervalFast)
				conn1, err := conn.DialContext(context.Background(), l.Addr().Network(), l.Addr().String())
				require.NoError(t, err)
				defer conn1.Close()
				conn2, err := conn.DialContext(context.Background(), l.Addr().Network(), l.Addr().String())
				require.NoError(t, err)
				defer conn2.Close()
				testDial(t, conn2)
				testDial(t, conn1)
				time.Sleep(150 * time.Millisecond)
			})
		}
	})

	t.Run("Tailnet", func(t *testing.T) {
		t.Parallel()
		derpMap := tailnettest.RunDERPAndSTUN(t)
		conn, _ := setupAgent(t, codersdk.WorkspaceAgentMetadata{
			DERPMap: derpMap,
		}, 0)
		defer conn.Close()
		require.Eventually(t, func() bool {
			_, err := conn.Ping()
			return err == nil
		}, testutil.WaitMedium, testutil.IntervalFast)
	})

	t.Run("Speedtest", func(t *testing.T) {
		t.Parallel()
		if testing.Short() {
			t.Skip("The minimum duration for a speedtest is hardcoded in Tailscale to 5s!")
		}
		derpMap := tailnettest.RunDERPAndSTUN(t)
		conn, _ := setupAgent(t, codersdk.WorkspaceAgentMetadata{
			DERPMap: derpMap,
		}, 0)
		defer conn.Close()
		res, err := conn.Speedtest(speedtest.Upload, 250*time.Millisecond)
		require.NoError(t, err)
		t.Logf("%.2f MBits/s", res[len(res)-1].MBitsPerSecond())
	})
}

func setupSSHCommand(t *testing.T, beforeArgs []string, afterArgs []string) *exec.Cmd {
	agentConn, _ := setupAgent(t, codersdk.WorkspaceAgentMetadata{}, 0)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	waitGroup := sync.WaitGroup{}
	go func() {
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			ssh, err := agentConn.SSH()
			if err != nil {
				_ = conn.Close()
				return
			}
			waitGroup.Add(1)
			go func() {
				agent.Bicopy(context.Background(), conn, ssh)
				waitGroup.Done()
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		waitGroup.Wait()
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

func setupSSHSession(t *testing.T, options codersdk.WorkspaceAgentMetadata) *ssh.Session {
	conn, _ := setupAgent(t, options, 0)
	sshClient, err := conn.SSHClient()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sshClient.Close()
	})
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	return session
}

type closeFunc func() error

func (c closeFunc) Close() error {
	return c()
}

func setupAgent(t *testing.T, metadata codersdk.WorkspaceAgentMetadata, ptyTimeout time.Duration) (
	*codersdk.AgentConn,
	<-chan *codersdk.AgentStats,
) {
	if metadata.DERPMap == nil {
		metadata.DERPMap = tailnettest.RunDERPAndSTUN(t)
	}
	coordinator := tailnet.NewCoordinator()
	agentID := uuid.New()
	statsCh := make(chan *codersdk.AgentStats)
	closer := agent.New(agent.Options{
		FetchMetadata: func(ctx context.Context) (codersdk.WorkspaceAgentMetadata, error) {
			return metadata, nil
		},
		CoordinatorDialer: func(ctx context.Context) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			closed := make(chan struct{})
			t.Cleanup(func() {
				_ = serverConn.Close()
				_ = clientConn.Close()
				<-closed
			})
			go func() {
				_ = coordinator.ServeAgent(serverConn, agentID)
				close(closed)
			}()
			return clientConn, nil
		},
		Logger:                 slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		ReconnectingPTYTimeout: ptyTimeout,
		StatsReporter: func(ctx context.Context, log slog.Logger, statsFn func() *codersdk.AgentStats) (io.Closer, error) {
			doneCh := make(chan struct{})
			ctx, cancel := context.WithCancel(ctx)

			go func() {
				defer close(doneCh)

				t := time.NewTicker(time.Millisecond * 100)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
					}
					select {
					case statsCh <- statsFn():
					case <-ctx.Done():
						return
					default:
						// We don't want to send old stats.
						continue
					}
				}
			}()
			return closeFunc(func() error {
				cancel()
				<-doneCh
				close(statsCh)
				return nil
			}), nil
		},
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		DERPMap:   metadata.DERPMap,
		Logger:    slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug),
	})
	require.NoError(t, err)
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = conn.Close()
	})
	go coordinator.ServeClient(serverConn, uuid.New(), agentID)
	sendNode, _ := tailnet.ServeCoordinator(clientConn, func(node []*tailnet.Node) error {
		return conn.UpdateNodes(node)
	})
	conn.SetNodeCallback(sendNode)
	return &codersdk.AgentConn{
		Conn: conn,
	}, statsCh
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
