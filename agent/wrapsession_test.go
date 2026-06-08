package agent_test

import (
	"io"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// driverSSHClient builds an agent whose sessions run the deterministic test
// shell, and returns an SSH client. Each test opens its own session from it.
func driverSSHClient(t *testing.T, opts ...func(*agenttest.Client, *agent.Options)) *ssh.Client {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	opts = append([]func(*agenttest.Client, *agent.Options){
		func(_ *agenttest.Client, o *agent.Options) {
			o.EnvInfo = agenttest.ShellEnvInfo(t)
		},
	}, opts...)
	//nolint:dogsled
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0, opts...)
	client, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func driverSession(t *testing.T, client *ssh.Client) *ssh.Session {
	t.Helper()
	sess, err := client.NewSession()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

// TestWrapSession_CommandExitCode asserts Wait surfaces the real exit code.
func TestWrapSession_CommandExitCode(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client := driverSSHClient(t)

	p, err := agenttest.WrapSession(t, driverSession(t, client)).Command(ctx, "exit 3")
	require.NoError(t, err)

	st, err := p.Wait(ctx)
	require.NoError(t, err)
	require.True(t, st.Done)
	require.Equal(t, 3, st.ExitCode)
}

// TestWrapSession_CommandProcessErrorOnDeadWrite is the #1560 repro: writing
// to a process that already exited yields a diagnosable *ProcessError with the
// exit code, not a bare EOF.
func TestWrapSession_CommandProcessErrorOnDeadWrite(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client := driverSSHClient(t)

	p, err := agenttest.WrapSession(t, driverSession(t, client)).Command(ctx, "exit 3")
	require.NoError(t, err)
	_, err = p.Wait(ctx)
	require.NoError(t, err)

	err = p.WriteLine("too late")
	var pe *agenttest.ProcessError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, "write", pe.Op)
	require.Equal(t, 3, pe.Status.ExitCode)
	require.Contains(t, pe.Error(), "exited code 3", "the error message is diagnosable, not a bare EOF")
}

// TestWrapSession_CommandStderr asserts stderr is captured continuously and
// surfaced on Wait and via Stderr().
func TestWrapSession_CommandStderr(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client := driverSSHClient(t)

	p, err := agenttest.WrapSession(t, driverSession(t, client)).Command(ctx, "echo oops 1>&2")
	require.NoError(t, err)

	st, err := p.Wait(ctx)
	require.NoError(t, err)
	require.Contains(t, st.Stderr, "oops")
	require.Contains(t, p.Stderr(), "oops")
}

// TestWrapSession_CommandReadFromLiveProcess reads output from a process that
// is still running (the streaming-readiness case, e.g. a server logging
// "listening on ..."), then confirms a clean exit. Cross-platform.
func TestWrapSession_CommandReadFromLiveProcess(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client := driverSSHClient(t)

	// Print a marker, then stay alive briefly so the read happens while the
	// process is running rather than after it exits.
	cmd := "echo ready; sleep 1"
	if runtime.GOOS == "windows" {
		// ping is the Windows sleep that tolerates redirected stdin; timeout
		// and findstr reject it ("Input redirection is not supported").
		cmd = "echo ready& ping -n 2 127.0.0.1 >nul"
	}
	p, err := agenttest.WrapSession(t, driverSession(t, client)).Command(ctx, cmd)
	require.NoError(t, err)

	out := p.ReadUntil(ctx, "ready")
	require.NotContains(t, out, "ready", "ReadUntil returns the text before the token")

	st, err := p.Wait(ctx)
	require.NoError(t, err)
	require.True(t, st.Done)
	require.Equal(t, 0, st.ExitCode)
}

// TestWrapSession_CommandStdinEcho drives a bidirectional stdin->stdout echo:
// write a line, read it back while the process runs, close stdin, confirm a
// clean exit. This is the netcat/cat streaming case. It is POSIX-only: there
// is no reliable non-PTY line echo on Windows (findstr rejects piped input),
// so Windows interactive stdin is covered by the PTY Shell test instead.
func TestWrapSession_CommandStdinEcho(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("no reliable non-PTY line echo on Windows; see the Shell PTY test")
	}
	ctx := testutil.Context(t, testutil.WaitLong)
	client := driverSSHClient(t)

	p, err := agenttest.WrapSession(t, driverSession(t, client)).Command(ctx, "cat")
	require.NoError(t, err)

	require.NoError(t, p.WriteLine("ping"))
	out := p.ReadUntil(ctx, "ping")
	require.NotContains(t, out, "ping", "ReadUntil returns the text before the token")

	require.NoError(t, p.Close())
	st, err := p.Wait(ctx)
	require.NoError(t, err)
	require.True(t, st.Done)
	require.Equal(t, 0, st.ExitCode)
}

// TestWrapSession_CommandReadStream consumes stdout through the io.Reader
// interface (io.ReadAll), proving the driver composes with the stdlib and
// returns io.EOF once the process exits.
func TestWrapSession_CommandReadStream(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client := driverSSHClient(t)

	p, err := agenttest.WrapSession(t, driverSession(t, client)).Command(ctx, "echo hello")
	require.NoError(t, err)

	out, err := io.ReadAll(p)
	require.NoError(t, err)
	require.Contains(t, string(out), "hello")
}

// TestWrapSession_Shell starts a login shell over a PTY and confirms it returns
// after the first prompt (no Ready() footgun), then drives sequential commands
// through the prompt cycle. The prompt marker must be matched as a substring
// despite terminal control sequences.
func TestWrapSession_Shell(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client := driverSSHClient(t)

	sh, err := agenttest.WrapSession(t, driverSession(t, client)).Shell(ctx)
	require.NoError(t, err)

	out, err := sh.Run(ctx, "echo hello-shell")
	require.NoError(t, err)
	require.Contains(t, out, "hello-shell")

	// A second command cycles the prompt and does not bleed the first
	// command's output.
	out2, err := sh.Run(ctx, "echo second-line")
	require.NoError(t, err)
	require.Contains(t, out2, "second-line")
	require.NotContains(t, out2, "hello-shell")
}

// TestWrapSession_Signal signals a running process and confirms Wait reports it
// as terminated rather than a clean exit. POSIX-only: SSH signal delivery to
// Windows processes is not supported by the agent.
func TestWrapSession_Signal(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	ctx := testutil.Context(t, testutil.WaitLong)
	client := driverSSHClient(t)

	p, err := agenttest.WrapSession(t, driverSession(t, client)).Command(ctx, "echo started; exec sleep 60")
	require.NoError(t, err)

	// exec replaces the shell with sleep so the signal targets the actual
	// long-running process. Without exec the shell stays resident and the
	// orphaned sleep keeps stdout open, so the agent (which signals only the
	// shell, matching OpenSSH) would not tear the command down.
	p.ReadUntil(ctx, "started")

	require.NoError(t, p.Signal(ssh.SIGKILL))

	st, err := p.Wait(ctx)
	require.NoError(t, err)
	require.True(t, st.Done)
	require.NotEqual(t, 0, st.ExitCode, "a killed process is not a clean exit")
}

// TestWrapSession_ShellBanner confirms the login output before the first prompt
// is captured as the banner and the consumed prompt marker is not in it.
func TestWrapSession_ShellBanner(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client := driverSSHClient(t)

	sh, err := agenttest.WrapSession(t, driverSession(t, client)).Shell(ctx)
	require.NoError(t, err)

	require.NotContains(t, sh.Banner(), agenttest.PromptMarker, "the marker is consumed, not part of the banner")
	if runtime.GOOS == "windows" {
		require.Contains(t, sh.Banner(), "Microsoft Windows")
	}
}
