package agent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
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
	"github.com/prometheus/client_golang/prometheus"
	promgo "github.com/prometheus/client_model/go"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/crypto/ssh"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
	"tailscale.com/net/speedtest"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/agent/agentssh"
	"github.com/coder/coder/agent/agenttest"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/pty"
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

	//nolint:dogsled
	conn, _, stats, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)

	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshClient.Close()
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	defer session.Close()
	stdin, err := session.StdinPipe()
	require.NoError(t, err)
	err = session.Shell()
	require.NoError(t, err)

	var s *agentsdk.Stats
	require.Eventuallyf(t, func() bool {
		var ok bool
		s, ok = <-stats
		return ok && s.ConnectionCount > 0 && s.RxBytes > 0 && s.TxBytes > 0 && s.SessionCountSSH == 1
	}, testutil.WaitLong, testutil.IntervalFast,
		"never saw stats: %+v", s,
	)
	_ = stdin.Close()
	err = session.Wait()
	require.NoError(t, err)
}

func TestAgent_Stats_ReconnectingPTY(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	//nolint:dogsled
	conn, _, stats, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)

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
		return ok && s.ConnectionCount > 0 && s.RxBytes > 0 && s.TxBytes > 0 && s.SessionCountReconnectingPTY == 1
	}, testutil.WaitLong, testutil.IntervalFast,
		"never saw stats: %+v", s,
	)
}

func TestAgent_Stats_Magic(t *testing.T) {
	t.Parallel()
	t.Run("StripsEnvironmentVariable", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		//nolint:dogsled
		conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
		sshClient, err := conn.SSHClient(ctx)
		require.NoError(t, err)
		defer sshClient.Close()
		session, err := sshClient.NewSession()
		require.NoError(t, err)
		session.Setenv(agentssh.MagicSessionTypeEnvironmentVariable, agentssh.MagicSessionTypeVSCode)
		defer session.Close()

		command := "sh -c 'echo $" + agentssh.MagicSessionTypeEnvironmentVariable + "'"
		expected := ""
		if runtime.GOOS == "windows" {
			expected = "%" + agentssh.MagicSessionTypeEnvironmentVariable + "%"
			command = "cmd.exe /c echo " + expected
		}
		output, err := session.Output(command)
		require.NoError(t, err)
		require.Equal(t, expected, strings.TrimSpace(string(output)))
	})
	t.Run("Tracks", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "window" {
			t.Skip("Sleeping for infinity doesn't work on Windows")
		}
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		//nolint:dogsled
		conn, _, stats, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
		sshClient, err := conn.SSHClient(ctx)
		require.NoError(t, err)
		defer sshClient.Close()
		session, err := sshClient.NewSession()
		require.NoError(t, err)
		session.Setenv(agentssh.MagicSessionTypeEnvironmentVariable, agentssh.MagicSessionTypeVSCode)
		defer session.Close()
		stdin, err := session.StdinPipe()
		require.NoError(t, err)
		err = session.Shell()
		require.NoError(t, err)
		var s *agentsdk.Stats
		require.Eventuallyf(t, func() bool {
			var ok bool
			s, ok = <-stats
			return ok && s.ConnectionCount > 0 && s.RxBytes > 0 && s.TxBytes > 0 &&
				// Ensure that the connection didn't count as a "normal" SSH session.
				// This was a special one, so it should be labeled specially in the stats!
				s.SessionCountVSCode == 1 &&
				// Ensure that connection latency is being counted!
				// If it isn't, it's set to -1.
				s.ConnectionMedianLatencyMS >= 0
		}, testutil.WaitLong, testutil.IntervalFast,
			"never saw stats: %+v", s,
		)
		// The shell will automatically exit if there is no stdin!
		_ = stdin.Close()
		err = session.Wait()
		require.NoError(t, err)
	})
}

func TestAgent_SessionExec(t *testing.T) {
	t.Parallel()
	session := setupSSHSession(t, agentsdk.Manifest{}, codersdk.ServiceBannerConfig{}, nil)

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
	session := setupSSHSession(t, agentsdk.Manifest{}, codersdk.ServiceBannerConfig{}, nil)
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
	session := setupSSHSession(t, agentsdk.Manifest{}, codersdk.ServiceBannerConfig{}, nil)
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
	session := setupSSHSession(t, agentsdk.Manifest{}, codersdk.ServiceBannerConfig{}, nil)
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

func TestAgent_Session_TTY_MOTD(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		// This might be our implementation, or ConPTY itself.
		// It's difficult to find extensive tests for it, so
		// it seems like it could be either.
		t.Skip("ConPTY appears to be inconsistent on Windows.")
	}

	u, err := user.Current()
	require.NoError(t, err, "get current user")

	name := filepath.Join(u.HomeDir, "motd")

	wantMOTD := "Welcome to your Coder workspace!"
	wantServiceBanner := "Service banner text goes here"

	tests := []struct {
		name       string
		manifest   agentsdk.Manifest
		banner     codersdk.ServiceBannerConfig
		expected   []string
		unexpected []string
		expectedRe *regexp.Regexp
	}{
		{
			name:       "WithoutServiceBanner",
			manifest:   agentsdk.Manifest{MOTDFile: name},
			banner:     codersdk.ServiceBannerConfig{},
			expected:   []string{wantMOTD},
			unexpected: []string{wantServiceBanner},
		},
		{
			name:     "WithServiceBanner",
			manifest: agentsdk.Manifest{MOTDFile: name},
			banner: codersdk.ServiceBannerConfig{
				Enabled: true,
				Message: wantServiceBanner,
			},
			expected: []string{wantMOTD, wantServiceBanner},
		},
		{
			name:     "ServiceBannerDisabled",
			manifest: agentsdk.Manifest{MOTDFile: name},
			banner: codersdk.ServiceBannerConfig{
				Enabled: false,
				Message: wantServiceBanner,
			},
			expected:   []string{wantMOTD},
			unexpected: []string{wantServiceBanner},
		},
		{
			name:     "ServiceBannerOnly",
			manifest: agentsdk.Manifest{},
			banner: codersdk.ServiceBannerConfig{
				Enabled: true,
				Message: wantServiceBanner,
			},
			expected:   []string{wantServiceBanner},
			unexpected: []string{wantMOTD},
		},
		{
			name:       "None",
			manifest:   agentsdk.Manifest{},
			banner:     codersdk.ServiceBannerConfig{},
			unexpected: []string{wantServiceBanner, wantMOTD},
		},
		{
			name:     "CarriageReturns",
			manifest: agentsdk.Manifest{},
			banner: codersdk.ServiceBannerConfig{
				Enabled: true,
				Message: "service\n\nbanner\nhere",
			},
			expected:   []string{"service\r\n\r\nbanner\r\nhere\r\n\r\n"},
			unexpected: []string{},
		},
		{
			name:     "Trim",
			manifest: agentsdk.Manifest{},
			banner: codersdk.ServiceBannerConfig{
				Enabled: true,
				Message: "\n\n\n\n\n\nbanner\n\n\n\n\n\n",
			},
			expectedRe: regexp.MustCompile("([^\n\r]|^)banner\r\n\r\n[^\r\n]"),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			session := setupSSHSession(t, test.manifest, test.banner, func(fs afero.Fs) {
				err := fs.MkdirAll(filepath.Dir(name), 0o700)
				require.NoError(t, err)
				err = afero.WriteFile(fs, name, []byte(wantMOTD), 0o600)
				require.NoError(t, err)
			})
			testSessionOutput(t, session, test.expected, test.unexpected, test.expectedRe)
		})
	}
}

