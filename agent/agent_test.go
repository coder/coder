package agent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"go.uber.org/goleak"

	"tailscale.com/net/speedtest"
	"tailscale.com/tailcfg"
	"github.com/bramvdbogaerde/go-scp"
	"github.com/google/uuid"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/pion/udp"
	"github.com/pkg/sftp"
	"github.com/prometheus/client_golang/prometheus"
	promgo "github.com/prometheus/client_model/go"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentssh"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/usershell"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}
var sshPorts = []uint16{workspacesdk.AgentSSHPort, workspacesdk.AgentStandardSSHPort}
// NOTE: These tests only work when your default shell is bash for some reason.
func TestAgent_Stats_SSH(t *testing.T) {

	t.Parallel()
	for _, port := range sshPorts {
		port := port
		t.Run(fmt.Sprintf("(:%d)", port), func(t *testing.T) {

			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)

			defer cancel()
			//nolint:dogsled

			conn, _, stats, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
			sshClient, err := conn.SSHClientOnPort(ctx, port)
			require.NoError(t, err)

			defer sshClient.Close()
			session, err := sshClient.NewSession()
			require.NoError(t, err)
			defer session.Close()
			stdin, err := session.StdinPipe()

			require.NoError(t, err)
			err = session.Shell()
			require.NoError(t, err)

			var s *proto.Stats
			require.Eventuallyf(t, func() bool {
				var ok bool

				s, ok = <-stats
				return ok && s.ConnectionCount > 0 && s.RxBytes > 0 && s.TxBytes > 0 && s.SessionCountSsh == 1
			}, testutil.WaitLong, testutil.IntervalFast,
				"never saw stats: %+v", s,
			)
			_ = stdin.Close()
			err = session.Wait()
			require.NoError(t, err)
		})
	}
}

