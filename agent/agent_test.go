package agent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/google/uuid"
	"github.com/pion/udp"
	"github.com/pkg/sftp"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/crypto/ssh"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"golang.org/x/xerrors"
	"tailscale.com/net/speedtest"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/tailnet/tailnettest"
	"github.com/coder/coder/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// NOTE: These tests only work when your default shell is bash for some reason.

func TestAgent_Stats_SSH(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	conn, _, stats, _ := setupAgent(t, agentsdk.Metadata{}, 0)

	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshClient.Close()
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	defer session.Close()
	require.NoError(t, session.Run("echo test"))

	var s *agentsdk.Stats
	require.Eventuallyf(t, func() bool {
		var ok bool
		s, ok = <-stats
		return ok && s.ConnectionCount > 0 && s.RxBytes > 0 && s.TxBytes > 0
	}, testutil.WaitLong, testutil.IntervalFast,
		"never saw stats: %+v", s,
	)
}

func TestAgent_Stats_ReconnectingPTY(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	conn, _, stats, _ := setupAgent(t, agentsdk.Metadata{}, 0)

	ptyConn, err := conn.ReconnectingPTY(ctx, uuid.New(), 128, 128, "/bin/bash")
	require.NoError(t, err)
	defer ptyConn.Close()

	data, err := json.Marshal(codersdk.ReconnectingPTYRequest{
		Data: "echo test\r\n",
	})
	require.NoError(t, err)
	_, err = ptyConn.Write(data)
	require.NoError(t, err)

	var s *agentsdk.Stats
	require.Eventuallyf(t, func() bool {
		var ok bool
		s, ok = <-stats
		return ok && s.ConnectionCount > 0 && s.RxBytes > 0 && s.TxBytes > 0
	}, testutil.WaitLong, testutil.IntervalFast,
		"never saw stats: %+v", s,
	)
}

func TestAgent_SessionExec(t *testing.T) {
	t.Parallel()
	session := setupSSHSession(t, agentsdk.Metadata{})

	command := "echo test"
	if runtime.GOOS == "windows" {
		command = "cmd.exe /c echo test"
	}
	output, err := session.Output(command)
	require.NoError(t, err)
	require.Equal(t, "test", strings.TrimSpace(string(output)))
}

func TestAgent_GitSSH(t *testing.T) {
	t.Parallel()
	session := setupSSHSession(t, agentsdk.Metadata{})
	command := "sh -c 'echo $GIT_SSH_COMMAND'"
	if runtime.GOOS == "windows" {
		command = "cmd.exe /c echo %GIT_SSH_COMMAND%"
	}
	output, err := session.Output(command)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(strings.TrimSpace(string(output)), "gitssh --"))
}

func TestAgent_SessionTTYShell(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)
	if runtime.GOOS == "windows" {
		// This might be our implementation, or ConPTY itself.
		// It's difficult to find extensive tests for it, so
		// it seems like it could be either.
		t.Skip("ConPTY appears to be inconsistent on Windows.")
	}
	session := setupSSHSession(t, agentsdk.Metadata{})
	command := "sh"
	if runtime.GOOS == "windows" {
		command = "cmd.exe"
	}
	err := session.RequestPty("xterm", 128, 128, ssh.TerminalModes{})
	require.NoError(t, err)
	ptty := ptytest.New(t)
	session.Stdout = ptty.Output()
	session.Stderr = ptty.Output()
	session.Stdin = ptty.Input()
	err = session.Start(command)
	require.NoError(t, err)
	_ = ptty.Peek(ctx, 1) // wait for the prompt
	ptty.WriteLine("echo test")
	ptty.ExpectMatch("test")
	ptty.WriteLine("exit")
	err = session.Wait()
	require.NoError(t, err)
}

func TestAgent_SessionTTYExitCode(t *testing.T) {
	t.Parallel()
	session := setupSSHSession(t, agentsdk.Metadata{})
	command := "areallynotrealcommand"
	err := session.RequestPty("xterm", 128, 128, ssh.TerminalModes{})
	require.NoError(t, err)
	ptty := ptytest.New(t)
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
}