func TestAgent_Session_TTY_MOTD_Update(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		// This might be our implementation, or ConPTY itself.
		// It's difficult to find extensive tests for it, so
		// it seems like it could be either.
		t.Skip("ConPTY appears to be inconsistent on Windows.")
	}

	// Only the banner updates dynamically; the MOTD file does not.
	wantServiceBanner := "Service banner text goes here"

	tests := []struct {
		banner     codersdk.ServiceBannerConfig
		expected   []string
		unexpected []string
	}{
		{
			banner:     codersdk.ServiceBannerConfig{},
			expected:   []string{},
			unexpected: []string{wantServiceBanner},
		},
		{
			banner: codersdk.ServiceBannerConfig{
				Enabled: true,
				Message: wantServiceBanner,
			},
			expected: []string{wantServiceBanner},
		},
		{
			banner: codersdk.ServiceBannerConfig{
				Enabled: false,
				Message: wantServiceBanner,
			},
			expected:   []string{},
			unexpected: []string{wantServiceBanner},
		},
		{
			banner: codersdk.ServiceBannerConfig{
				Enabled: true,
				Message: wantServiceBanner,
			},
			expected:   []string{wantServiceBanner},
			unexpected: []string{},
		},
		{
			banner:     codersdk.ServiceBannerConfig{},
			unexpected: []string{wantServiceBanner},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	setSBInterval := func(_ *agenttest.Client, opts *agent.Options) {
		opts.ServiceBannerRefreshInterval = 5 * time.Millisecond
	}
	//nolint:dogsled // Allow the blank identifiers.
	conn, client, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0, setSBInterval)
	for _, test := range tests {
		test := test
		// Set new banner func and wait for the agent to call it to update the
		// banner.
		ready := make(chan struct{}, 2)
		client.SetServiceBannerFunc(func() (codersdk.ServiceBannerConfig, error) {
			select {
			case ready <- struct{}{}:
			default:
			}
			return test.banner, nil
		})
		<-ready
		<-ready // Wait for two updates to ensure the value has propagated.

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

		testSessionOutput(t, session, test.expected, test.unexpected, nil)
	}
}