func TestAgent_Stats_ReconnectingPTY(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	//nolint:dogsled
	conn, _, stats, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
	ptyConn, err := conn.ReconnectingPTY(ctx, uuid.New(), 128, 128, "bash")
	require.NoError(t, err)
	defer ptyConn.Close()
	data, err := json.Marshal(workspacesdk.ReconnectingPTYRequest{
		Data: "echo test\r\n",
	})
	require.NoError(t, err)
	_, err = ptyConn.Write(data)
	require.NoError(t, err)

	var s *proto.Stats
	require.Eventuallyf(t, func() bool {
		var ok bool

		s, ok = <-stats
		return ok && s.ConnectionCount > 0 && s.RxBytes > 0 && s.TxBytes > 0 && s.SessionCountReconnectingPty == 1
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
		session.Setenv(agentssh.MagicSessionTypeEnvironmentVariable, string(agentssh.MagicSessionTypeVSCode))
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
	t.Run("TracksVSCode", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "window" {
			t.Skip("Sleeping for infinity doesn't work on Windows")
		}
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		//nolint:dogsled
		conn, agentClient, stats, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
		sshClient, err := conn.SSHClient(ctx)
		require.NoError(t, err)
		defer sshClient.Close()

		session, err := sshClient.NewSession()
		require.NoError(t, err)
		session.Setenv(agentssh.MagicSessionTypeEnvironmentVariable, string(agentssh.MagicSessionTypeVSCode))
		defer session.Close()
		stdin, err := session.StdinPipe()
		require.NoError(t, err)
		err = session.Shell()
		require.NoError(t, err)
		require.Eventuallyf(t, func() bool {
			s, ok := <-stats
			t.Logf("got stats: ok=%t, ConnectionCount=%d, RxBytes=%d, TxBytes=%d, SessionCountVSCode=%d, ConnectionMedianLatencyMS=%f",
				ok, s.ConnectionCount, s.RxBytes, s.TxBytes, s.SessionCountVscode, s.ConnectionMedianLatencyMs)
			return ok && s.ConnectionCount > 0 && s.RxBytes > 0 && s.TxBytes > 0 &&
				// Ensure that the connection didn't count as a "normal" SSH session.
				// This was a special one, so it should be labeled specially in the stats!
				s.SessionCountVscode == 1 &&
				// Ensure that connection latency is being counted!
				// If it isn't, it's set to -1.
				s.ConnectionMedianLatencyMs >= 0
		}, testutil.WaitLong, testutil.IntervalFast,
			"never saw stats",
		)
		// The shell will automatically exit if there is no stdin!
		_ = stdin.Close()
		err = session.Wait()
		require.NoError(t, err)
		assertConnectionReport(t, agentClient, proto.Connection_VSCODE, 0, "")
	})
	t.Run("TracksJetBrains", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS != "linux" {
			t.Skip("JetBrains tracking is only supported on Linux")
		}
		ctx := testutil.Context(t, testutil.WaitLong)
		// JetBrains tracking works by looking at the process name listening on the
		// forwarded port.  If the process's command line includes the magic string
		// we are looking for, then we assume it is a JetBrains editor.  So when we
		// connect to the port we must ensure the process includes that magic string
		// to fool the agent into thinking this is JetBrains.  To do this we need to
		// spawn an external process (in this case a simple echo server) so we can
		// control the process name.  The -D here is just to mimic how Java options
		// are set but is not necessary as the agent looks only for the magic
		// string itself anywhere in the command.
		_, b, _, ok := runtime.Caller(0)
		require.True(t, ok)
		dir := filepath.Join(filepath.Dir(b), "../scripts/echoserver/main.go")
		echoServerCmd := exec.Command("go", "run", dir,
			"-D", agentssh.MagicProcessCmdlineJetBrains)
		stdout, err := echoServerCmd.StdoutPipe()

		require.NoError(t, err)
		err = echoServerCmd.Start()
		require.NoError(t, err)

		defer echoServerCmd.Process.Kill()
		// The echo server prints its port as the first line.
		sc := bufio.NewScanner(stdout)
		sc.Scan()
		remotePort := sc.Text()
		//nolint:dogsled

		conn, agentClient, stats, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
		sshClient, err := conn.SSHClient(ctx)

		require.NoError(t, err)
		tunneledConn, err := sshClient.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", remotePort))
		require.NoError(t, err)
		t.Cleanup(func() {
			// always close on failure of test
			_ = conn.Close()
			_ = tunneledConn.Close()
		})
		require.Eventuallyf(t, func() bool {
			s, ok := <-stats
			t.Logf("got stats with conn open: ok=%t, ConnectionCount=%d, SessionCountJetBrains=%d",
				ok, s.ConnectionCount, s.SessionCountJetbrains)
			return ok && s.ConnectionCount > 0 &&
				s.SessionCountJetbrains == 1
		}, testutil.WaitLong, testutil.IntervalFast,
			"never saw stats with conn open",
		)
		// Kill the server and connection after checking for the echo.
		requireEcho(t, tunneledConn)
		_ = echoServerCmd.Process.Kill()

		_ = tunneledConn.Close()
		require.Eventuallyf(t, func() bool {
			s, ok := <-stats
			t.Logf("got stats after disconnect %t, %d",
				ok, s.SessionCountJetbrains)

			return ok &&
				s.SessionCountJetbrains == 0
		}, testutil.WaitLong, testutil.IntervalFast,
			"never saw stats after conn closes",
		)

		assertConnectionReport(t, agentClient, proto.Connection_JETBRAINS, 0, "")
	})
}
func TestAgent_SessionExec(t *testing.T) {
	t.Parallel()
	for _, port := range sshPorts {
		port := port
		t.Run(fmt.Sprintf("(:%d)", port), func(t *testing.T) {

			t.Parallel()
			session := setupSSHSessionOnPort(t, agentsdk.Manifest{}, codersdk.ServiceBannerConfig{}, nil, port)
			command := "echo test"
			if runtime.GOOS == "windows" {
				command = "cmd.exe /c echo test"
			}
			output, err := session.Output(command)
			require.NoError(t, err)
			require.Equal(t, "test", strings.TrimSpace(string(output)))
		})

	}
}
//nolint:tparallel // Sub tests need to run sequentially.
func TestAgent_Session_EnvironmentVariables(t *testing.T) {
	t.Parallel()

	tmpdir := t.TempDir()
	// Defined by the coder script runner, hardcoded here since we don't
	// have a reference to it.
	scriptBinDir := filepath.Join(tmpdir, "coder-script-data", "bin")
	manifest := agentsdk.Manifest{
		EnvironmentVariables: map[string]string{
			"MY_MANIFEST":         "true",
			"MY_OVERRIDE":         "false",
			"MY_SESSION_MANIFEST": "false",
		},

	}
	banner := codersdk.ServiceBannerConfig{}
	session := setupSSHSession(t, manifest, banner, nil, func(_ *agenttest.Client, opts *agent.Options) {
		opts.ScriptDataDir = tmpdir

		opts.EnvironmentVariables["MY_OVERRIDE"] = "true"
	})
	err := session.Setenv("MY_SESSION_MANIFEST", "true")

	require.NoError(t, err)
	err = session.Setenv("MY_SESSION", "true")
	require.NoError(t, err)
	command := "sh"
	echoEnv := func(t *testing.T, w io.Writer, env string) {

		if runtime.GOOS == "windows" {
			_, err := fmt.Fprintf(w, "echo %%%s%%\r\n", env)

			require.NoError(t, err)
		} else {
			_, err := fmt.Fprintf(w, "echo $%s\n", env)
			require.NoError(t, err)
		}
	}
	if runtime.GOOS == "windows" {
		command = "cmd.exe"
	}
	stdin, err := session.StdinPipe()
	require.NoError(t, err)

	defer stdin.Close()
	stdout, err := session.StdoutPipe()
	require.NoError(t, err)
	err = session.Start(command)

	require.NoError(t, err)
	// Context is fine here since we're not doing a parallel subtest.

	ctx := testutil.Context(t, testutil.WaitLong)
	go func() {
		<-ctx.Done()
		_ = session.Close()

	}()
	s := bufio.NewScanner(stdout)
	//nolint:paralleltest // These tests need to run sequentially.
	for k, partialV := range map[string]string{
		"CODER":               "true",  // From the agent.
		"MY_MANIFEST":         "true",  // From the manifest.
		"MY_OVERRIDE":         "true",  // From the agent environment variables option, overrides manifest.
		"MY_SESSION_MANIFEST": "false", // From the manifest, overrides session env.
		"MY_SESSION":          "true",  // From the session.
		"PATH":                scriptBinDir + string(filepath.ListSeparator),
	} {
		t.Run(k, func(t *testing.T) {
			echoEnv(t, stdin, k)

			// Windows is unreliable, so keep scanning until we find a match.
			for s.Scan() {
				got := strings.TrimSpace(s.Text())
				t.Logf("%s=%s", k, got)
				if strings.Contains(got, partialV) {

					break
				}
			}
			if err := s.Err(); !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
		})
	}
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
	for _, port := range sshPorts {

		port := port
		t.Run(fmt.Sprintf("(%d)", port), func(t *testing.T) {
			t.Parallel()
			session := setupSSHSessionOnPort(t, agentsdk.Manifest{}, codersdk.ServiceBannerConfig{}, nil, port)
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
		})
	}
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
	require.True(t, errors.As(err, &exitErr))
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
			name: "Trim",
			// Enable motd since it will be printed after the banner,
			// this ensures that we can test for an exact mount of
			// newlines.
			manifest: agentsdk.Manifest{
				MOTDFile: name,
			},
			banner: codersdk.ServiceBannerConfig{
				Enabled: true,
				Message: "\n\n\n\n\n\nbanner\n\n\n\n\n\n",
			},
			expectedRe: regexp.MustCompile(`([^\n\r]|^)banner\r\n\r\n[^\r\n]`),
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
//nolint:tparallel // Sub tests need to run sequentially.
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
	//nolint:paralleltest // These tests need to swap the banner func.
	for _, port := range sshPorts {
		port := port

		sshClient, err := conn.SSHClientOnPort(ctx, port)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sshClient.Close()
		})
		for i, test := range tests {
			test := test
			t.Run(fmt.Sprintf("(:%d)/%d", port, i), func(t *testing.T) {
				// Set new banner func and wait for the agent to call it to update the
				// banner.

				ready := make(chan struct{}, 2)
				client.SetAnnouncementBannersFunc(func() ([]codersdk.BannerConfig, error) {
					select {

					case ready <- struct{}{}:
					default:
					}
					return []codersdk.BannerConfig{test.banner}, nil
				})
				<-ready
				<-ready // Wait for two updates to ensure the value has propagated.
				session, err := sshClient.NewSession()
				require.NoError(t, err)
				t.Cleanup(func() {
					_ = session.Close()
				})
				testSessionOutput(t, session, test.expected, test.unexpected, nil)
			})
		}
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
func TestAgent_TCPLocalForwarding(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	rl, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer rl.Close()
	tcpAddr, valid := rl.Addr().(*net.TCPAddr)
	require.True(t, valid)
	remotePort := tcpAddr.Port
	go echoOnce(t, rl)
	sshClient := setupAgentSSHClient(ctx, t)

	conn, err := sshClient.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", remotePort))
	require.NoError(t, err)

	defer conn.Close()
	requireEcho(t, conn)
}
func TestAgent_TCPRemoteForwarding(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	sshClient := setupAgentSSHClient(ctx, t)
	localhost := netip.MustParseAddr("127.0.0.1")
	var randomPort uint16
	var ll net.Listener
	var err error
	for {

		randomPort = testutil.RandomPortNoListen(t)
		addr := net.TCPAddrFromAddrPort(netip.AddrPortFrom(localhost, randomPort))
		ll, err = sshClient.ListenTCP(addr)
		if err != nil {
			t.Logf("error remote forwarding: %s", err.Error())
			select {

			case <-ctx.Done():
				t.Fatal("timed out getting random listener")
			default:
				continue
			}
		}

		break
	}
	defer ll.Close()
	go echoOnce(t, ll)
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", randomPort))
	require.NoError(t, err)
	defer conn.Close()

	requireEcho(t, conn)
}
func TestAgent_UnixLocalForwarding(t *testing.T) {

	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix domain sockets are not fully supported on Windows")
	}
	ctx := testutil.Context(t, testutil.WaitLong)
	tmpdir := tempDirUnixSocket(t)
	remoteSocketPath := filepath.Join(tmpdir, "remote-socket")
	l, err := net.Listen("unix", remoteSocketPath)
	require.NoError(t, err)
	defer l.Close()
	go echoOnce(t, l)
	sshClient := setupAgentSSHClient(ctx, t)
	conn, err := sshClient.Dial("unix", remoteSocketPath)
	require.NoError(t, err)

	defer conn.Close()
	_, err = conn.Write([]byte("test"))

	require.NoError(t, err)
	b := make([]byte, 4)
	_, err = conn.Read(b)
	require.NoError(t, err)
	require.Equal(t, "test", string(b))
	_ = conn.Close()
}
func TestAgent_UnixRemoteForwarding(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix domain sockets are not fully supported on Windows")

	}
	tmpdir := tempDirUnixSocket(t)
	remoteSocketPath := filepath.Join(tmpdir, "remote-socket")
	ctx := testutil.Context(t, testutil.WaitLong)
	sshClient := setupAgentSSHClient(ctx, t)
	l, err := sshClient.ListenUnix(remoteSocketPath)

	require.NoError(t, err)
	defer l.Close()
	go echoOnce(t, l)
	conn, err := net.Dial("unix", remoteSocketPath)
	require.NoError(t, err)
	defer conn.Close()
	requireEcho(t, conn)

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
	conn, agentClient, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
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
	// Close the client to trigger disconnect event.
	_ = client.Close()
	assertConnectionReport(t, agentClient, proto.Connection_SSH, 0, "")
}
func TestAgent_SCP(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	//nolint:dogsled
	conn, agentClient, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
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
	// Close the client to trigger disconnect event.
	scpClient.Close()
	assertConnectionReport(t, agentClient, proto.Connection_SSH, 0, "")

}
func TestAgent_FileTransferBlocked(t *testing.T) {
	t.Parallel()
	assertFileTransferBlocked := func(t *testing.T, errorMessage string) {
		// NOTE: Checking content of the error message is flaky. Most likely there is a race condition, which results
		// in stopping the client in different phases, and returning different errors:
		// - client read the full error message: File transfer has been disabled.
		// - client's stream was terminated before reading the error message: EOF
		// - client just read the error code (Windows): Process exited with status 65

		isErr := strings.Contains(errorMessage, agentssh.BlockedFileTransferErrorMessage) ||
			strings.Contains(errorMessage, "EOF") ||
			strings.Contains(errorMessage, "Process exited with status 65")
		require.True(t, isErr, "Message: "+errorMessage)
	}

	t.Run("SFTP", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		//nolint:dogsled
		conn, agentClient, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, o *agent.Options) {
			o.BlockFileTransfer = true
		})
		sshClient, err := conn.SSHClient(ctx)
		require.NoError(t, err)
		defer sshClient.Close()
		_, err = sftp.NewClient(sshClient)
		require.Error(t, err)
		assertFileTransferBlocked(t, err.Error())

		assertConnectionReport(t, agentClient, proto.Connection_SSH, agentssh.BlockedFileTransferErrorCode, "")
	})
	t.Run("SCP with go-scp package", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		//nolint:dogsled
		conn, agentClient, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, o *agent.Options) {
			o.BlockFileTransfer = true

		})
		sshClient, err := conn.SSHClient(ctx)
		require.NoError(t, err)

		defer sshClient.Close()
		scpClient, err := scp.NewClientBySSH(sshClient)
		require.NoError(t, err)
		defer scpClient.Close()
		tempFile := filepath.Join(t.TempDir(), "scp")

		err = scpClient.CopyFile(context.Background(), strings.NewReader("hello world"), tempFile, "0755")
		require.Error(t, err)
		assertFileTransferBlocked(t, err.Error())
		assertConnectionReport(t, agentClient, proto.Connection_SSH, agentssh.BlockedFileTransferErrorCode, "")
	})
	t.Run("Forbidden commands", func(t *testing.T) {

		t.Parallel()
		for _, c := range agentssh.BlockedFileTransferCommands {
			t.Run(c, func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
				defer cancel()
				//nolint:dogsled
				conn, agentClient, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, o *agent.Options) {
					o.BlockFileTransfer = true
				})
				sshClient, err := conn.SSHClient(ctx)
				require.NoError(t, err)
				defer sshClient.Close()
				session, err := sshClient.NewSession()
				require.NoError(t, err)
				defer session.Close()
				stdout, err := session.StdoutPipe()
				require.NoError(t, err)
				//nolint:govet // we don't need `c := c` in Go 1.22
				err = session.Start(c)
				require.NoError(t, err)
				defer session.Close()
				msg, err := io.ReadAll(stdout)
				require.NoError(t, err)
				assertFileTransferBlocked(t, string(msg))
				assertConnectionReport(t, agentClient, proto.Connection_SSH, agentssh.BlockedFileTransferErrorCode, "")
			})
		}
	})
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
	for _, key := range []string{"CODER", "CODER_WORKSPACE_NAME", "CODER_WORKSPACE_AGENT_NAME"} {
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
func TestAgent_SSHConnectionLoginVars(t *testing.T) {
	t.Parallel()

	envInfo := usershell.SystemEnvInfo{}
	u, err := envInfo.User()
	require.NoError(t, err, "get current user")
	shell, err := envInfo.Shell(u.Username)
	require.NoError(t, err, "get current shell")
	tests := []struct {
		key  string
		want string
	}{
		{
			key:  "USER",
			want: u.Username,
		},
		{
			key:  "LOGNAME",

			want: u.Username,
		},
		{

			key:  "HOME",
			want: u.HomeDir,
		},

		{
			key:  "SHELL",
			want: shell,
		},

	}
	for _, tt := range tests {
		tt := tt

		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			session := setupSSHSession(t, agentsdk.Manifest{}, codersdk.ServiceBannerConfig{}, nil)
			command := "sh -c 'echo $" + tt.key + "'"
			if runtime.GOOS == "windows" {
				command = "cmd.exe /c echo %" + tt.key + "%"
			}
			output, err := session.Output(command)

			require.NoError(t, err)
			require.Equal(t, tt.want, strings.TrimSpace(string(output)))
		})
	}

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
					Key:      "greeting1",
					Interval: 0,

					Script:   echoHello,
				},
				{
					Key:      "greeting2",
					Interval: 1,
					Script:   echoHello,

				},
			},
		}, 0, func(_ *agenttest.Client, opts *agent.Options) {
			opts.ReportMetadataInterval = testutil.IntervalFast
		})
		var gotMd map[string]agentsdk.Metadata
		require.Eventually(t, func() bool {
			gotMd = client.GetMetadata()
			return len(gotMd) == 2
		}, testutil.WaitShort, testutil.IntervalFast/2)
		collectedAt1 := gotMd["greeting1"].CollectedAt
		collectedAt2 := gotMd["greeting2"].CollectedAt
		require.Eventually(t, func() bool {
			gotMd = client.GetMetadata()
			if len(gotMd) != 2 {
				panic("unexpected number of metadata")
			}
			return !gotMd["greeting2"].CollectedAt.Equal(collectedAt2)

		}, testutil.WaitShort, testutil.IntervalFast/2)
		require.Equal(t, gotMd["greeting1"].CollectedAt, collectedAt1, "metadata should not be collected again")
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
		var gotMd map[string]agentsdk.Metadata
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
			Scripts: []codersdk.WorkspaceAgentScript{{
				Script:     "sleep 3",
				Timeout:    time.Millisecond,
				RunOnStart: true,
			}},
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
			Scripts: []codersdk.WorkspaceAgentScript{{
				Script:     "false",
				Timeout:    30 * time.Second,
				RunOnStart: true,
			}},
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

			Scripts: []codersdk.WorkspaceAgentScript{{
				Script:     "true",
				Timeout:    30 * time.Second,

				RunOnStart: true,
			}},
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
			Scripts: []codersdk.WorkspaceAgentScript{{
				Script:    "sleep 3",
				Timeout:   30 * time.Second,
				RunOnStop: true,
			}},
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
			Scripts: []codersdk.WorkspaceAgentScript{{
				Script:    "sleep 3",
				Timeout:   time.Millisecond,

				RunOnStop: true,
			}},

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
			Scripts: []codersdk.WorkspaceAgentScript{{
				Script:    "false",

				Timeout:   30 * time.Second,
				RunOnStop: true,
			}},

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
		logger := testutil.Logger(t)

		expected := "this-is-shutdown"
		derpMap, _ := tailnettest.RunDERPAndSTUN(t)
		client := agenttest.NewClient(t,

			logger,
			uuid.New(),
			agentsdk.Manifest{

				DERPMap: derpMap,
				Scripts: []codersdk.WorkspaceAgentScript{{
					ID:         uuid.New(),
					LogPath:    "coder-startup-script.log",
					Script:     "echo 1",
					RunOnStart: true,
				}, {
					ID:        uuid.New(),

					LogPath:   "coder-shutdown-script.log",
					Script:    "echo " + expected,
					RunOnStop: true,
				}},
			},

			make(chan *proto.Stats, 50),
			tailnet.NewCoordinator(logger),
		)
		defer client.Close()
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
		ctx := testutil.Context(t, testutil.WaitShort)
		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			Directory: "",
		}, 0)
		startup := testutil.RequireRecvCtx(ctx, t, client.GetStartup())

		require.Equal(t, "", startup.GetExpandedDirectory())
	})
	t.Run("HomeDirectory", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			Directory: "~",
		}, 0)
		startup := testutil.RequireRecvCtx(ctx, t, client.GetStartup())
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		require.Equal(t, homeDir, startup.GetExpandedDirectory())
	})
	t.Run("NotAbsoluteDirectory", func(t *testing.T) {

		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{

			Directory: "coder/coder",
		}, 0)
		startup := testutil.RequireRecvCtx(ctx, t, client.GetStartup())
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(homeDir, "coder/coder"), startup.GetExpandedDirectory())
	})
	t.Run("HomeEnvironmentVariable", func(t *testing.T) {

		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
			Directory: "$HOME",

		}, 0)
		startup := testutil.RequireRecvCtx(ctx, t, client.GetStartup())
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		require.Equal(t, homeDir, startup.GetExpandedDirectory())
	})
}
//nolint:paralleltest // This test sets an environment variable.
func TestAgent_ReconnectingPTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		// This might be our implementation, or ConPTY itself.

		// It's difficult to find extensive tests for it, so
		// it seems like it could be either.
		t.Skip("ConPTY appears to be inconsistent on Windows.")
	}
	backends := []string{"Buffered", "Screen"}
	_, err := exec.LookPath("screen")

	hasScreen := err == nil
	// Make sure UTF-8 works even with LANG set to something like C.
	t.Setenv("LANG", "C")
	for _, backendType := range backends {
		backendType := backendType
		t.Run(backendType, func(t *testing.T) {

			if backendType == "Screen" {
				if runtime.GOOS != "linux" {
					t.Skipf("`screen` is not supported on %s", runtime.GOOS)

				} else if !hasScreen {
					t.Skip("`screen` not found")
				}

			} else if hasScreen && runtime.GOOS == "linux" {
				// Set up a PATH that does not have screen in it.
				bashPath, err := exec.LookPath("bash")
				require.NoError(t, err)
				dir, err := os.MkdirTemp("/tmp", "coder-test-reconnecting-pty-PATH")
				require.NoError(t, err, "create temp dir for reconnecting pty PATH")
				err = os.Symlink(bashPath, filepath.Join(dir, "bash"))
				require.NoError(t, err, "symlink bash into reconnecting pty PATH")

				t.Setenv("PATH", dir)
			}
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			//nolint:dogsled
			conn, agentClient, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
			id := uuid.New()
			// Test that the connection is reported. This must be tested in the
			// first connection because we care about verifying all of these.
			netConn0, err := conn.ReconnectingPTY(ctx, id, 80, 80, "bash --norc")
			require.NoError(t, err)
			_ = netConn0.Close()
			assertConnectionReport(t, agentClient, proto.Connection_RECONNECTING_PTY, 0, "")
			// --norc disables executing .bashrc, which is often used to customize the bash prompt
			netConn1, err := conn.ReconnectingPTY(ctx, id, 80, 80, "bash --norc")

			require.NoError(t, err)
			defer netConn1.Close()
			tr1 := testutil.NewTerminalReader(t, netConn1)
			// A second simultaneous connection.
			netConn2, err := conn.ReconnectingPTY(ctx, id, 80, 80, "bash --norc")
			require.NoError(t, err)
			defer netConn2.Close()

			tr2 := testutil.NewTerminalReader(t, netConn2)
			matchPrompt := func(line string) bool {
				return strings.Contains(line, "$ ") || strings.Contains(line, "# ")
			}
			matchEchoCommand := func(line string) bool {
				return strings.Contains(line, "echo test")

			}
			matchEchoOutput := func(line string) bool {
				return strings.Contains(line, "test") && !strings.Contains(line, "echo")

			}
			matchExitCommand := func(line string) bool {
				return strings.Contains(line, "exit")

			}
			matchExitOutput := func(line string) bool {
				return strings.Contains(line, "exit") || strings.Contains(line, "logout")
			}
			// Wait for the prompt before writing commands.  If the command arrives before the prompt is written, screen
			// will sometimes put the command output on the same line as the command and the test will flake
			require.NoError(t, tr1.ReadUntil(ctx, matchPrompt), "find prompt")
			require.NoError(t, tr2.ReadUntil(ctx, matchPrompt), "find prompt")

			data, err := json.Marshal(workspacesdk.ReconnectingPTYRequest{
				Data: "echo test\r",
			})
			require.NoError(t, err)

			_, err = netConn1.Write(data)
			require.NoError(t, err)
			// Once for typing the command...
			require.NoError(t, tr1.ReadUntil(ctx, matchEchoCommand), "find echo command")
			// And another time for the actual output.
			require.NoError(t, tr1.ReadUntil(ctx, matchEchoOutput), "find echo output")
			// Same for the other connection.
			require.NoError(t, tr2.ReadUntil(ctx, matchEchoCommand), "find echo command")
			require.NoError(t, tr2.ReadUntil(ctx, matchEchoOutput), "find echo output")
			_ = netConn1.Close()
			_ = netConn2.Close()

			netConn3, err := conn.ReconnectingPTY(ctx, id, 80, 80, "bash --norc")
			require.NoError(t, err)
			defer netConn3.Close()
			tr3 := testutil.NewTerminalReader(t, netConn3)
			// Same output again!
			require.NoError(t, tr3.ReadUntil(ctx, matchEchoCommand), "find echo command")
			require.NoError(t, tr3.ReadUntil(ctx, matchEchoOutput), "find echo output")

			// Exit should cause the connection to close.
			data, err = json.Marshal(workspacesdk.ReconnectingPTYRequest{
				Data: "exit\r",
			})
			require.NoError(t, err)
			_, err = netConn3.Write(data)

			require.NoError(t, err)
			// Once for the input and again for the output.
			require.NoError(t, tr3.ReadUntil(ctx, matchExitCommand), "find exit command")

			require.NoError(t, tr3.ReadUntil(ctx, matchExitOutput), "find exit output")
			// Wait for the connection to close.
			require.ErrorIs(t, tr3.ReadUntil(ctx, nil), io.EOF)
			// Try a non-shell command.  It should output then immediately exit.
			netConn4, err := conn.ReconnectingPTY(ctx, uuid.New(), 80, 80, "echo test")
			require.NoError(t, err)

			defer netConn4.Close()
			tr4 := testutil.NewTerminalReader(t, netConn4)
			require.NoError(t, tr4.ReadUntil(ctx, matchEchoOutput), "find echo output")
			require.ErrorIs(t, tr4.ReadUntil(ctx, nil), io.EOF)
			// Ensure that UTF-8 is supported.  Avoid the terminal emulator because it
			// does not appear to support UTF-8, just make sure the bytes that come
			// back have the character in it.
			netConn5, err := conn.ReconnectingPTY(ctx, uuid.New(), 80, 80, "echo ❯")
			require.NoError(t, err)
			defer netConn5.Close()
			bytes, err := io.ReadAll(netConn5)
			require.NoError(t, err)
			require.Contains(t, string(bytes), "❯")
		})
	}
}
// This tests end-to-end functionality of connecting to a running container
// and executing a command. It creates a real Docker container and runs a
// command. As such, it does not run by default in CI.
// You can run it manually as follows:
//
// CODER_TEST_USE_DOCKER=1 go test -count=1 ./agent -run TestAgent_ReconnectingPTYContainer