//nolint:paralleltest // This test sets an environment variable.
func TestAgent_Session_TTY_MOTD(t *testing.T) {
	if runtime.GOOS == "windows" {
		// This might be our implementation, or ConPTY itself.
		// It's difficult to find extensive tests for it, so
		// it seems like it could be either.
		t.Skip("ConPTY appears to be inconsistent on Windows.")
	}

	wantMOTD := "Welcome to your Coder workspace!"

	tmpdir := t.TempDir()
	name := filepath.Join(tmpdir, "motd")
	err := os.WriteFile(name, []byte(wantMOTD), 0o600)
	require.NoError(t, err, "write motd file")

	// Set HOME so we can ensure no ~/.hushlogin is present.
	t.Setenv("HOME", tmpdir)

	session := setupSSHSession(t, agentsdk.Metadata{
		MOTDFile: name,
	})
	err = session.RequestPty("xterm", 128, 128, ssh.TerminalModes{})
	require.NoError(t, err)

	ptty := ptytest.New(t)
	var stdout bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = ptty.Output()
	session.Stdin = ptty.Input()
	err = session.Shell()
	require.NoError(t, err)

	ptty.WriteLine("exit 0")
	err = session.Wait()
	require.NoError(t, err)

	require.Contains(t, stdout.String(), wantMOTD, "should show motd")
}

//nolint:paralleltest // This test sets an environment variable.
func TestAgent_Session_TTY_Hushlogin(t *testing.T) {
	if runtime.GOOS == "windows" {
		// This might be our implementation, or ConPTY itself.
		// It's difficult to find extensive tests for it, so
		// it seems like it could be either.
		t.Skip("ConPTY appears to be inconsistent on Windows.")
	}

	wantNotMOTD := "Welcome to your Coder workspace!"

	tmpdir := t.TempDir()
	name := filepath.Join(tmpdir, "motd")
	err := os.WriteFile(name, []byte(wantNotMOTD), 0o600)
	require.NoError(t, err, "write motd file")

	// Create hushlogin to silence motd.
	f, err := os.Create(filepath.Join(tmpdir, ".hushlogin"))
	require.NoError(t, err, "create .hushlogin file")
	err = f.Close()
	require.NoError(t, err, "close .hushlogin file")

	// Set HOME so we can ensure ~/.hushlogin is present.
	t.Setenv("HOME", tmpdir)

	session := setupSSHSession(t, agentsdk.Metadata{
		MOTDFile: name,
	})
	err = session.RequestPty("xterm", 128, 128, ssh.TerminalModes{})
	require.NoError(t, err)

	ptty := ptytest.New(t)
	var stdout bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = ptty.Output()
	session.Stdin = ptty.Input()
	err = session.Shell()
	require.NoError(t, err)

	ptty.WriteLine("exit 0")
	err = session.Wait()
	require.NoError(t, err)

	require.NotContains(t, stdout.String(), wantNotMOTD, "should not show motd")
}

//nolint:paralleltest // This test reserves a port.
func TestAgent_TCPLocalForwarding(t *testing.T) {
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
	remotePort := tcpAddr.Port
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := local.Accept()
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()
		b := make([]byte, 4)
		_, err = conn.Read(b)
		if !assert.NoError(t, err) {
			return
		}
		_, err = conn.Write(b)
		if !assert.NoError(t, err) {
			return
		}
	}()

	cmd := setupSSHCommand(t, []string{"-L", fmt.Sprintf("%d:127.0.0.1:%d", randomPort, remotePort)}, []string{"sleep", "5"})
	err = cmd.Start()
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(randomPort))
		if err != nil {
			return false
		}
		defer conn.Close()
		_, err = conn.Write([]byte("test"))
		if !assert.NoError(t, err) {
			return false
		}
		b := make([]byte, 4)
		_, err = conn.Read(b)
		if !assert.NoError(t, err) {
			return false
		}
		if !assert.Equal(t, "test", string(b)) {
			return false
		}

		return true
	}, testutil.WaitLong, testutil.IntervalSlow)

	<-done

	_ = cmd.Process.Kill()
}