//nolint:paralleltest // This test sets an environment variable.
func TestAgent_Session_TTY_QuietLogin(t *testing.T) {
	if runtime.GOOS == "windows" {
		// This might be our implementation, or ConPTY itself.
		// It's difficult to find extensive tests for it, so
		// it seems like it could be either.
		t.Skip("ConPTY appears to be inconsistent on Windows.")
	}

	wantNotMOTD := "Welcome to your Coder workspace!"
	wantMaybeServiceBanner := "Service banner text goes here"

	u, err := user.Current()
	require.NoError(t, err, "get current user")

	name := filepath.Join(u.HomeDir, "motd")

	// Neither banner nor MOTD should show if not a login shell.
	t.Run("NotLogin", func(t *testing.T) {
		session := setupSSHSession(t, agentsdk.Manifest{
			MOTDFile: name,
		}, codersdk.ServiceBannerConfig{
			Enabled: true,
			Message: wantMaybeServiceBanner,
		}, func(fs afero.Fs) {
			err := afero.WriteFile(fs, name, []byte(wantNotMOTD), 0o600)
			require.NoError(t, err, "write motd file")
		})
		err = session.RequestPty("xterm", 128, 128, ssh.TerminalModes{})
		require.NoError(t, err)

		wantEcho := "foobar"
		command := "echo " + wantEcho
		output, err := session.Output(command)
		require.NoError(t, err)

		require.Contains(t, string(output), wantEcho, "should show echo")
		require.NotContains(t, string(output), wantNotMOTD, "should not show motd")
		require.NotContains(t, string(output), wantMaybeServiceBanner, "should not show service banner")
	})

	// Only the MOTD should be silenced when hushlogin is present.
	t.Run("Hushlogin", func(t *testing.T) {
		session := setupSSHSession(t, agentsdk.Manifest{
			MOTDFile: name,
		}, codersdk.ServiceBannerConfig{
			Enabled: true,
			Message: wantMaybeServiceBanner,
		}, func(fs afero.Fs) {
			err := afero.WriteFile(fs, name, []byte(wantNotMOTD), 0o600)
			require.NoError(t, err, "write motd file")

			// Create hushlogin to silence motd.
			err = afero.WriteFile(fs, name, []byte{}, 0o600)
			require.NoError(t, err, "write hushlogin file")
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
		require.Contains(t, stdout.String(), wantMaybeServiceBanner, "should show service banner")
	})
}

func TestAgent_Session_TTY_FastCommandHasOutput(t *testing.T) {
	t.Parallel()
	// This test is here to prevent regressions where quickly executing
	// commands (with TTY) don't sync their output to the SSH session.
	//
	// See: https://github.com/coder/coder/issues/6656
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	//nolint:dogsled
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshClient.Close()

	ptty := ptytest.New(t)

	var stdout bytes.Buffer
	// NOTE(mafredri): Increase iterations to increase chance of failure,
	//                 assuming bug is present. Limiting GOMAXPROCS further
	//                 increases the chance of failure.
	// Using 1000 iterations is basically a guaranteed failure (but let's
	// not increase test times needlessly).
	// Limit GOMAXPROCS (e.g. `export GOMAXPROCS=1`) to further increase
	// chance of failure. Also -race helps.
	for i := 0; i < 5; i++ {
		func() {
			stdout.Reset()

			session, err := sshClient.NewSession()
			require.NoError(t, err)
			defer session.Close()
			err = session.RequestPty("xterm", 128, 128, ssh.TerminalModes{})
			require.NoError(t, err)

			session.Stdout = &stdout
			session.Stderr = ptty.Output()
			session.Stdin = ptty.Input()
			err = session.Start("echo wazzup")
			require.NoError(t, err)

			err = session.Wait()
			require.NoError(t, err)
			require.Contains(t, stdout.String(), "wazzup", "should output greeting")
		}()
	}
}

func TestAgent_Session_TTY_HugeOutputIsNotLost(t *testing.T) {
	t.Parallel()

	// This test is here to prevent regressions where a command (with or
	// without) a large amount of output would not be fully copied to the
	// SSH session. On unix systems, this was fixed by duplicating the file
	// descriptor of the PTY master and using it for copying the output.
	//
	// See: https://github.com/coder/coder/issues/6656
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	//nolint:dogsled
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshClient.Close()

	ptty := ptytest.New(t)

	var stdout bytes.Buffer
	// NOTE(mafredri): Increase iterations to increase chance of failure,
	//                 assuming bug is present.
	// Using 10 iterations is basically a guaranteed failure (but let's
	// not increase test times needlessly). Run with -race and do not
	// limit parallelism (`export GOMAXPROCS=10`) to increase the chance
	// of failure.
	for i := 0; i < 1; i++ {
		func() {
			stdout.Reset()

			session, err := sshClient.NewSession()
			require.NoError(t, err)
			defer session.Close()
			err = session.RequestPty("xterm", 128, 128, ssh.TerminalModes{})
			require.NoError(t, err)

			session.Stdout = &stdout
			session.Stderr = ptty.Output()
			session.Stdin = ptty.Input()
			want := strings.Repeat("wazzup", 1024+1) // ~6KB, +1 because 1024 is a common buffer size.
			err = session.Start("echo " + want)
			require.NoError(t, err)

			err = session.Wait()
			require.NoError(t, err)
			require.Contains(t, stdout.String(), want, "should output entire greeting")
		}()
	}
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

	_, proc := setupSSHCommand(t, []string{"-L", fmt.Sprintf("%d:127.0.0.1:%d", randomPort, remotePort)}, []string{"sleep", "5"})

	go func() {
		err := proc.Wait()
		select {
		case <-done:
		default:
			assert.NoError(t, err)
		}
	}()

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

	_ = proc.Kill()
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

	_, proc := setupSSHCommand(t, []string{"-R", fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", randomPort, localPort)}, []string{"sleep", "5"})

	go func() {
		err := proc.Wait()
		select {
		case <-done:
		default:
			assert.NoError(t, err)
		}
	}()

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

	_ = proc.Kill()
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

	_, proc := setupSSHCommand(t, []string{"-L", fmt.Sprintf("%s:%s", localSocketPath, remoteSocketPath)}, []string{"sleep", "5"})

	go func() {
		err := proc.Wait()
		select {
		case <-done:
		default:
			assert.NoError(t, err)
		}
	}()

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

	_ = proc.Kill()
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

	_, proc := setupSSHCommand(t, []string{"-R", fmt.Sprintf("%s:%s", remoteSocketPath, localSocketPath)}, []string{"sleep", "5"})

	go func() {
		err := proc.Wait()
		select {
		case <-done:
		default:
			assert.NoError(t, err)
		}
	}()

	// It's possible that the socket is created but the server is not ready to
	// accept connections yet. We need to retry until we can connect.
	//
	// Note that we wait long here because if the tailnet connection has trouble
	// connecting, it could take 5 seconds or more to reconnect.
	var conn net.Conn
	require.Eventually(t, func() bool {
		var err error
		conn, err = net.Dial("unix", remoteSocketPath)
		return err == nil
	}, testutil.WaitLong, testutil.IntervalFast)
	defer conn.Close()
	_, err = conn.Write([]byte("test"))
	require.NoError(t, err)
	b := make([]byte, 4)
	_, err = conn.Read(b)
	require.NoError(t, err)
	require.Equal(t, "test", string(b))
	_ = conn.Close()

	<-done

	_ = proc.Kill()
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
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
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
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
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
	session := setupSSHSession(t, agentsdk.Manifest{
		EnvironmentVariables: map[string]string{
			key: value,
		},
	}, codersdk.ServiceBannerConfig{}, nil)
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
	session := setupSSHSession(t, agentsdk.Manifest{
		EnvironmentVariables: map[string]string{
			key: "$SOMETHINGNOTSET",
		},
	}, codersdk.ServiceBannerConfig{}, nil)
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

			session := setupSSHSession(t, agentsdk.Manifest{}, codersdk.ServiceBannerConfig{}, nil)
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

			session := setupSSHSession(t, agentsdk.Manifest{}, codersdk.ServiceBannerConfig{}, nil)
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
	output := "something"
	command := "sh -c 'echo " + output + "'"
	if runtime.GOOS == "windows" {
		command = "cmd.exe /c echo " + output
	}
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		client := agenttest.NewClient(t,
			logger,
			uuid.New(),
			agentsdk.Manifest{
				StartupScript: command,
				DERPMap:       &tailcfg.DERPMap{},
			},
			make(chan *agentsdk.Stats),
			tailnet.NewCoordinator(logger),
		)
		closer := agent.New(agent.Options{
			Client:                 client,
			Filesystem:             afero.NewMemMapFs(),
			Logger:                 logger.Named("agent"),
			ReconnectingPTYTimeout: 0,
		})
		t.Cleanup(func() {
			_ = closer.Close()
		})
		assert.Eventually(t, func() bool {
			got := client.GetLifecycleStates()
			return len(got) > 0 && got[len(got)-1] == codersdk.WorkspaceAgentLifecycleReady
		}, testutil.WaitShort, testutil.IntervalMedium)

		require.Len(t, client.GetStartupLogs(), 1)
		require.Equal(t, output, client.GetStartupLogs()[0].Output)
	})
	// This ensures that even when coderd sends back that the startup
	// script has written too many lines it will still succeed!
	t.Run("OverflowsAndSkips", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		client := agenttest.NewClient(t,
			logger,
			uuid.New(),
			agentsdk.Manifest{
				StartupScript: command,
				DERPMap:       &tailcfg.DERPMap{},
			},
			make(chan *agentsdk.Stats, 50),
			tailnet.NewCoordinator(logger),
		)
		client.PatchWorkspaceLogs = func() error {
			resp := httptest.NewRecorder()
			httpapi.Write(context.Background(), resp, http.StatusRequestEntityTooLarge, codersdk.Response{
				Message: "Too many lines!",
			})
			res := resp.Result()
			defer res.Body.Close()
			return codersdk.ReadBodyAsError(res)
		}
		closer := agent.New(agent.Options{
			Client:                 client,
			Filesystem:             afero.NewMemMapFs(),
			Logger:                 logger.Named("agent"),
			ReconnectingPTYTimeout: 0,
		})
		t.Cleanup(func() {
			_ = closer.Close()
		})
		assert.Eventually(t, func() bool {
			got := client.GetLifecycleStates()
			return len(got) > 0 && got[len(got)-1] == codersdk.WorkspaceAgentLifecycleReady
		}, testutil.WaitShort, testutil.IntervalMedium)
		require.Len(t, client.GetStartupLogs(), 0)
	})
}

func TestAgent_Metadata(t *testing.T) {
	t.Parallel()

	echoHello := "echo 'hello'"

	t.Run("Once", func(t *testing.T) {
		t.Parallel()
		//nolint:dogsled
		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			Metadata: []codersdk.WorkspaceAgentMetadataDescription{
				{
					Key:      "greeting",
					Interval: 0,
					Script:   echoHello,
				},
			},
		}, 0, func(_ *agenttest.Client, opts *agent.Options) {
			opts.ReportMetadataInterval = 100 * time.Millisecond
		})

		var gotMd map[string]agentsdk.PostMetadataRequest
		require.Eventually(t, func() bool {
			gotMd = client.GetMetadata()
			return len(gotMd) == 1
		}, testutil.WaitShort, testutil.IntervalMedium)

		collectedAt := gotMd["greeting"].CollectedAt

		require.Never(t, func() bool {
			gotMd = client.GetMetadata()
			if len(gotMd) != 1 {
				panic("unexpected number of metadata")
			}
			return !gotMd["greeting"].CollectedAt.Equal(collectedAt)
		}, testutil.WaitShort, testutil.IntervalMedium)
	})

	t.Run("Many", func(t *testing.T) {
		t.Parallel()
		//nolint:dogsled
		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			Metadata: []codersdk.WorkspaceAgentMetadataDescription{
				{
					Key:      "greeting",
					Interval: 1,
					Timeout:  100,
					Script:   echoHello,
				},
			},
		}, 0, func(_ *agenttest.Client, opts *agent.Options) {
			opts.ReportMetadataInterval = testutil.IntervalFast
		})

		var gotMd map[string]agentsdk.PostMetadataRequest
		require.Eventually(t, func() bool {
			gotMd = client.GetMetadata()
			return len(gotMd) == 1
		}, testutil.WaitShort, testutil.IntervalFast/2)

		collectedAt1 := gotMd["greeting"].CollectedAt
		require.Equal(t, "hello", strings.TrimSpace(gotMd["greeting"].Value))

		if !assert.Eventually(t, func() bool {
			gotMd = client.GetMetadata()
			return gotMd["greeting"].CollectedAt.After(collectedAt1)
		}, testutil.WaitShort, testutil.IntervalFast/2) {
			t.Fatalf("expected metadata to be collected again")
		}
	})
}