func TestAgent_ReconnectingPTYContainer(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_TEST_USE_DOCKER") != "1" {
		t.Skip("Set CODER_TEST_USE_DOCKER=1 to run this test")
	}
	ctx := testutil.Context(t, testutil.WaitLong)
	pool, err := dockertest.NewPool("")

	require.NoError(t, err, "Could not connect to docker")
	ct, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "busybox",
		Tag:        "latest",
		Cmd:        []string{"sleep", "infnity"},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err, "Could not start container")
	t.Cleanup(func() {
		err := pool.Purge(ct)
		require.NoError(t, err, "Could not stop container")

	})
	// Wait for container to start
	require.Eventually(t, func() bool {

		ct, ok := pool.ContainerByName(ct.Container.Name)
		return ok && ct.Container.State.Running
	}, testutil.WaitShort, testutil.IntervalSlow, "Container did not start in time")
	// nolint: dogsled
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, o *agent.Options) {

		o.ExperimentalDevcontainersEnabled = true
	})
	ac, err := conn.ReconnectingPTY(ctx, uuid.New(), 80, 80, "/bin/sh", func(arp *workspacesdk.AgentReconnectingPTYInit) {
		arp.Container = ct.Container.ID

	})
	require.NoError(t, err, "failed to create ReconnectingPTY")
	defer ac.Close()
	tr := testutil.NewTerminalReader(t, ac)
	require.NoError(t, tr.ReadUntil(ctx, func(line string) bool {
		return strings.Contains(line, "#") || strings.Contains(line, "$")

	}), "find prompt")
	require.NoError(t, json.NewEncoder(ac).Encode(workspacesdk.ReconnectingPTYRequest{
		Data: "hostname\r",

	}), "write hostname")
	require.NoError(t, tr.ReadUntil(ctx, func(line string) bool {
		return strings.Contains(line, "hostname")
	}), "find hostname command")

	require.NoError(t, tr.ReadUntil(ctx, func(line string) bool {
		return strings.Contains(line, ct.Container.Config.Hostname)
	}), "find hostname output")
	require.NoError(t, json.NewEncoder(ac).Encode(workspacesdk.ReconnectingPTYRequest{
		Data: "exit\r",
	}), "write exit command")
	// Wait for the connection to close.

	require.ErrorIs(t, tr.ReadUntil(ctx, nil), io.EOF)
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
			// The purpose of this test is to ensure that a client can dial a
			// listener in the workspace over tailnet.
			l := c.setup(t)
			done := make(chan struct{})
			defer func() {
				l.Close()
				<-done

			}()
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()
			go func() {
				defer close(done)
				for range 2 {
					c, err := l.Accept()
					if assert.NoError(t, err, "accept connection") {
						testAccept(ctx, t, c)

						_ = c.Close()
					}

				}
			}()
			agentID := uuid.UUID{0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}

			//nolint:dogsled
			agentConn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{
				AgentID: agentID,

			}, 0)
			require.True(t, agentConn.AwaitReachable(ctx))
			conn, err := agentConn.DialContext(ctx, l.Addr().Network(), l.Addr().String())
			require.NoError(t, err)
			testDial(ctx, t, conn)
			err = conn.Close()
			require.NoError(t, err)
			// also connect via the CoderServicePrefix, to test that we can reach the agent on this
			// IP. This will be required for CoderVPN.
			_, rawPort, _ := net.SplitHostPort(l.Addr().String())
			port, _ := strconv.ParseUint(rawPort, 10, 16)
			ipp := netip.AddrPortFrom(tailnet.CoderServicePrefix.AddrFromUUID(agentID), uint16(port))
			switch l.Addr().Network() {
			case "tcp":
				conn, err = agentConn.Conn.DialContextTCP(ctx, ipp)
			case "udp":
				conn, err = agentConn.Conn.DialContextUDP(ctx, ipp)
			default:
				t.Fatalf("unknown network: %s", l.Addr().Network())
			}

			require.NoError(t, err)
			testDial(ctx, t, conn)
			err = conn.Close()

			require.NoError(t, err)
		})
	}
}

