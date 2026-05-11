//go:build !windows

package agentssh

import (
	"bufio"
	"context"
	"io"
	"net"
	"os/user"
	"testing"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/testutil"
)

const longScript = `
echo "started"
sleep 30
echo "done"
`

// Test_sessionStart_orphan tests running a command that takes a long time to
// exit normally, and terminate the SSH session context early to verify that we
// return quickly and don't leave the command running as an "orphan" with no
// active SSH session.
func Test_sessionStart_orphan(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	logger := testutil.Logger(t)
	s, err := NewServer(ctx, logger, prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, nil)
	require.NoError(t, err)
	defer s.Close()
	err = s.UpdateHostSigner(42)
	assert.NoError(t, err)

	// Here we're going to call the handler directly with a faked SSH session
	// that just uses io.Pipes instead of a network socket.  There is a large
	// variation in the time between closing the socket from the client side and
	// the SSH server canceling the session Context, which would lead to a flaky
	// test if we did it that way.  So instead, we directly cancel the context
	// in this test.
	sessionCtx, sessionCancel := context.WithCancel(ctx)
	toClient, fromClient, sess := newTestSession(sessionCtx)
	ptyInfo := gliderssh.Pty{}
	windowSize := make(chan gliderssh.Window)
	close(windowSize)
	// the command gets the session context so that Go will terminate it when
	// the session expires.
	cmd := pty.CommandContext(sessionCtx, "sh", "-c", longScript)

	done := make(chan struct{})
	go func() {
		defer close(done)

		// we don't really care what the error is here.  In the larger scenario,
		// the client has disconnected, so we can't return any error information
		// to them.
		_ = s.startPTYSession(logger, sess, "ssh", cmd, ptyInfo, windowSize)
	}()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		s := bufio.NewScanner(toClient)
		assert.True(t, s.Scan())
		txt := s.Text()
		assert.Equal(t, "started", txt, "output corrupted")
	}()

	waitForChan(ctx, t, readDone, "read timeout")
	// process is started, and should be sleeping for ~30 seconds

	sessionCancel()

	// now, we wait for the handler to complete.  If it does so before the
	// main test timeout, we consider this a pass.  If not, it indicates
	// that the server isn't properly shutting down sessions when they are
	// disconnected client side, which could lead to processes hanging around
	// indefinitely.
	waitForChan(ctx, t, done, "handler timeout")

	err = fromClient.Close()
	require.NoError(t, err)
}

func waitForChan(ctx context.Context, t *testing.T, c <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-c:
		// OK!
	case <-ctx.Done():
		t.Fatal(msg)
	}
}

type testSession struct {
	ctx testSSHContext

	// c2p is the client -> pty buffer
	toPty *io.PipeReader
	// p2c is the pty -> client buffer
	fromPty *io.PipeWriter
}

type testSSHContext struct {
	context.Context
}

var (
	_ gliderssh.Context = testSSHContext{}
	_ ptySession        = &testSession{}
)

func newTestSession(ctx context.Context) (toClient *io.PipeReader, fromClient *io.PipeWriter, s ptySession) {
	toClient, fromPty := io.Pipe()
	toPty, fromClient := io.Pipe()

	return toClient, fromClient, &testSession{
		ctx:     testSSHContext{ctx},
		toPty:   toPty,
		fromPty: fromPty,
	}
}

func (s *testSession) Context() gliderssh.Context {
	return s.ctx
}

func (*testSession) DisablePTYEmulation() {}

// RawCommand returns "quiet logon" so that the PTY handler doesn't attempt to
// write the message of the day, which will interfere with our tests.  It writes
// the message of the day if it's a shell login (zero length RawCommand()).
func (*testSession) RawCommand() string { return "quiet logon" }

func (s *testSession) Read(p []byte) (n int, err error) {
	return s.toPty.Read(p)
}

func (s *testSession) Write(p []byte) (n int, err error) {
	return s.fromPty.Write(p)
}