func TestAgentMetadata_Timing(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Shell scripting in Windows is a pain, and we have already tested
		// that the OS logic works in the simpler tests.
		t.SkipNow()
	}
	testutil.SkipIfNotTiming(t)
	t.Parallel()

	dir := t.TempDir()

	const reportInterval = 2
	const intervalUnit = 100 * time.Millisecond
	var (
		greetingPath = filepath.Join(dir, "greeting")
		script       = "echo hello | tee -a " + greetingPath
	)
	//nolint:dogsled
	_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
		Metadata: []codersdk.WorkspaceAgentMetadataDescription{
			{
				Key:      "greeting",
				Interval: reportInterval,
				Script:   script,
			},
			{
				Key:      "bad",
				Interval: reportInterval,
				Script:   "exit 1",
			},
		},
	}, 0, func(_ *agenttest.Client, opts *agent.Options) {
		opts.ReportMetadataInterval = intervalUnit
	})

	require.Eventually(t, func() bool {
		return len(client.GetMetadata()) == 2
	}, testutil.WaitShort, testutil.IntervalMedium)

	for start := time.Now(); time.Since(start) < testutil.WaitMedium; time.Sleep(testutil.IntervalMedium) {
		md := client.GetMetadata()
		require.Len(t, md, 2, "got: %+v", md)

		require.Equal(t, "hello\n", md["greeting"].Value)
		require.Equal(t, "run cmd: exit status 1", md["bad"].Error)

		greetingByt, err := os.ReadFile(greetingPath)
		require.NoError(t, err)

		var (
			numGreetings      = bytes.Count(greetingByt, []byte("hello"))
			idealNumGreetings = time.Since(start) / (reportInterval * intervalUnit)
			// We allow a 50% error margin because the report loop may backlog
			// in CI and other toasters. In production, there is no hard
			// guarantee on timing either, and the frontend gives similar
			// wiggle room to the staleness of the value.
			upperBound = int(idealNumGreetings) + 1
			lowerBound = (int(idealNumGreetings) / 2)
		)

		if idealNumGreetings < 50 {
			// There is an insufficient sample size.
			continue
		}

		t.Logf("numGreetings: %d, idealNumGreetings: %d", numGreetings, idealNumGreetings)
		// The report loop may slow down on load, but it should never, ever
		// speed up.
		if numGreetings > upperBound {
			t.Fatalf("too many greetings: %d > %d in %v", numGreetings, upperBound, time.Since(start))
		} else if numGreetings < lowerBound {
			t.Fatalf("too few greetings: %d < %d", numGreetings, lowerBound)
		}
	}
}