//nolint:paralleltest // This test reserves a port.
func TestAgent_TCPRemoteForwarding(t *testing.T) {
	random, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_ = random.Close()
	tcpAddr, valid := random.Addr().(*net.TCPAddr)
	require.True(t, valid)
	randomPort := tcpAddr.Port

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	tcpAddr, valid = l.Addr().(*net.TCPAddr)
	require.True(t, valid)
	localPort := tcpAddr.Port

	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		b := make([]byte, 4)
		_, err = conn.Read(b)
		if !assert.NoError(t, err) {
			return
		}
		_, err = conn.Write(b)
		if !assert.NoError(t, err) {
			return
		}
	}()

	cmd := setupSSHCommand(t, []string{"-R", fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", randomPort, localPort)}, []string{"sleep", "5"})
	err = cmd.Start()
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", randomPort))
		if err != nil {
			return false
		}
		defer conn.Close()
		_, err = conn.Write([]byte("test"))
		if !assert.NoError(t, err) {
			return false
		}
		b := make([]byte, 4)
		_, err = conn.Read(b)
		if !assert.NoError(t, err) {
			return false
		}
		if !assert.Equal(t, "test", string(b)) {
			return false
		}

		return true
	}, testutil.WaitLong, testutil.IntervalSlow)

	<-done

	_ = cmd.Process.Kill()
}

func TestAgent_UnixLocalForwarding(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix domain sockets are not fully supported on Windows")
	}

	tmpdir := tempDirUnixSocket(t)
	remoteSocketPath := filepath.Join(tmpdir, "remote-socket")
	localSocketPath := filepath.Join(tmpdir, "local-socket")

	l, err := net.Listen("unix", remoteSocketPath)
	require.NoError(t, err)
	defer l.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		b := make([]byte, 4)
		_, err = conn.Read(b)
		if !assert.NoError(t, err) {
			return
		}
		_, err = conn.Write(b)
		if !assert.NoError(t, err) {
			return
		}
	}()

	cmd := setupSSHCommand(t, []string{"-L", fmt.Sprintf("%s:%s", localSocketPath, remoteSocketPath)}, []string{"sleep", "5"})
	err = cmd.Start()
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := os.Stat(localSocketPath)
		return err == nil
	}, testutil.WaitLong, testutil.IntervalFast)

	conn, err := net.Dial("unix", localSocketPath)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write([]byte("test"))
	require.NoError(t, err)
	b := make([]byte, 4)
	_, err = conn.Read(b)
	require.NoError(t, err)
	require.Equal(t, "test", string(b))
	_ = conn.Close()
	<-done

	_ = cmd.Process.Kill()
}

func TestAgent_UnixRemoteForwarding(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix domain sockets are not fully supported on Windows")
	}

	tmpdir := tempDirUnixSocket(t)
	remoteSocketPath := filepath.Join(tmpdir, "remote-socket")
	localSocketPath := filepath.Join(tmpdir, "local-socket")

	l, err := net.Listen("unix", localSocketPath)
	require.NoError(t, err)
	defer l.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		b := make([]byte, 4)
		_, err = conn.Read(b)
		if !assert.NoError(t, err) {
			return
		}
		_, err = conn.Write(b)
		if !assert.NoError(t, err) {
			return
		}
	}()

	cmd := setupSSHCommand(t, []string{"-R", fmt.Sprintf("%s:%s", remoteSocketPath, localSocketPath)}, []string{"sleep", "5"})
	err = cmd.Start()
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := os.Stat(remoteSocketPath)
		return err == nil
	}, testutil.WaitLong, testutil.IntervalFast)

	conn, err := net.Dial("unix", remoteSocketPath)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write([]byte("test"))
	require.NoError(t, err)
	b := make([]byte, 4)
	_, err = conn.Read(b)
	require.NoError(t, err)
	require.Equal(t, "test", string(b))
	_ = conn.Close()

	<-done

	_ = cmd.Process.Kill()
}

func TestAgent_SFTP(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	u, err := user.Current()
	require.NoError(t, err, "get current user")
	home := u.HomeDir
	if runtime.GOOS == "windows" {
		home = "/" + strings.ReplaceAll(home, "\\", "/")
	}
	//nolint:dogsled
	conn, _, _, _ := setupAgent(t, agentsdk.Metadata{}, 0)
	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshClient.Close()
	client, err := sftp.NewClient(sshClient)
	require.NoError(t, err)
	defer client.Close()
	wd, err := client.Getwd()
	require.NoError(t, err, "get working directory")
	require.Equal(t, home, wd, "working directory should be home user home")
	tempFile := filepath.Join(t.TempDir(), "sftp")
	// SFTP only accepts unix-y paths.
	remoteFile := filepath.ToSlash(tempFile)
	if !path.IsAbs(remoteFile) {
		// On Windows, e.g. "/C:/Users/...".
		remoteFile = path.Join("/", remoteFile)
	}
	file, err := client.Create(remoteFile)
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)
	_, err = os.Stat(tempFile)
	require.NoError(t, err)
}