// TestAgent_UpdatedDERP checks that agents can handle their DERP map being
// updated, and that clients can also handle it.
func TestAgent_UpdatedDERP(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t)
	originalDerpMap, _ := tailnettest.RunDERPAndSTUN(t)
	require.NotNil(t, originalDerpMap)

	coordinator := tailnet.NewCoordinator(logger)
	// use t.Cleanup so the coordinator closing doesn't deadlock with in-memory
	// coordination
	t.Cleanup(func() {
		_ = coordinator.Close()
	})

	agentID := uuid.New()
	statsCh := make(chan *proto.Stats, 50)
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
	t.Cleanup(func() {
		t.Log("closing client")
		client.Close()
	})
	uut := agent.New(agent.Options{
		Client:                 client,
		Filesystem:             fs,
		Logger:                 logger.Named("agent"),

		ReconnectingPTYTimeout: time.Minute,
	})
	t.Cleanup(func() {
		t.Log("closing agent")
		_ = uut.Close()

	})
	// Setup a client connection.
	newClientConn := func(derpMap *tailcfg.DERPMap, name string) *workspacesdk.AgentConn {
		conn, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
			DERPMap:   derpMap,
			Logger:    logger.Named(name),

		})
		require.NoError(t, err)
		t.Cleanup(func() {
			t.Logf("closing conn %s", name)
			_ = conn.Close()

		})
		testCtx, testCtxCancel := context.WithCancel(context.Background())
		t.Cleanup(testCtxCancel)
		clientID := uuid.New()

		ctrl := tailnet.NewTunnelSrcCoordController(logger, conn)
		ctrl.AddDestination(agentID)
		auth := tailnet.ClientCoordinateeAuth{AgentID: agentID}
		coordination := ctrl.New(tailnet.NewInMemoryCoordinatorClient(logger, clientID, auth, coordinator))
		t.Cleanup(func() {
			t.Logf("closing coordination %s", name)
			cctx, ccancel := context.WithTimeout(testCtx, testutil.WaitShort)

			defer ccancel()
			err := coordination.Close(cctx)
			if err != nil {
				t.Logf("error closing in-memory coordination: %s", err.Error())

			}
			t.Logf("closed coordination %s", name)
		})
		// Force DERP.
		conn.SetBlockEndpoints(true)
		sdkConn := workspacesdk.NewAgentConn(conn, workspacesdk.AgentConnOptions{
			AgentID:   agentID,
			CloseFunc: func() error { return workspacesdk.ErrSkipClose },

		})
		t.Cleanup(func() {
			t.Logf("closing sdkConn %s", name)
			_ = sdkConn.Close()

		})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		if !sdkConn.AwaitReachable(ctx) {
			t.Fatal("agent not reachable")
		}
		return sdkConn
	}

	conn1 := newClientConn(originalDerpMap, "client1")
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
	err := client.PushDERPMapUpdate(newDerpMap)
	require.NoError(t, err)
	t.Log("pushed DERPMap update to agent")
	require.Eventually(t, func() bool {
		conn := uut.TailnetConn()
		if conn == nil {

			return false
		}
		regionIDs := conn.DERPMap().RegionIDs()
		preferredDERP := conn.Node().PreferredDERP
		t.Logf("agent Conn DERPMap with regionIDs %v, PreferredDERP %d", regionIDs, preferredDERP)
		return len(regionIDs) == 1 && regionIDs[0] == 2 && preferredDERP == 2
	}, testutil.WaitLong, testutil.IntervalFast)
	t.Log("agent got the new DERPMap")
	// Connect from a second client and make sure it uses the new DERP map.
	conn2 := newClientConn(newDerpMap, "client2")
	require.Equal(t, []int{2}, conn2.DERPMap().RegionIDs())
	t.Log("conn2 got the new DERPMap")

	// If the first client gets a DERP map update, it should be able to
	// reconnect just fine.

	conn1.SetDERPMap(newDerpMap)
	require.Equal(t, []int{2}, conn1.DERPMap().RegionIDs())
	t.Log("set the new DERPMap on conn1")
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	require.True(t, conn1.AwaitReachable(ctx))
	t.Log("conn1 reached agent with new DERP")
}
func TestAgent_Speedtest(t *testing.T) {
	t.Parallel()
	t.Skip("This test is relatively flakey because of Tailscale's speedtest code...")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	//nolint:dogsled
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{
		DERPMap: derpMap,
	}, 0, func(client *agenttest.Client, options *agent.Options) {
		options.Logger = logger.Named("agent")
	})

	defer conn.Close()
	res, err := conn.Speedtest(ctx, speedtest.Upload, 250*time.Millisecond)
	require.NoError(t, err)
	t.Logf("%.2f MBits/s", res[len(res)-1].MBitsPerSecond())
}
func TestAgent_Reconnect(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t)
	// After the agent is disconnected from a coordinator, it's supposed
	// to reconnect!
	coordinator := tailnet.NewCoordinator(logger)

	defer coordinator.Close()
	agentID := uuid.New()
	statsCh := make(chan *proto.Stats, 50)
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
	defer client.Close()
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
	logger := testutil.Logger(t)
	coordinator := tailnet.NewCoordinator(logger)
	defer coordinator.Close()
	client := agenttest.NewClient(t,
		logger,
		uuid.New(),
		agentsdk.Manifest{
			GitAuthConfigs: 1,
			DERPMap:        &tailcfg.DERPMap{},
		},
		make(chan *proto.Stats, 50),
		coordinator,
	)
	defer client.Close()
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
func TestAgent_DebugServer(t *testing.T) {
	t.Parallel()

	logDir := t.TempDir()
	logPath := filepath.Join(logDir, "coder-agent.log")
	randLogStr, err := cryptorand.String(32)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(logPath, []byte(randLogStr), 0o600))
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	//nolint:dogsled
	conn, _, _, _, agnt := setupAgent(t, agentsdk.Manifest{
		DERPMap: derpMap,
	}, 0, func(c *agenttest.Client, o *agent.Options) {
		o.ExchangeToken = func(context.Context) (string, error) {

			return "token", nil
		}
		o.LogDir = logDir
	})
	awaitReachableCtx := testutil.Context(t, testutil.WaitLong)
	ok := conn.AwaitReachable(awaitReachableCtx)
	require.True(t, ok)
	_ = conn.Close()
	srv := httptest.NewServer(agnt.HTTPDebug())
	t.Cleanup(srv.Close)
	t.Run("MagicsockDebug", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/debug/magicsock", nil)
		require.NoError(t, err)
		res, err := srv.Client().Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		require.Equal(t, http.StatusOK, res.StatusCode)
		resBody, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Contains(t, string(resBody), "<h1>magicsock</h1>")
	})
	t.Run("MagicsockDebugLogging", func(t *testing.T) {
		t.Parallel()
		t.Run("Enable", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/debug/magicsock/debug-logging/t", nil)
			require.NoError(t, err)
			res, err := srv.Client().Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)

			resBody, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Contains(t, string(resBody), "updated magicsock debug logging to true")
		})
		t.Run("Disable", func(t *testing.T) {

			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/debug/magicsock/debug-logging/0", nil)
			require.NoError(t, err)
			res, err := srv.Client().Do(req)

			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)
			resBody, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Contains(t, string(resBody), "updated magicsock debug logging to false")
		})
		t.Run("Invalid", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/debug/magicsock/debug-logging/blah", nil)
			require.NoError(t, err)
			res, err := srv.Client().Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusBadRequest, res.StatusCode)
			resBody, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Contains(t, string(resBody), `invalid state "blah", must be a boolean`)
		})
	})
	t.Run("Manifest", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/debug/manifest", nil)
		require.NoError(t, err)
		res, err := srv.Client().Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		var v agentsdk.Manifest
		require.NoError(t, json.NewDecoder(res.Body).Decode(&v))
		require.NotNil(t, v)
	})
	t.Run("Logs", func(t *testing.T) {

		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/debug/logs", nil)
		require.NoError(t, err)
		res, err := srv.Client().Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)
		defer res.Body.Close()
		resBody, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NotEmpty(t, string(resBody))
		require.Contains(t, string(resBody), randLogStr)
	})
}
func TestAgent_ScriptLogging(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash scripts only")
	}
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitMedium)
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	logsCh := make(chan *proto.BatchCreateLogsRequest, 100)
	lsStart := uuid.UUID{0x11}
	lsStop := uuid.UUID{0x22}
	//nolint:dogsled
	_, _, _, _, agnt := setupAgent(
		t,
		agentsdk.Manifest{
			DERPMap: derpMap,
			Scripts: []codersdk.WorkspaceAgentScript{
				{
					LogSourceID: lsStart,

					RunOnStart:  true,
					Script: `#!/bin/sh
i=0
while [ $i -ne 5 ]
do
        i=$(($i+1))
        echo "start $i"
done
`,
				},
				{
					LogSourceID: lsStop,
					RunOnStop:   true,
					Script: `#!/bin/sh

i=0
while [ $i -ne 3000 ]
do
        i=$(($i+1))

        echo "stop $i"
done
`, // send a lot of stop logs to make sure we don't truncate shutdown logs before closing the API conn
				},

			},
		},
		0,
		func(cl *agenttest.Client, _ *agent.Options) {
			cl.SetLogsChannel(logsCh)
		},
	)
	n := 1

	for n <= 5 {
		logs := testutil.RequireRecvCtx(ctx, t, logsCh)
		require.NotNil(t, logs)
		for _, l := range logs.GetLogs() {
			require.Equal(t, fmt.Sprintf("start %d", n), l.GetOutput())

			n++
		}
	}
	err := agnt.Close()
	require.NoError(t, err)
	n = 1
	for n <= 3000 {
		logs := testutil.RequireRecvCtx(ctx, t, logsCh)
		require.NotNil(t, logs)
		for _, l := range logs.GetLogs() {
			require.Equal(t, fmt.Sprintf("stop %d", n), l.GetOutput())
			n++

		}
		t.Logf("got %d stop logs", n-1)
	}
}
// setupAgentSSHClient creates an agent, dials it, and sets up an ssh.Client for it