func TestAgent_Lifecycle(t *testing.T) {
	t.Parallel()

	t.Run("StartTimeout", func(t *testing.T) {
		t.Parallel()

		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			StartupScript:        "sleep 3",
			StartupScriptTimeout: time.Nanosecond,
		}, 0)

		want := []codersdk.WorkspaceAgentLifecycle{
			codersdk.WorkspaceAgentLifecycleStarting,
			codersdk.WorkspaceAgentLifecycleStartTimeout,
		}

		var got []codersdk.WorkspaceAgentLifecycle
		assert.Eventually(t, func() bool {
			got = client.GetLifecycleStates()
			return slices.Contains(got, want[len(want)-1])
		}, testutil.WaitShort, testutil.IntervalMedium)

		require.Equal(t, want, got[:len(want)])
	})

	t.Run("StartError", func(t *testing.T) {
		t.Parallel()

		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			StartupScript:        "false",
			StartupScriptTimeout: 30 * time.Second,
		}, 0)

		want := []codersdk.WorkspaceAgentLifecycle{
			codersdk.WorkspaceAgentLifecycleStarting,
			codersdk.WorkspaceAgentLifecycleStartError,
		}

		var got []codersdk.WorkspaceAgentLifecycle
		assert.Eventually(t, func() bool {
			got = client.GetLifecycleStates()
			return slices.Contains(got, want[len(want)-1])
		}, testutil.WaitShort, testutil.IntervalMedium)

		require.Equal(t, want, got[:len(want)])
	})

	t.Run("Ready", func(t *testing.T) {
		t.Parallel()

		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			StartupScript:        "true",
			StartupScriptTimeout: 30 * time.Second,
		}, 0)

		want := []codersdk.WorkspaceAgentLifecycle{
			codersdk.WorkspaceAgentLifecycleStarting,
			codersdk.WorkspaceAgentLifecycleReady,
		}

		var got []codersdk.WorkspaceAgentLifecycle
		assert.Eventually(t, func() bool {
			got = client.GetLifecycleStates()
			return len(got) > 0 && got[len(got)-1] == want[len(want)-1]
		}, testutil.WaitShort, testutil.IntervalMedium)

		require.Equal(t, want, got)
	})

	t.Run("ShuttingDown", func(t *testing.T) {
		t.Parallel()

		_, client, _, _, closer := setupAgent(t, agentsdk.Manifest{
			ShutdownScript:       "sleep 3",
			StartupScriptTimeout: 30 * time.Second,
		}, 0)

		assert.Eventually(t, func() bool {
			return slices.Contains(client.GetLifecycleStates(), codersdk.WorkspaceAgentLifecycleReady)
		}, testutil.WaitShort, testutil.IntervalMedium)

		// Start close asynchronously so that we an inspect the state.
		done := make(chan struct{})
		go func() {
			defer close(done)
			err := closer.Close()
			assert.NoError(t, err)
		}()
		t.Cleanup(func() {
			<-done
		})

		want := []codersdk.WorkspaceAgentLifecycle{
			codersdk.WorkspaceAgentLifecycleStarting,
			codersdk.WorkspaceAgentLifecycleReady,
			codersdk.WorkspaceAgentLifecycleShuttingDown,
		}

		var got []codersdk.WorkspaceAgentLifecycle
		assert.Eventually(t, func() bool {
			got = client.GetLifecycleStates()
			return slices.Contains(got, want[len(want)-1])
		}, testutil.WaitShort, testutil.IntervalMedium)

		require.Equal(t, want, got[:len(want)])
	})

	t.Run("ShutdownTimeout", func(t *testing.T) {
		t.Parallel()

		_, client, _, _, closer := setupAgent(t, agentsdk.Manifest{
			ShutdownScript:        "sleep 3",
			ShutdownScriptTimeout: time.Nanosecond,
		}, 0)

		assert.Eventually(t, func() bool {
			return slices.Contains(client.GetLifecycleStates(), codersdk.WorkspaceAgentLifecycleReady)
		}, testutil.WaitShort, testutil.IntervalMedium)

		// Start close asynchronously so that we an inspect the state.
		done := make(chan struct{})
		go func() {
			defer close(done)
			err := closer.Close()
			assert.NoError(t, err)
		}()
		t.Cleanup(func() {
			<-done
		})

		want := []codersdk.WorkspaceAgentLifecycle{
			codersdk.WorkspaceAgentLifecycleStarting,
			codersdk.WorkspaceAgentLifecycleReady,
			codersdk.WorkspaceAgentLifecycleShuttingDown,
			codersdk.WorkspaceAgentLifecycleShutdownTimeout,
		}

		var got []codersdk.WorkspaceAgentLifecycle
		assert.Eventually(t, func() bool {
			got = client.GetLifecycleStates()
			return slices.Contains(got, want[len(want)-1])
		}, testutil.WaitShort, testutil.IntervalMedium)

		require.Equal(t, want, got[:len(want)])
	})

	t.Run("ShutdownError", func(t *testing.T) {
		t.Parallel()

		_, client, _, _, closer := setupAgent(t, agentsdk.Manifest{
			ShutdownScript:        "false",
			ShutdownScriptTimeout: 30 * time.Second,
		}, 0)

		assert.Eventually(t, func() bool {
			return slices.Contains(client.GetLifecycleStates(), codersdk.WorkspaceAgentLifecycleReady)
		}, testutil.WaitShort, testutil.IntervalMedium)

		// Start close asynchronously so that we an inspect the state.
		done := make(chan struct{})
		go func() {
			defer close(done)
			err := closer.Close()
			assert.NoError(t, err)
		}()
		t.Cleanup(func() {
			<-done
		})

		want := []codersdk.WorkspaceAgentLifecycle{
			codersdk.WorkspaceAgentLifecycleStarting,
			codersdk.WorkspaceAgentLifecycleReady,
			codersdk.WorkspaceAgentLifecycleShuttingDown,
			codersdk.WorkspaceAgentLifecycleShutdownError,
		}

		var got []codersdk.WorkspaceAgentLifecycle
		assert.Eventually(t, func() bool {
			got = client.GetLifecycleStates()
			return slices.Contains(got, want[len(want)-1])
		}, testutil.WaitShort, testutil.IntervalMedium)

		require.Equal(t, want, got[:len(want)])
	})

	t.Run("ShutdownScriptOnce", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		expected := "this-is-shutdown"
		derpMap, _ := tailnettest.RunDERPAndSTUN(t)

		client := agenttest.NewClient(t,
			logger,
			uuid.New(),
			agentsdk.Manifest{
				DERPMap:        derpMap,
				StartupScript:  "echo 1",
				ShutdownScript: "echo " + expected,
			},
			make(chan *agentsdk.Stats, 50),
			tailnet.NewCoordinator(logger),
		)

		fs := afero.NewMemMapFs()
		agent := agent.New(agent.Options{
			Client:     client,
			Logger:     logger.Named("agent"),
			Filesystem: fs,
		})

		// agent.Close() loads the shutdown script from the agent metadata.
		// The metadata is populated just before execution of the startup script, so it's mandatory to wait
		// until the startup starts.
		require.Eventually(t, func() bool {
			outputPath := filepath.Join(os.TempDir(), "coder-startup-script.log")
			content, err := afero.ReadFile(fs, outputPath)
			if err != nil {
				t.Logf("read file %q: %s", outputPath, err)
				return false
			}
			return len(content) > 0 // something is in the startup log file
		}, testutil.WaitShort, testutil.IntervalMedium)

		err := agent.Close()
		require.NoError(t, err, "agent should be closed successfully")

		outputPath := filepath.Join(os.TempDir(), "coder-shutdown-script.log")
		logFirstRead, err := afero.ReadFile(fs, outputPath)
		require.NoError(t, err, "log file should be present")
		require.Equal(t, expected, string(bytes.TrimSpace(logFirstRead)))

		// Make sure that script can't be executed twice.
		err = agent.Close()
		require.NoError(t, err, "don't need to close the agent twice, no effect")

		logSecondRead, err := afero.ReadFile(fs, outputPath)
		require.NoError(t, err, "log file should be present")
		require.Equal(t, string(bytes.TrimSpace(logFirstRead)), string(bytes.TrimSpace(logSecondRead)))
	})
}