func TestAgent_SCP(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	//nolint:dogsled
	conn, _, _, _ := setupAgent(t, agentsdk.Metadata{}, 0)
	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshClient.Close()
	scpClient, err := scp.NewClientBySSH(sshClient)
	require.NoError(t, err)
	defer scpClient.Close()
	tempFile := filepath.Join(t.TempDir(), "scp")
	content := "hello world"
	err = scpClient.CopyFile(context.Background(), strings.NewReader(content), tempFile, "0755")
	require.NoError(t, err)
	_, err = os.Stat(tempFile)
	require.NoError(t, err)
}

func TestAgent_EnvironmentVariables(t *testing.T) {
	t.Parallel()
	key := "EXAMPLE"
	value := "value"
	session := setupSSHSession(t, agentsdk.Metadata{
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
}

func TestAgent_EnvironmentVariableExpansion(t *testing.T) {
	t.Parallel()
	key := "EXAMPLE"
	session := setupSSHSession(t, agentsdk.Metadata{
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
}

func TestAgent_CoderEnvVars(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"CODER"} {
		key := key
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			session := setupSSHSession(t, agentsdk.Metadata{})
			command := "sh -c 'echo $" + key + "'"
			if runtime.GOOS == "windows" {
				command = "cmd.exe /c echo %" + key + "%"
			}
			output, err := session.Output(command)
			require.NoError(t, err)
			require.NotEmpty(t, strings.TrimSpace(string(output)))
		})
	}
}

func TestAgent_SSHConnectionEnvVars(t *testing.T) {
	t.Parallel()

	// Note: the SSH_TTY environment variable should only be set for TTYs.
	// For some reason this test produces a TTY locally and a non-TTY in CI
	// so we don't test for the absence of SSH_TTY.
	for _, key := range []string{"SSH_CONNECTION", "SSH_CLIENT"} {
		key := key
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			session := setupSSHSession(t, agentsdk.Metadata{})
			command := "sh -c 'echo $" + key + "'"
			if runtime.GOOS == "windows" {
				command = "cmd.exe /c echo %" + key + "%"
			}
			output, err := session.Output(command)
			require.NoError(t, err)
			require.NotEmpty(t, strings.TrimSpace(string(output)))
		})
	}
}

func TestAgent_StartupScript(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("This test doesn't work on Windows for some reason...")
	}
	content := "output"
	//nolint:dogsled
	_, _, _, fs := setupAgent(t, agentsdk.Metadata{
		StartupScript: "echo " + content,
	}, 0)
	var gotContent string
	require.Eventually(t, func() bool {
		outputPath := filepath.Join(os.TempDir(), "coder-startup-script.log")
		content, err := afero.ReadFile(fs, outputPath)
		if err != nil {
			t.Logf("read file %q: %s", outputPath, err)
			return false
		}
		if len(content) == 0 {
			t.Logf("no content in %q", outputPath)
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
	}, testutil.WaitShort, testutil.IntervalMedium)
	require.Equal(t, content, strings.TrimSpace(gotContent))
}

