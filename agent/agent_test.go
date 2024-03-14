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
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/bramvdbogaerde/go-scp"
	"github.com/google/uuid"
	"github.com/pion/udp"
	"github.com/pkg/sftp"
	"github.com/prometheus/client_golang/prometheus"
	promgo "github.com/prometheus/client_model/go"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/ssh"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
	"tailscale.com/net/speedtest"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/agent/agentproc/agentproctest"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
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

	data, err := json.Marshal(codersdk.ReconnectingPTYRequest{
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
	t.Run("TracksVSCode", func(t *testing.T) {
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
		conn, _, stats, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
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

	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sshClient.Close()
	})

	//nolint:paralleltest // These tests need to swap the banner func.
	for i, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
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

			session, err := sshClient.NewSession()
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = session.Close()
			})

			testSessionOutput(t, session, test.expected, test.unexpected, nil)
		})
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
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		expected := "this-is-shutdown"
		derpMap, _ := tailnettest.RunDERPAndSTUN(t)

		client := agenttest.NewClient(t,
			logger,
			uuid.New(),
			agentsdk.Manifest{
				DERPMap: derpMap,
				Scripts: []codersdk.WorkspaceAgentScript{{
					LogPath:    "coder-startup-script.log",
					Script:     "echo 1",
					RunOnStart: true,
				}, {
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
			conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
			id := uuid.New()
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

			data, err := json.Marshal(codersdk.ReconnectingPTYRequest{
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
			data, err = json.Marshal(codersdk.ReconnectingPTYRequest{
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
				c, err := l.Accept()
				if assert.NoError(t, err, "accept connection") {
					defer c.Close()
					testAccept(ctx, t, c)
				}
			}()

			//nolint:dogsled
			agentConn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
			require.True(t, agentConn.AwaitReachable(ctx))
			conn, err := agentConn.DialContext(ctx, l.Addr().Network(), l.Addr().String())
			require.NoError(t, err)
			defer conn.Close()
			testDial(ctx, t, conn)
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
	newClientConn := func(derpMap *tailcfg.DERPMap, name string) *codersdk.WorkspaceAgentConn {
		conn, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
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
		coordination := tailnet.NewInMemoryCoordination(
			testCtx, logger,
			clientID, agentID,
			coordinator, conn)
		t.Cleanup(func() {
			t.Logf("closing coordination %s", name)
			err := coordination.Close()
			if err != nil {
				t.Logf("error closing in-memory coordination: %s", err.Error())
			}
			t.Logf("closed coordination %s", name)
		})
		// Force DERP.
		conn.SetBlockEndpoints(true)

		sdkConn := codersdk.NewWorkspaceAgentConn(conn, codersdk.WorkspaceAgentConnOptions{
			AgentID:   agentID,
			CloseFunc: func() error { return codersdk.ErrSkipClose },
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
	t.Logf("pushed DERPMap update to agent")

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
	t.Logf("agent got the new DERPMap")

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
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
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

	t.Run("Token", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/debug/token", nil)
		require.NoError(t, err)

		res, err := srv.Client().Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)
		defer res.Body.Close()
		resBody, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, "token", string(resBody))
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
	serviceBanner codersdk.ServiceBannerConfig,
	prepareFS func(fs afero.Fs),
	opts ...func(*agenttest.Client, *agent.Options),
) *ssh.Session {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	opts = append(opts, func(c *agenttest.Client, o *agent.Options) {
		c.SetServiceBannerFunc(func() (codersdk.ServiceBannerConfig, error) {
			return serviceBanner, nil
		})
	})
	//nolint:dogsled
	conn, _, _, fs, _ := setupAgent(t, manifest, 0, opts...)
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
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
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
	coordination := tailnet.NewInMemoryCoordination(
		testCtx, logger,
		clientID, metadata.AgentID,
		coordinator, conn)
	t.Cleanup(func() {
		err := coordination.Close()
		if err != nil {
			t.Logf("error closing in-mem coordination: %s", err.Error())
		}
	})
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
		{
			Name:  "coderd_agentstats_startup_script_seconds",
			Type:  agentsdk.AgentMetricTypeGauge,
			Value: 0,
			Labels: []agentsdk.AgentMetricLabel{
				{
					Name:  "success",
					Value: "true",
				},
			},
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

func TestAgent_ManageProcessPriority(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS != "linux" {
			t.Skip("Skipping non-linux environment")
		}

		var (
			expectedProcs = map[int32]agentproc.Process{}
			fs            = afero.NewMemMapFs()
			syscaller     = agentproctest.NewMockSyscaller(gomock.NewController(t))
			ticker        = make(chan time.Time)
			modProcs      = make(chan []*agentproc.Process)
			logger        = slog.Make(sloghuman.Sink(io.Discard))
		)

		// Create some processes.
		for i := 0; i < 4; i++ {
			// Create a prioritized process. This process should
			// have it's oom_score_adj set to -500 and its nice
			// score should be untouched.
			var proc agentproc.Process
			if i == 0 {
				proc = agentproctest.GenerateProcess(t, fs,
					func(p *agentproc.Process) {
						p.CmdLine = "./coder\x00agent\x00--no-reap"
						p.PID = int32(i)
					},
				)
			} else {
				proc = agentproctest.GenerateProcess(t, fs,
					func(p *agentproc.Process) {
						// Make the cmd something similar to a prioritized
						// process but differentiate the arguments.
						p.CmdLine = "./coder\x00stat"
					},
				)

				syscaller.EXPECT().SetPriority(proc.PID, 10).Return(nil)
				syscaller.EXPECT().GetPriority(proc.PID).Return(20, nil)
			}
			syscaller.EXPECT().
				Kill(proc.PID, syscall.Signal(0)).
				Return(nil)

			expectedProcs[proc.PID] = proc
		}

		_, _, _, _, _ = setupAgent(t, agentsdk.Manifest{}, 0, func(c *agenttest.Client, o *agent.Options) {
			o.Syscaller = syscaller
			o.ModifiedProcesses = modProcs
			o.EnvironmentVariables = map[string]string{agent.EnvProcPrioMgmt: "1"}
			o.Filesystem = fs
			o.Logger = logger
			o.ProcessManagementTick = ticker
		})
		actualProcs := <-modProcs
		require.Len(t, actualProcs, len(expectedProcs)-1)
	})

	t.Run("IgnoreCustomNice", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS != "linux" {
			t.Skip("Skipping non-linux environment")
		}

		var (
			expectedProcs = map[int32]agentproc.Process{}
			fs            = afero.NewMemMapFs()
			ticker        = make(chan time.Time)
			syscaller     = agentproctest.NewMockSyscaller(gomock.NewController(t))
			modProcs      = make(chan []*agentproc.Process)
			logger        = slog.Make(sloghuman.Sink(io.Discard))
		)

		// Create some processes.
		for i := 0; i < 2; i++ {
			proc := agentproctest.GenerateProcess(t, fs)
			syscaller.EXPECT().
				Kill(proc.PID, syscall.Signal(0)).
				Return(nil)

			if i == 0 {
				// Set a random nice score. This one should not be adjusted by
				// our management loop.
				syscaller.EXPECT().GetPriority(proc.PID).Return(25, nil)
			} else {
				syscaller.EXPECT().GetPriority(proc.PID).Return(20, nil)
				syscaller.EXPECT().SetPriority(proc.PID, 10).Return(nil)
			}

			expectedProcs[proc.PID] = proc
		}

		_, _, _, _, _ = setupAgent(t, agentsdk.Manifest{}, 0, func(c *agenttest.Client, o *agent.Options) {
			o.Syscaller = syscaller
			o.ModifiedProcesses = modProcs
			o.EnvironmentVariables = map[string]string{agent.EnvProcPrioMgmt: "1"}
			o.Filesystem = fs
			o.Logger = logger
			o.ProcessManagementTick = ticker
		})
		actualProcs := <-modProcs
		// We should ignore the process with a custom nice score.
		require.Len(t, actualProcs, 1)
	})

	t.Run("DisabledByDefault", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS != "linux" {
			t.Skip("Skipping non-linux environment")
		}

		var (
			buf bytes.Buffer
			wr  = &syncWriter{
				w: &buf,
			}
		)
		log := slog.Make(sloghuman.Sink(wr)).Leveled(slog.LevelDebug)

		_, _, _, _, _ = setupAgent(t, agentsdk.Manifest{}, 0, func(c *agenttest.Client, o *agent.Options) {
			o.Logger = log
		})

		require.Eventually(t, func() bool {
			wr.mu.Lock()
			defer wr.mu.Unlock()
			return strings.Contains(buf.String(), "process priority not enabled")
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("DisabledForNonLinux", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "linux" {
			t.Skip("Skipping linux environment")
		}

		var (
			buf bytes.Buffer
			wr  = &syncWriter{
				w: &buf,
			}
		)
		log := slog.Make(sloghuman.Sink(wr)).Leveled(slog.LevelDebug)

		_, _, _, _, _ = setupAgent(t, agentsdk.Manifest{}, 0, func(c *agenttest.Client, o *agent.Options) {
			o.Logger = log
			// Try to enable it so that we can assert that non-linux
			// environments are truly disabled.
			o.EnvironmentVariables = map[string]string{agent.EnvProcPrioMgmt: "1"}
		})
		require.Eventually(t, func() bool {
			wr.mu.Lock()
			defer wr.mu.Unlock()

			return strings.Contains(buf.String(), "process priority not enabled")
		}, testutil.WaitLong, testutil.IntervalFast)
	})
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

type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
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