func TestAgent_Startup(t *testing.T) {
	t.Parallel()

	t.Run("EmptyDirectory", func(t *testing.T) {
		t.Parallel()

		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			StartupScript:        "true",
			StartupScriptTimeout: 30 * time.Second,
			Directory:            "",
		}, 0)
		assert.Eventually(t, func() bool {
			return client.GetStartup().Version != ""
		}, testutil.WaitShort, testutil.IntervalFast)
		require.Equal(t, "", client.GetStartup().ExpandedDirectory)
	})

	t.Run("HomeDirectory", func(t *testing.T) {
		t.Parallel()

		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			StartupScript:        "true",
			StartupScriptTimeout: 30 * time.Second,
			Directory:            "~",
		}, 0)
		assert.Eventually(t, func() bool {
			return client.GetStartup().Version != ""
		}, testutil.WaitShort, testutil.IntervalFast)
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		require.Equal(t, homeDir, client.GetStartup().ExpandedDirectory)
	})

	t.Run("NotAbsoluteDirectory", func(t *testing.T) {
		t.Parallel()

		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			StartupScript:        "true",
			StartupScriptTimeout: 30 * time.Second,
			Directory:            "coder/coder",
		}, 0)
		assert.Eventually(t, func() bool {
			return client.GetStartup().Version != ""
		}, testutil.WaitShort, testutil.IntervalFast)
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(homeDir, "coder/coder"), client.GetStartup().ExpandedDirectory)
	})

	t.Run("HomeEnvironmentVariable", func(t *testing.T) {
		t.Parallel()

		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			StartupScript:        "true",
			StartupScriptTimeout: 30 * time.Second,
			Directory:            "$HOME",
		}, 0)
		assert.Eventually(t, func() bool {
			return client.GetStartup().Version != ""
		}, testutil.WaitShort, testutil.IntervalFast)
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		require.Equal(t, homeDir, client.GetStartup().ExpandedDirectory)
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
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
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
			conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
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