func TestAgent_Lifecycle(t *testing.T) {
	t.Parallel()

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()

		_, client, _, _ := setupAgent(t, agentsdk.Metadata{
			StartupScript:        "sleep 5",
			StartupScriptTimeout: time.Nanosecond,
		}, 0)

		want := []codersdk.WorkspaceAgentLifecycle{
			codersdk.WorkspaceAgentLifecycleStarting,
			codersdk.WorkspaceAgentLifecycleStartTimeout,
		}

		var got []codersdk.WorkspaceAgentLifecycle
		assert.Eventually(t, func() bool {
			got = client.getLifecycleStates()
			return len(got) > 0 && got[len(got)-1] == want[len(want)-1]
		}, testutil.WaitShort, testutil.IntervalMedium)
		switch len(got) {
		case 1:
			// This can happen if lifecycle state updates are
			// too fast, only the latest one is reported.
			require.Equal(t, want[1:], got)
		default:
			// This is the expected case.
			require.Equal(t, want, got)
		}
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()

		_, client, _, _ := setupAgent(t, agentsdk.Metadata{
			StartupScript:        "false",
			StartupScriptTimeout: 30 * time.Second,
		}, 0)

		want := []codersdk.WorkspaceAgentLifecycle{
			codersdk.WorkspaceAgentLifecycleStarting,
			codersdk.WorkspaceAgentLifecycleStartError,
		}

		var got []codersdk.WorkspaceAgentLifecycle
		assert.Eventually(t, func() bool {
			got = client.getLifecycleStates()
			return len(got) > 0 && got[len(got)-1] == want[len(want)-1]
		}, testutil.WaitShort, testutil.IntervalMedium)
		switch len(got) {
		case 1:
			// This can happen if lifecycle state updates are
			// too fast, only the latest one is reported.
			require.Equal(t, want[1:], got)
		default:
			// This is the expected case.
			require.Equal(t, want, got)
		}
	})

	t.Run("Ready", func(t *testing.T) {
		t.Parallel()

		_, client, _, _ := setupAgent(t, agentsdk.Metadata{
			StartupScript:        "true",
			StartupScriptTimeout: 30 * time.Second,
		}, 0)

		want := []codersdk.WorkspaceAgentLifecycle{
			codersdk.WorkspaceAgentLifecycleStarting,
			codersdk.WorkspaceAgentLifecycleReady,
		}

		var got []codersdk.WorkspaceAgentLifecycle
		assert.Eventually(t, func() bool {
			got = client.getLifecycleStates()
			return len(got) > 0 && got[len(got)-1] == want[len(want)-1]
		}, testutil.WaitShort, testutil.IntervalMedium)
		switch len(got) {
		case 1:
			// This can happen if lifecycle state updates are
			// too fast, only the latest one is reported.
			require.Equal(t, want[1:], got)
		default:
			// This is the expected case.
			require.Equal(t, want, got)
		}
	})
}

func TestAgent_Startup(t *testing.T) {
	t.Parallel()

	t.Run("EmptyDirectory", func(t *testing.T) {
		t.Parallel()

		_, client, _, _ := setupAgent(t, agentsdk.Metadata{
			StartupScript:        "true",
			StartupScriptTimeout: 30 * time.Second,
			Directory:            "",
		}, 0)
		assert.Eventually(t, func() bool {
			return client.getStartup().Version != ""
		}, testutil.WaitShort, testutil.IntervalFast)
		require.Equal(t, "", client.getStartup().ExpandedDirectory)
	})

	t.Run("HomeDirectory", func(t *testing.T) {
		t.Parallel()

		_, client, _, _ := setupAgent(t, agentsdk.Metadata{
			StartupScript:        "true",
			StartupScriptTimeout: 30 * time.Second,
			Directory:            "~",
		}, 0)
		assert.Eventually(t, func() bool {
			return client.getStartup().Version != ""
		}, testutil.WaitShort, testutil.IntervalFast)
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		require.Equal(t, homeDir, client.getStartup().ExpandedDirectory)
	})

	t.Run("HomeEnvironmentVariable", func(t *testing.T) {
		t.Parallel()

		_, client, _, _ := setupAgent(t, agentsdk.Metadata{
			StartupScript:        "true",
			StartupScriptTimeout: 30 * time.Second,
			Directory:            "$HOME",
		}, 0)
		assert.Eventually(t, func() bool {
			return client.getStartup().Version != ""
		}, testutil.WaitShort, testutil.IntervalFast)
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		require.Equal(t, homeDir, client.getStartup().ExpandedDirectory)
	})
}