func (*testSession) Signals(_ chan<- gliderssh.Signal) {
	// Not implemented, but will be called.
}

func (testSSHContext) Lock() {
	panic("not implemented")
}

func (testSSHContext) Unlock() {
	panic("not implemented")
}

// User returns the username used when establishing the SSH connection.
func (testSSHContext) User() string {
	panic("not implemented")
}

// SessionID returns the session hash.
func (testSSHContext) SessionID() string {
	panic("not implemented")
}

// ClientVersion returns the version reported by the client.
func (testSSHContext) ClientVersion() string {
	panic("not implemented")
}

// ServerVersion returns the version reported by the server.
func (testSSHContext) ServerVersion() string {
	panic("not implemented")
}

// RemoteAddr returns the remote address for this connection.
func (testSSHContext) RemoteAddr() net.Addr {
	panic("not implemented")
}

// LocalAddr returns the local address for this connection.
func (testSSHContext) LocalAddr() net.Addr {
	panic("not implemented")
}

// Permissions returns the Permissions object used for this connection.
func (testSSHContext) Permissions() *gliderssh.Permissions {
	panic("not implemented")
}

// SetValue allows you to easily write new values into the underlying context.
func (testSSHContext) SetValue(_, _ interface{}) {
	panic("not implemented")
}

func (testSSHContext) KeepAlive() *gliderssh.SessionKeepAlive {
	panic("not implemented")
}

func TestFilterLoginEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "DropsLoginVars",
			in:   []string{"USER=root", "LOGNAME=root", "SHELL=/bin/bash", "PATH=/usr/bin"},
			want: []string{"PATH=/usr/bin"},
		},
		{
			name: "KeepsEntriesWithoutEquals",
			in:   []string{"NOEQUALS", "SHELL=/bin/bash", "OK=1"},
			want: []string{"NOEQUALS", "OK=1"},
		},
		{
			name: "CaseSensitive",
			in:   []string{"shell=foo", "Shell=bar", "SHELL=baz"},
			// USER/LOGNAME/SHELL are upper-case in POSIX, and
			// usershell.SystemEnvInfo.Shell only emits the upper-case
			// SHELL= key (see agent/usershell/usershell.go). The agent
			// therefore only needs to strip the upper-case forms; a
			// lower-case "shell" set by a user must pass through.
			want: []string{"shell=foo", "Shell=bar"},
		},
		{
			name: "EmptyInput",
			in:   []string{},
			want: []string{},
		},
		{
			name: "EmptyValueStillFiltered",
			in:   []string{"SHELL=", "PATH=/x"},
			want: []string{"PATH=/x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterLoginEnv(tt.in)
			require.Equal(t, tt.want, got)
		})
	}
}