// TestAgent_UpdatedDERP checks that agents can handle their DERP map being
// updated, and that clients can also handle it.
func TestAgent_UpdatedDERP(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	originalDerpMap, _ := tailnettest.RunDERPAndSTUN(t)
	require.NotNil(t, originalDerpMap)

	coordinator := tailnet.NewCoordinator(logger)
	defer func() {
		_ = coordinator.Close()
	}()
	agentID := uuid.New()
	statsCh := make(chan *agentsdk.Stats, 50)
	fs := afero.NewMemMapFs()
	client := agenttest.NewClient(t,
		logger.Named("agent"),
		agentID,
		agentsdk.Manifest{
			DERPMap: originalDerpMap,
			// Force DERP.
			DisableDirectConnections: true,
		},
		statsCh,
		coordinator,
	)
	closer := agent.New(agent.Options{
		Client:                 client,
		Filesystem:             fs,
		Logger:                 logger.Named("agent"),
		ReconnectingPTYTimeout: time.Minute,
	})
	defer func() {
		_ = closer.Close()
	}()

	// Setup a client connection.
	newClientConn := func(derpMap *tailcfg.DERPMap) *codersdk.WorkspaceAgentConn {
		conn, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
			DERPMap:   derpMap,
			Logger:    logger.Named("client"),
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
			err := coordinator.ServeClient(serverConn, uuid.New(), agentID)
			assert.NoError(t, err)
		}()
		sendNode, _ := tailnet.ServeCoordinator(clientConn, func(nodes []*tailnet.Node) error {
			return conn.UpdateNodes(nodes, false)
		})
		conn.SetNodeCallback(sendNode)
		// Force DERP.
		conn.SetBlockEndpoints(true)

		sdkConn := codersdk.NewWorkspaceAgentConn(conn, codersdk.WorkspaceAgentConnOptions{
			AgentID:   agentID,
			CloseFunc: func() error { return codersdk.ErrSkipClose },
		})
		t.Cleanup(func() {
			_ = sdkConn.Close()
		})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		if !sdkConn.AwaitReachable(ctx) {
			t.Fatal("agent not reachable")
		}

		return sdkConn
	}
	conn1 := newClientConn(originalDerpMap)

	// Change the DERP map.
	newDerpMap, _ := tailnettest.RunDERPAndSTUN(t)
	require.NotNil(t, newDerpMap)

	// Change the region ID.
	newDerpMap.Regions[2] = newDerpMap.Regions[1]
	delete(newDerpMap.Regions, 1)
	newDerpMap.Regions[2].RegionID = 2
	for _, node := range newDerpMap.Regions[2].Nodes {
		node.RegionID = 2
	}

	// Push a new DERP map to the agent.
	err := client.PushDERPMapUpdate(agentsdk.DERPMapUpdate{
		DERPMap: newDerpMap,
	})
	require.NoError(t, err)

	// Connect from a second client and make sure it uses the new DERP map.
	conn2 := newClientConn(newDerpMap)
	require.Equal(t, []int{2}, conn2.DERPMap().RegionIDs())

	// If the first client gets a DERP map update, it should be able to
	// reconnect just fine.
	conn1.SetDERPMap(newDerpMap)
	require.Equal(t, []int{2}, conn1.DERPMap().RegionIDs())
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	require.True(t, conn1.AwaitReachable(ctx))
}

func TestAgent_Speedtest(t *testing.T) {
	t.Parallel()
	t.Skip("This test is relatively flakey because of Tailscale's speedtest code...")
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	//nolint:dogsled
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{
		DERPMap: derpMap,
	}, 0)
	defer conn.Close()
	res, err := conn.Speedtest(ctx, speedtest.Upload, 250*time.Millisecond)
	require.NoError(t, err)
	t.Logf("%.2f MBits/s", res[len(res)-1].MBitsPerSecond())
}