func TestAgent_ReconnectingPTY(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		// This might be our implementation, or ConPTY itself.
		// It's difficult to find extensive tests for it, so
		// it seems like it could be either.
		t.Skip("ConPTY appears to be inconsistent on Windows.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	//nolint:dogsled
	conn, _, _, _ := setupAgent(t, agentsdk.Metadata{}, 0)
	id := uuid.New()
	netConn, err := conn.ReconnectingPTY(ctx, id, 100, 100, "/bin/bash")
	require.NoError(t, err)
	defer netConn.Close()

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
	netConn, err = conn.ReconnectingPTY(ctx, id, 100, 100, "/bin/bash")
	require.NoError(t, err)
	defer netConn.Close()

	bufRead = bufio.NewReader(netConn)

	// Same output again!
	expectLine(matchEchoCommand)
	expectLine(matchEchoOutput)
}

func TestAgent_Dial(t *testing.T) {
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

			//nolint:dogsled
			conn, _, _, _ := setupAgent(t, agentsdk.Metadata{}, 0)
			require.True(t, conn.AwaitReachable(context.Background()))
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
}

func TestAgent_Speedtest(t *testing.T) {
	t.Parallel()
	t.Skip("This test is relatively flakey because of Tailscale's speedtest code...")
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	derpMap := tailnettest.RunDERPAndSTUN(t)
	//nolint:dogsled
	conn, _, _, _ := setupAgent(t, agentsdk.Metadata{
		DERPMap: derpMap,
	}, 0)
	defer conn.Close()
	res, err := conn.Speedtest(ctx, speedtest.Upload, 250*time.Millisecond)
	require.NoError(t, err)
	t.Logf("%.2f MBits/s", res[len(res)-1].MBitsPerSecond())
}

func TestAgent_Reconnect(t *testing.T) {
	t.Parallel()
	// After the agent is disconnected from a coordinator, it's supposed
	// to reconnect!
	coordinator := tailnet.NewCoordinator()
	defer coordinator.Close()

	agentID := uuid.New()
	statsCh := make(chan *agentsdk.Stats)
	derpMap := tailnettest.RunDERPAndSTUN(t)
	client := &client{
		t:       t,
		agentID: agentID,
		metadata: agentsdk.Metadata{
			DERPMap: derpMap,
		},
		statsChan:   statsCh,
		coordinator: coordinator,
	}
	initialized := atomic.Int32{}
	closer := agent.New(agent.Options{
		ExchangeToken: func(ctx context.Context) (string, error) {
			initialized.Add(1)
			return "", nil
		},
		Client: client,
		Logger: slogtest.Make(t, nil).Leveled(slog.LevelInfo),
	})
	defer closer.Close()

	require.Eventually(t, func() bool {
		return coordinator.Node(agentID) != nil
	}, testutil.WaitShort, testutil.IntervalFast)
	client.lastWorkspaceAgent()
	require.Eventually(t, func() bool {
		return initialized.Load() == 2
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestAgent_WriteVSCodeConfigs(t *testing.T) {
	t.Parallel()

	coordinator := tailnet.NewCoordinator()
	defer coordinator.Close()

	client := &client{
		t:       t,
		agentID: uuid.New(),
		metadata: agentsdk.Metadata{
			GitAuthConfigs: 1,
			DERPMap:        &tailcfg.DERPMap{},
		},
		statsChan:   make(chan *agentsdk.Stats),
		coordinator: coordinator,
	}
	filesystem := afero.NewMemMapFs()
	closer := agent.New(agent.Options{
		ExchangeToken: func(ctx context.Context) (string, error) {
			return "", nil
		},
		Client:     client,
		Logger:     slogtest.Make(t, nil).Leveled(slog.LevelInfo),
		Filesystem: filesystem,
	})
	defer closer.Close()

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	name := filepath.Join(home, ".vscode-server", "data", "Machine", "settings.json")
	require.Eventually(t, func() bool {
		_, err := filesystem.Stat(name)
		return err == nil
	}, testutil.WaitShort, testutil.IntervalFast)
}

func setupSSHCommand(t *testing.T, beforeArgs []string, afterArgs []string) *exec.Cmd {
	//nolint:dogsled
	agentConn, _, _, _ := setupAgent(t, agentsdk.Metadata{}, 0)
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

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			ssh, err := agentConn.SSH(ctx)
			cancel()
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
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"host",
	)
	args = append(args, afterArgs...)
	return exec.Command("ssh", args...)
}

func setupSSHSession(t *testing.T, options agentsdk.Metadata) *ssh.Session {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	//nolint:dogsled
	conn, _, _, _ := setupAgent(t, options, 0)
	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sshClient.Close()
	})
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = session.Close()
	})
	return session
}

type closeFunc func() error

func (c closeFunc) Close() error {
	return c()
}