// Regression test for https://github.com/coder/coder/issues/24414.
// The agent process snapshots SHELL=/bin/bash from os.Environ() at
// startup, then the workspace startup script runs `chsh -s /usr/bin/zsh`.
// Sessions created afterwards must see SHELL=/usr/bin/zsh (the fresh
// /etc/passwd value), not the stale snapshot.
func TestCommandEnv_FreshShellWinsOverStaleEnv(t *testing.T) {
	t.Parallel()

	stale := []string{
		// What os.Environ() returns inside the agent process after
		// the agent booted but before chsh ran.
		"SHELL=/bin/bash",
		"USER=staleuser",
		"LOGNAME=staleuser",
		"PATH=/usr/bin",
	}
	ei := &fakeEnvInfoer{
		userFn:    func() (*user.User, error) { return &user.User{Username: "coder", HomeDir: "/home/coder"}, nil },
		shellFn:   func(string) (string, error) { return "/usr/bin/zsh", nil },
		environFn: func() []string { return stale },
		homeDirFn: func() (string, error) { return "/home/coder", nil },
	}

	s := &Server{config: &Config{
		WorkingDirectory: func() string { return "/home/coder" },
		UpdateEnv: func(current []string) ([]string, error) {
			// Mirror agent.updateCommandEnv's first-wins dedup. With
			// the legacy ordering (SHELL=/bin/bash from os.Environ
			// first), this loop would lock in the stale shell.
			seen := map[string]string{}
			for _, kv := range current {
				key, val, ok := splitEnv(kv)
				if !ok {
					continue
				}
				if _, dup := seen[key]; !dup {
					seen[key] = val
				}
			}
			out := make([]string, 0, len(seen))
			for k, v := range seen {
				out = append(out, k+"="+v)
			}
			return out, nil
		},
	}}

	shell, _, env, err := s.CommandEnv(ei, nil)
	require.NoError(t, err)
	require.Equal(t, "/usr/bin/zsh", shell, "Shell return value must come from the passwd read")

	got := envMap(env)
	require.Equal(t, "/usr/bin/zsh", got["SHELL"], "fresh SHELL must win over the stale agent-startup snapshot")
	require.Equal(t, "coder", got["USER"], "fresh USER must win over the stale snapshot")
	require.Equal(t, "coder", got["LOGNAME"], "fresh LOGNAME must win over the stale snapshot")
	require.Equal(t, "/usr/bin", got["PATH"], "non-login env vars from the agent process must still be inherited")
}

// addEnv (passed by users via e.g. `coder ssh --env SHELL=...`) wins
// over both the stale snapshot and the fresh passwd value. This locks
// in the precedence order the CommandEnv docstring promises and
// guards against an over-aggressive fix to #24414 that would also
// drop user-specified overrides.
func TestCommandEnv_AddEnvShellWins(t *testing.T) {
	t.Parallel()

	ei := &fakeEnvInfoer{
		userFn:    func() (*user.User, error) { return &user.User{Username: "coder", HomeDir: "/home/coder"}, nil },
		shellFn:   func(string) (string, error) { return "/usr/bin/zsh", nil },
		environFn: func() []string { return []string{"SHELL=/bin/bash"} },
		homeDirFn: func() (string, error) { return "/home/coder", nil },
	}

	s := &Server{config: &Config{
		WorkingDirectory: func() string { return "/home/coder" },
		UpdateEnv: func(current []string) ([]string, error) {
			seen := map[string]string{}
			for _, kv := range current {
				key, val, ok := splitEnv(kv)
				if !ok {
					continue
				}
				if _, dup := seen[key]; !dup {
					seen[key] = val
				}
			}
			out := make([]string, 0, len(seen))
			for k, v := range seen {
				out = append(out, k+"="+v)
			}
			return out, nil
		},
	}}

	_, _, env, err := s.CommandEnv(ei, []string{"SHELL=/opt/fish/bin/fish"})
	require.NoError(t, err)
	require.Equal(t, "/opt/fish/bin/fish", envMap(env)["SHELL"],
		"explicit addEnv SHELL must override both the stale snapshot and the passwd value")
}

type fakeEnvInfoer struct {
	userFn    func() (*user.User, error)
	shellFn   func(string) (string, error)
	environFn func() []string
	homeDirFn func() (string, error)
}

func (f *fakeEnvInfoer) User() (*user.User, error)      { return f.userFn() }
func (f *fakeEnvInfoer) Shell(u string) (string, error) { return f.shellFn(u) }
func (f *fakeEnvInfoer) Environ() []string              { return f.environFn() }
func (f *fakeEnvInfoer) HomeDir() (string, error)       { return f.homeDirFn() }
func (*fakeEnvInfoer) ModifyCommand(c string, a ...string) (string, []string) {
	return c, a
}

func splitEnv(kv string) (string, string, bool) {
	for i := 0; i < len(kv); i++ {
		if kv[i] == '=' {
			return kv[:i], kv[i+1:], true
		}
	}
	return "", "", false
}

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, kv := range env {
		k, v, ok := splitEnv(kv)
		if !ok {
			continue
		}
		m[k] = v
	}
	return m
}