func TestAgent_Reconnect(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	// After the agent is disconnected from a coordinator, it's supposed
	// to reconnect!
	coordinator := tailnet.NewCoordinator(logger)
	defer coordinator.Close()

	agentID := uuid.New()
	statsCh := make(chan *agentsdk.Stats, 50)
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	client := agenttest.NewClient(t,
		logger,
		agentID,
		agentsdk.Manifest{
			DERPMap: derpMap,
		},
		statsCh,
		coordinator,
	)
	initialized := atomic.Int32{}
	closer := agent.New(agent.Options{
		ExchangeToken: func(ctx context.Context) (string, error) {
			initialized.Add(1)
			return "", nil
		},
		Client: client,
		Logger: logger.Named("agent"),
	})
	defer closer.Close()

	require.Eventually(t, func() bool {
		return coordinator.Node(agentID) != nil
	}, testutil.WaitShort, testutil.IntervalFast)
	client.LastWorkspaceAgent()
	require.Eventually(t, func() bool {
		return initialized.Load() == 2
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestAgent_WriteVSCodeConfigs(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coordinator := tailnet.NewCoordinator(logger)
	defer coordinator.Close()

	client := agenttest.NewClient(t,
		logger,
		uuid.New(),
		agentsdk.Manifest{
			GitAuthConfigs: 1,
			DERPMap:        &tailcfg.DERPMap{},
		},
		make(chan *agentsdk.Stats, 50),
		coordinator,
	)
	filesystem := afero.NewMemMapFs()
	closer := agent.New(agent.Options{
		ExchangeToken: func(ctx context.Context) (string, error) {
			return "", nil
		},
		Client:     client,
		Logger:     logger.Named("agent"),
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

func setupSSHCommand(t *testing.T, beforeArgs []string, afterArgs []string) (*ptytest.PTYCmd, pty.Process) {
	//nolint:dogsled
	agentConn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
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
				agentssh.Bicopy(context.Background(), conn, ssh)
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
	cmd := pty.Command("ssh", args...)
	return ptytest.Start(t, cmd)
}

func setupSSHSession(
	t *testing.T,
	manifest agentsdk.Manifest,
	serviceBanner codersdk.ServiceBannerConfig,
	prepareFS func(fs afero.Fs),
) *ssh.Session {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	//nolint:dogsled
	conn, _, _, fs, _ := setupAgent(t, manifest, 0, func(c *agenttest.Client, _ *agent.Options) {
		c.SetServiceBannerFunc(func() (codersdk.ServiceBannerConfig, error) {
			return serviceBanner, nil
		})
	})
	if prepareFS != nil {
		prepareFS(fs)
	}
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

func setupAgent(t *testing.T, metadata agentsdk.Manifest, ptyTimeout time.Duration, opts ...func(*agenttest.Client, *agent.Options)) (
	*codersdk.WorkspaceAgentConn,
	*agenttest.Client,
	<-chan *agentsdk.Stats,
	afero.Fs,
	io.Closer,
) {
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	if metadata.DERPMap == nil {
		metadata.DERPMap, _ = tailnettest.RunDERPAndSTUN(t)
	}
	if metadata.AgentID == uuid.Nil {
		metadata.AgentID = uuid.New()
	}
	coordinator := tailnet.NewCoordinator(logger)
	t.Cleanup(func() {
		_ = coordinator.Close()
	})
	statsCh := make(chan *agentsdk.Stats, 50)
	fs := afero.NewMemMapFs()
	c := agenttest.NewClient(t, logger.Named("agent"), metadata.AgentID, metadata, statsCh, coordinator)

	options := agent.Options{
		Client:                 c,
		Filesystem:             fs,
		Logger:                 logger.Named("agent"),
		ReconnectingPTYTimeout: ptyTimeout,
	}

	for _, opt := range opts {
		opt(c, &options)
	}

	closer := agent.New(options)
	t.Cleanup(func() {
		_ = closer.Close()
	})
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		DERPMap:   metadata.DERPMap,
		Logger:    logger.Named("client"),
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
		coordinator.ServeClient(serverConn, uuid.New(), metadata.AgentID)
	}()
	sendNode, _ := tailnet.ServeCoordinator(clientConn, func(nodes []*tailnet.Node) error {
		return conn.UpdateNodes(nodes, false)
	})
	conn.SetNodeCallback(sendNode)
	agentConn := codersdk.NewWorkspaceAgentConn(conn, codersdk.WorkspaceAgentConnOptions{
		AgentID: metadata.AgentID,
	})
	t.Cleanup(func() {
		_ = agentConn.Close()
	})
	// Ideally we wouldn't wait too long here, but sometimes the the
	// networking needs more time to resolve itself.
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	if !agentConn.AwaitReachable(ctx) {
		t.Fatal("agent not reachable")
	}
	return agentConn, c, statsCh, fs, closer
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

func testSessionOutput(t *testing.T, session *ssh.Session, expected, unexpected []string, expectedRe *regexp.Regexp) {
	t.Helper()

	err := session.RequestPty("xterm", 128, 128, ssh.TerminalModes{})
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

	for _, unexpected := range unexpected {
		require.NotContains(t, stdout.String(), unexpected, "should not show output")
	}
	for _, expect := range expected {
		require.Contains(t, stdout.String(), expect, "should show output")
	}
	if expectedRe != nil {
		require.Regexp(t, expectedRe, stdout.String())
	}
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

func TestAgent_Metrics_SSH(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	registry := prometheus.NewRegistry()

	//nolint:dogsled
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, o *agent.Options) {
		o.PrometheusRegistry = registry
	})

	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshClient.Close()
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	defer session.Close()
	stdin, err := session.StdinPipe()
	require.NoError(t, err)
	err = session.Shell()
	require.NoError(t, err)

	expected := []agentsdk.AgentMetric{
		{
			Name:  "agent_reconnecting_pty_connections_total",
			Type:  agentsdk.AgentMetricTypeCounter,
			Value: 0,
		},
		{
			Name:  "agent_sessions_total",
			Type:  agentsdk.AgentMetricTypeCounter,
			Value: 1,
			Labels: []agentsdk.AgentMetricLabel{
				{
					Name:  "magic_type",
					Value: "ssh",
				},
				{
					Name:  "pty",
					Value: "no",
				},
			},
		},
		{
			Name:  "agent_ssh_server_failed_connections_total",
			Type:  agentsdk.AgentMetricTypeCounter,
			Value: 0,
		},
		{
			Name:  "agent_ssh_server_sftp_connections_total",
			Type:  agentsdk.AgentMetricTypeCounter,
			Value: 0,
		},
		{
			Name:  "agent_ssh_server_sftp_server_errors_total",
			Type:  agentsdk.AgentMetricTypeCounter,
			Value: 0,
		},
	}

	var actual []*promgo.MetricFamily
	assert.Eventually(t, func() bool {
		actual, err = registry.Gather()
		if err != nil {
			return false
		}

		if len(expected) != len(actual) {
			return false
		}

		return verifyCollectedMetrics(t, expected, actual)
	}, testutil.WaitLong, testutil.IntervalFast)

	require.Len(t, actual, len(expected))
	collected := verifyCollectedMetrics(t, expected, actual)
	require.True(t, collected, "expected metrics were not collected")

	_ = stdin.Close()
	err = session.Wait()
	require.NoError(t, err)
}

func verifyCollectedMetrics(t *testing.T, expected []agentsdk.AgentMetric, actual []*promgo.MetricFamily) bool {
	t.Helper()

	for i, e := range expected {
		assert.Equal(t, e.Name, actual[i].GetName())
		assert.Equal(t, string(e.Type), strings.ToLower(actual[i].GetType().String()))

		for _, m := range actual[i].GetMetric() {
			assert.Equal(t, e.Value, m.Counter.GetValue())

			if len(m.GetLabel()) > 0 {
				for j, lbl := range m.GetLabel() {
					assert.Equal(t, e.Labels[j].Name, lbl.GetName())
					assert.Equal(t, e.Labels[j].Value, lbl.GetValue())
				}
			}
			m.GetLabel()
		}
	}
	return true
}