func setupAgent(t *testing.T, metadata agentsdk.Metadata, ptyTimeout time.Duration) (
	*codersdk.WorkspaceAgentConn,
	*client,
	<-chan *agentsdk.Stats,
	afero.Fs,
) {
	if metadata.DERPMap == nil {
		metadata.DERPMap = tailnettest.RunDERPAndSTUN(t)
	}
	coordinator := tailnet.NewCoordinator()
	t.Cleanup(func() {
		_ = coordinator.Close()
	})
	agentID := uuid.New()
	statsCh := make(chan *agentsdk.Stats, 50)
	fs := afero.NewMemMapFs()
	c := &client{
		t:           t,
		agentID:     agentID,
		metadata:    metadata,
		statsChan:   statsCh,
		coordinator: coordinator,
	}
	closer := agent.New(agent.Options{
		Client:                 c,
		Filesystem:             fs,
		Logger:                 slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
		ReconnectingPTYTimeout: ptyTimeout,
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
	serveClientDone := make(chan struct{})
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = conn.Close()
		<-serveClientDone
	})
	go func() {
		defer close(serveClientDone)
		coordinator.ServeClient(serverConn, uuid.New(), agentID)
	}()
	sendNode, _ := tailnet.ServeCoordinator(clientConn, func(node []*tailnet.Node) error {
		return conn.UpdateNodes(node, false)
	})
	conn.SetNodeCallback(sendNode)
	agentConn := &codersdk.WorkspaceAgentConn{
		Conn: conn,
	}
	t.Cleanup(func() {
		_ = agentConn.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	if !agentConn.AwaitReachable(ctx) {
		t.Fatal("agent not reachable")
	}
	return agentConn, c, statsCh, fs
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

type client struct {
	t                  *testing.T
	agentID            uuid.UUID
	metadata           agentsdk.Metadata
	statsChan          chan *agentsdk.Stats
	coordinator        tailnet.Coordinator
	lastWorkspaceAgent func()

	mu              sync.Mutex // Protects following.
	lifecycleStates []codersdk.WorkspaceAgentLifecycle
	startup         agentsdk.PostStartupRequest
}

func (c *client) Metadata(_ context.Context) (agentsdk.Metadata, error) {
	return c.metadata, nil
}

func (c *client) Listen(_ context.Context) (net.Conn, error) {
	clientConn, serverConn := net.Pipe()
	closed := make(chan struct{})
	c.lastWorkspaceAgent = func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
		<-closed
	}
	c.t.Cleanup(c.lastWorkspaceAgent)
	go func() {
		_ = c.coordinator.ServeAgent(serverConn, c.agentID, "")
		close(closed)
	}()
	return clientConn, nil
}

func (c *client) ReportStats(ctx context.Context, _ slog.Logger, statsChan <-chan *agentsdk.Stats, setInterval func(time.Duration)) (io.Closer, error) {
	doneCh := make(chan struct{})
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer close(doneCh)

		setInterval(500 * time.Millisecond)
		for {
			select {
			case <-ctx.Done():
				return
			case stat := <-statsChan:
				select {
				case c.statsChan <- stat:
				case <-ctx.Done():
					return
				default:
					// We don't want to send old stats.
					continue
				}
			}
		}
	}()
	return closeFunc(func() error {
		cancel()
		<-doneCh
		close(c.statsChan)
		return nil
	}), nil
}

func (c *client) getLifecycleStates() []codersdk.WorkspaceAgentLifecycle {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lifecycleStates
}

func (c *client) PostLifecycle(_ context.Context, req agentsdk.PostLifecycleRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lifecycleStates = append(c.lifecycleStates, req.State)
	return nil
}

func (*client) PostAppHealth(_ context.Context, _ agentsdk.PostAppHealthsRequest) error {
	return nil
}

func (c *client) getStartup() agentsdk.PostStartupRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startup
}

func (c *client) PostStartup(_ context.Context, startup agentsdk.PostStartupRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startup = startup
	return nil
}

// tempDirUnixSocket returns a temporary directory that can safely hold unix
// sockets (probably).
//
// During tests on darwin we hit the max path length limit for unix sockets
// pretty easily in the default location, so this function uses /tmp instead to
// get shorter paths.
func tempDirUnixSocket(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "darwin" {
		testName := strings.ReplaceAll(t.Name(), "/", "_")
		dir, err := os.MkdirTemp("/tmp", fmt.Sprintf("coder-test-%s-", testName))
		require.NoError(t, err, "create temp dir for gpg test")

		t.Cleanup(func() {
			err := os.RemoveAll(dir)
			assert.NoError(t, err, "remove temp dir", dir)
		})
		return dir
	}

	return t.TempDir()
}