func setupAgentSSHClient(ctx context.Context, t *testing.T) *ssh.Client {
	//nolint: dogsled
	agentConn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
	sshClient, err := agentConn.SSHClient(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { sshClient.Close() })
	return sshClient
}
func setupSSHSession(
	t *testing.T,
	manifest agentsdk.Manifest,

	banner codersdk.BannerConfig,
	prepareFS func(fs afero.Fs),
	opts ...func(*agenttest.Client, *agent.Options),
) *ssh.Session {
	return setupSSHSessionOnPort(t, manifest, banner, prepareFS, workspacesdk.AgentSSHPort, opts...)
}
func setupSSHSessionOnPort(
	t *testing.T,
	manifest agentsdk.Manifest,
	banner codersdk.BannerConfig,
	prepareFS func(fs afero.Fs),
	port uint16,
	opts ...func(*agenttest.Client, *agent.Options),
) *ssh.Session {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	opts = append(opts, func(c *agenttest.Client, o *agent.Options) {
		c.SetAnnouncementBannersFunc(func() ([]codersdk.BannerConfig, error) {
			return []codersdk.BannerConfig{banner}, nil

		})
	})
	//nolint:dogsled
	conn, _, _, fs, _ := setupAgent(t, manifest, 0, opts...)
	if prepareFS != nil {
		prepareFS(fs)
	}
	sshClient, err := conn.SSHClientOnPort(ctx, port)

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
	*workspacesdk.AgentConn,
	*agenttest.Client,
	<-chan *proto.Stats,
	afero.Fs,
	agent.Agent,
) {
	logger := slogtest.Make(t, &slogtest.Options{
		// Agent can drop errors when shutting down, and some, like the
		// fasthttplistener connection closed error, are unexported.
		IgnoreErrors: true,
	}).Leveled(slog.LevelDebug)
	if metadata.DERPMap == nil {

		metadata.DERPMap, _ = tailnettest.RunDERPAndSTUN(t)
	}
	if metadata.AgentID == uuid.Nil {
		metadata.AgentID = uuid.New()
	}
	if metadata.AgentName == "" {
		metadata.AgentName = "test-agent"
	}
	if metadata.WorkspaceName == "" {

		metadata.WorkspaceName = "test-workspace"
	}
	if metadata.WorkspaceID == uuid.Nil {
		metadata.WorkspaceID = uuid.New()
	}
	coordinator := tailnet.NewCoordinator(logger)

	t.Cleanup(func() {
		_ = coordinator.Close()
	})
	statsCh := make(chan *proto.Stats, 50)
	fs := afero.NewMemMapFs()
	c := agenttest.NewClient(t, logger.Named("agenttest"), metadata.AgentID, metadata, statsCh, coordinator)
	t.Cleanup(c.Close)
	options := agent.Options{
		Client:                 c,
		Filesystem:             fs,
		Logger:                 logger.Named("agent"),
		ReconnectingPTYTimeout: ptyTimeout,
		EnvironmentVariables:   map[string]string{},
	}
	for _, opt := range opts {
		opt(c, &options)
	}
	agnt := agent.New(options)
	t.Cleanup(func() {
		_ = agnt.Close()
	})
	conn, err := tailnet.NewConn(&tailnet.Options{

		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.TailscaleServicePrefix.RandomAddr(), 128)},
		DERPMap:   metadata.DERPMap,
		Logger:    logger.Named("client"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})
	testCtx, testCtxCancel := context.WithCancel(context.Background())

	t.Cleanup(testCtxCancel)
	clientID := uuid.New()
	ctrl := tailnet.NewTunnelSrcCoordController(logger, conn)

	ctrl.AddDestination(metadata.AgentID)
	auth := tailnet.ClientCoordinateeAuth{AgentID: metadata.AgentID}
	coordination := ctrl.New(tailnet.NewInMemoryCoordinatorClient(
		logger, clientID, auth, coordinator))
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(testCtx, testutil.WaitShort)
		defer ccancel()
		err := coordination.Close(cctx)
		if err != nil {
			t.Logf("error closing in-mem coordination: %s", err.Error())
		}
	})
	agentConn := workspacesdk.NewAgentConn(conn, workspacesdk.AgentConnOptions{
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
	return agentConn, c, statsCh, fs, agnt
}

var dialTestPayload = []byte("dean-was-here123")
func testDial(ctx context.Context, t *testing.T, c net.Conn) {
	t.Helper()
	if deadline, ok := ctx.Deadline(); ok {

		err := c.SetDeadline(deadline)
		assert.NoError(t, err)
		defer func() {
			err := c.SetDeadline(time.Time{})
			assert.NoError(t, err)

		}()
	}
	assertWritePayload(t, c, dialTestPayload)
	assertReadPayload(t, c, dialTestPayload)
}

func testAccept(ctx context.Context, t *testing.T, c net.Conn) {
	t.Helper()
	defer c.Close()

	if deadline, ok := ctx.Deadline(); ok {
		err := c.SetDeadline(deadline)
		assert.NoError(t, err)

		defer func() {
			err := c.SetDeadline(time.Time{})
			assert.NoError(t, err)
		}()

	}
	assertReadPayload(t, c, dialTestPayload)
	assertWritePayload(t, c, dialTestPayload)
}
func assertReadPayload(t *testing.T, r io.Reader, payload []byte) {

	t.Helper()
	b := make([]byte, len(payload)+16)
	n, err := r.Read(b)
	assert.NoError(t, err, "read payload")
	assert.Equal(t, len(payload), n, "read payload length does not match")

	assert.Equal(t, payload, b[:n])
}
func assertWritePayload(t *testing.T, w io.Writer, payload []byte) {

	t.Helper()
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
	expected := []*proto.Stats_Metric{
		{
			Name:  "agent_reconnecting_pty_connections_total",
			Type:  proto.Stats_Metric_COUNTER,
			Value: 0,
		},

		{
			Name:  "agent_sessions_total",
			Type:  proto.Stats_Metric_COUNTER,
			Value: 1,
			Labels: []*proto.Stats_Metric_Label{
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
			Type:  proto.Stats_Metric_COUNTER,
			Value: 0,
		},
		{
			Name:  "agent_ssh_server_sftp_connections_total",
			Type:  proto.Stats_Metric_COUNTER,
			Value: 0,
		},
		{
			Name:  "agent_ssh_server_sftp_server_errors_total",
			Type:  proto.Stats_Metric_COUNTER,
			Value: 0,
		},
		{
			Name:  "coderd_agentstats_currently_reachable_peers",
			Type:  proto.Stats_Metric_GAUGE,
			Value: 0,
			Labels: []*proto.Stats_Metric_Label{
				{
					Name:  "connection_type",
					Value: "derp",
				},
			},
		},
		{

			Name:  "coderd_agentstats_currently_reachable_peers",
			Type:  proto.Stats_Metric_GAUGE,
			Value: 1,
			Labels: []*proto.Stats_Metric_Label{
				{
					Name:  "connection_type",
					Value: "p2p",
				},
			},
		},

		{
			Name:  "coderd_agentstats_startup_script_seconds",
			Type:  proto.Stats_Metric_GAUGE,

			Value: 1,
		},
	}
	var actual []*promgo.MetricFamily
	assert.Eventually(t, func() bool {
		actual, err = registry.Gather()
		if err != nil {
			return false
		}
		count := 0
		for _, m := range actual {
			count += len(m.GetMetric())

		}
		return count == len(expected)
	}, testutil.WaitLong, testutil.IntervalFast)
	i := 0
	for _, mf := range actual {
		for _, m := range mf.GetMetric() {
			assert.Equal(t, expected[i].Name, mf.GetName())
			assert.Equal(t, expected[i].Type.String(), mf.GetType().String())
			// Value is max expected
			if expected[i].Type == proto.Stats_Metric_GAUGE {

				assert.GreaterOrEqualf(t, expected[i].Value, m.GetGauge().GetValue(), "expected %s to be greater than or equal to %f, got %f", expected[i].Name, expected[i].Value, m.GetGauge().GetValue())
			} else if expected[i].Type == proto.Stats_Metric_COUNTER {
				assert.GreaterOrEqualf(t, expected[i].Value, m.GetCounter().GetValue(), "expected %s to be greater than or equal to %f, got %f", expected[i].Name, expected[i].Value, m.GetCounter().GetValue())
			}
			for j, lbl := range expected[i].Labels {
				assert.Equal(t, m.GetLabel()[j], &promgo.LabelPair{
					Name:  &lbl.Name,
					Value: &lbl.Value,
				})
			}

			i++
		}
	}
	_ = stdin.Close()
	err = session.Wait()
	require.NoError(t, err)
}
// echoOnce accepts a single connection, reads 4 bytes and echos them back
func echoOnce(t *testing.T, ll net.Listener) {
	t.Helper()
	conn, err := ll.Accept()
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
}
// requireEcho sends 4 bytes and requires the read response to match what was sent.
func requireEcho(t *testing.T, conn net.Conn) {
	t.Helper()
	_, err := conn.Write([]byte("test"))
	require.NoError(t, err)
	b := make([]byte, 4)
	_, err = conn.Read(b)
	require.NoError(t, err)

	require.Equal(t, "test", string(b))
}
func assertConnectionReport(t testing.TB, agentClient *agenttest.Client, connectionType proto.Connection_Type, status int, reason string) {
	t.Helper()
	var reports []*proto.ReportConnectionRequest
	if !assert.Eventually(t, func() bool {
		reports = agentClient.GetConnectionReports()
		return len(reports) >= 2
	}, testutil.WaitMedium, testutil.IntervalFast, "waiting for 2 connection reports or more; got %d", len(reports)) {
		return
	}
	assert.Len(t, reports, 2, "want 2 connection reports")
	assert.Equal(t, proto.Connection_CONNECT, reports[0].GetConnection().GetAction(), "first report should be connect")
	assert.Equal(t, proto.Connection_DISCONNECT, reports[1].GetConnection().GetAction(), "second report should be disconnect")
	assert.Equal(t, connectionType, reports[0].GetConnection().GetType(), "connect type should be %s", connectionType)
	assert.Equal(t, connectionType, reports[1].GetConnection().GetType(), "disconnect type should be %s", connectionType)
	t1 := reports[0].GetConnection().GetTimestamp().AsTime()
	t2 := reports[1].GetConnection().GetTimestamp().AsTime()
	assert.True(t, t1.Before(t2) || t1.Equal(t2), "connect timestamp should be before or equal to disconnect timestamp")
	assert.NotEmpty(t, reports[0].GetConnection().GetIp(), "connect ip should not be empty")
	assert.NotEmpty(t, reports[1].GetConnection().GetIp(), "disconnect ip should not be empty")
	assert.Equal(t, 0, int(reports[0].GetConnection().GetStatusCode()), "connect status code should be 0")
	assert.Equal(t, status, int(reports[1].GetConnection().GetStatusCode()), "disconnect status code should be %d", status)
	assert.Equal(t, "", reports[0].GetConnection().GetReason(), "connect reason should be empty")
	if reason != "" {
		assert.Contains(t, reports[1].GetConnection().GetReason(), reason, "disconnect reason should contain %s", reason)
	} else {
		t.Logf("connection report disconnect reason: %s", reports[1].GetConnection().GetReason())
	}
}
