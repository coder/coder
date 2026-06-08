package agenttest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/acarl005/stripansi"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
)

// Session adapts a single-use *gossh.Session (the caller did any native
// Setenv/SendRequest first) into the protected test driver. Pre-start
// configuration is chained, e.g. WrapSession(t, s).PTY().Command(ctx, cmd).
type Session struct {
	t   testing.TB
	s   *gossh.Session
	pty bool
}

// WrapSession wraps a fresh, unstarted *gossh.Session.
func WrapSession(t testing.TB, s *gossh.Session) *Session {
	return &Session{t: t, s: s}
}

// PTY requests a wide PTY before start so a prompt marker cannot wrap.
func (s *Session) PTY() *Session {
	s.pty = true
	return s
}

// Command starts an arbitrary command and returns the protected stream
// driver. The command runs under the session's shell (shell -c cmd).
func (s *Session) Command(ctx context.Context, cmd string) (*Process, error) {
	return s.start(ctx, cmd)
}

// Shell starts a login shell over a PTY and returns after the first prompt.
// The consumed login output is available via Banner.
func (s *Session) Shell(ctx context.Context) (*Shell, error) {
	s.pty = true
	p, err := s.start(ctx, "")
	if err != nil {
		return nil, err
	}
	sh := &Shell{Process: p}
	sh.banner = p.ReadUntil(ctx, PromptMarker)
	return sh, nil
}

func (s *Session) start(ctx context.Context, cmd string) (*Process, error) {
	s.t.Helper()

	stdin, err := s.s.StdinPipe()
	if err != nil {
		return nil, xerrors.Errorf("stdin pipe: %w", err)
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	p := &Process{
		t:         s.t,
		s:         s.s,
		stdin:     stdin,
		out:       testutil.NewWaitBuffer(),
		waited:    make(chan struct{}),
		runCtx:    runCtx,
		runCancel: runCancel,
	}
	// gossh copies the channel streams into these writers and Wait() waits for
	// the copies to finish, so the driver owns no stdout/stderr copy
	// goroutines. Under a PTY the server folds stderr into the single terminal
	// stream, so stderr is captured separately only for non-PTY commands.
	s.s.Stdout = p.out
	if !s.pty {
		p.errBuf = testutil.NewWaitBuffer()
		s.s.Stderr = p.errBuf
	}

	// Enforce goroutine lifetimes: closing the session unblocks Wait and the
	// gossh copies, then wg.Wait confirms every goroutine we spawned has ended
	// before the test finishes. Registered before any goroutine starts so the
	// error paths below drain just as well as the success path.
	s.t.Cleanup(func() {
		_ = p.s.Close()
		p.runCancel()
		p.wg.Wait()
		p.t.Logf("ssh out:\n%s", clean(p.out.String()))
		if e := p.capturedStderr(); e != "" {
			p.t.Logf("ssh err:\n%s", clean(e))
		}
	})

	// Bound the start handshake (a wide PTY request keeps a prompt marker on
	// one line) on ctx so a stuck connection fails fast. The goroutine is
	// tracked by wg; the cleanup's session Close unblocks it on the ctx path.
	startErr := make(chan error, 1)
	p.wg.Go(func() {
		if s.pty {
			if err := s.s.RequestPty("xterm", 50, 200, gossh.TerminalModes{}); err != nil {
				startErr <- xerrors.Errorf("request pty: %w", err)
				return
			}
		}
		if cmd == "" {
			startErr <- s.s.Shell()
		} else {
			startErr <- s.s.Start(cmd)
		}
	})
	select {
	case err := <-startErr:
		if err != nil {
			return nil, xerrors.Errorf("start: %w", err)
		}
	case <-ctx.Done():
		return nil, xerrors.Errorf("start: %w", ctx.Err())
	}

	p.mu.Lock()
	p.status.Started = true
	p.mu.Unlock()
	p.wg.Go(p.waitLoop)

	return p, nil
}

// Process is the protected I/O core. It implements io.ReadWriteCloser over the
// remote process: Write feeds stdin, Read consumes stdout, Close closes stdin.
// Reads also have a token matcher (ReadUntil). Writes are logged. A write to a
// process that already exited returns a *ProcessError carrying the exit code
// and captured stderr, never a bare EOF, which is the diagnosable form of the
// #1560 bug.
type Process struct {
	t     testing.TB
	s     *gossh.Session
	stdin io.WriteCloser
	// out captures stdout (and merged stderr under a PTY); errBuf captures
	// stderr for non-PTY commands and is nil otherwise. Both are thread-safe.
	out    *testutil.WaitBuffer
	errBuf *testutil.WaitBuffer
	// readOff is the number of stdout bytes already consumed by Read/ReadUntil.
	// Reads are sequential, so it needs no lock.
	readOff int

	// waited is closed once the process has exited and status is final.
	waited chan struct{}
	// runCtx is canceled when the process exits (or on cleanup), so a Read
	// blocked for more stdout unblocks and returns io.EOF.
	runCtx    context.Context
	runCancel context.CancelFunc
	// wg tracks the goroutines this driver spawns so the cleanup can confirm
	// they have all ended.
	wg sync.WaitGroup

	mu     sync.Mutex
	status Status
}

var _ io.ReadWriteCloser = (*Process)(nil)

// waitLoop observes the process exit on the single allowed gossh Wait call.
// gossh's Wait returns only after the stdout/stderr copies finish, so the
// captured buffers are complete once status is final.
func (p *Process) waitLoop() {
	code, sig := exitInfo(p.s.Wait())
	p.mu.Lock()
	p.status.Done = true
	p.status.ExitCode = code
	p.status.Signal = sig
	p.mu.Unlock()
	close(p.waited)
	p.runCancel()
}

// exitInfo extracts the exit code and signal from a *gossh.Session.Wait error.
// It returns -1 when the code is unavailable.
func exitInfo(err error) (int, gossh.Signal) {
	if err == nil {
		return 0, ""
	}
	var ee *gossh.ExitError
	if errors.As(err, &ee) {
		return ee.ExitStatus(), gossh.Signal(ee.Signal())
	}
	return -1, ""
}

// Write implements io.Writer, feeding stdin. The bytes are logged. A write to
// an exited process returns a *ProcessError with the exit code and stderr.
func (p *Process) Write(b []byte) (int, error) {
	p.t.Logf("ssh in: %q", b)
	n, err := p.stdin.Write(b)
	if err != nil {
		return n, p.processError("write", err)
	}
	return n, nil
}

// Close implements io.Closer, closing stdin (signaling EOF to the process).
func (p *Process) Close() error {
	if err := p.stdin.Close(); err != nil {
		return p.processError("close", err)
	}
	return nil
}

// WriteLine writes a line terminated for the local shell.
func (p *Process) WriteLine(line string) error {
	_, err := io.WriteString(p, line+lineEnding)
	return err
}

// lineEnding submits a line to the local shell. cmd.exe under ConPTY treats CR
// as Enter and ignores a bare LF, so a stray LF would inject an empty-line
// prompt and desync later reads. POSIX shells accept LF.
var lineEnding = func() string {
	if runtime.GOOS == "windows" {
		return "\r"
	}
	return "\n"
}()

// Read implements io.Reader over stdout. It blocks until stdout has unread
// bytes, returning io.EOF once the process has exited and stdout is drained.
// Read shares its cursor with ReadUntil and is not safe for concurrent use.
func (p *Process) Read(b []byte) (int, error) {
	_ = p.out.WaitForCond(p.runCtx, func(s string) bool { return len(s) > p.readOff })
	data := p.out.Bytes()
	if p.readOff >= len(data) {
		return 0, io.EOF
	}
	n := copy(b, data[p.readOff:])
	p.readOff += n
	return n, nil
}

// ReadUntil reads until token appears and returns the text before it, with the
// token consumed. ANSI control sequences are stripped from the returned text.
// A token that never arrives fails the test once ctx expires, logging the
// output captured so far. Not safe for concurrent use.
func (p *Process) ReadUntil(ctx context.Context, token string) string {
	p.t.Helper()
	off := p.readOff
	err := p.out.WaitForCond(ctx, func(s string) bool {
		return strings.Contains(s[off:], token)
	})
	if err != nil {
		p.t.Fatalf("ReadUntil %q: %v\noutput so far:\n%s", token, err, p.out.String())
		return ""
	}
	rest := p.out.String()[off:]
	idx := strings.Index(rest, token)
	p.readOff = off + idx + len(token)
	return clean(rest[:idx])
}

// Signal sends an SSH signal to the process.
func (p *Process) Signal(sig gossh.Signal) error {
	if err := p.s.Signal(sig); err != nil {
		return p.processError("signal", err)
	}
	return nil
}

// Wait blocks until the process exits or ctx is done.
func (p *Process) Wait(ctx context.Context) (Status, error) {
	select {
	case <-p.waited:
		return p.Status(), nil
	case <-ctx.Done():
		return p.Status(), xerrors.Errorf("wait: %w", ctx.Err())
	}
}

// Status returns a snapshot of the process lifecycle and exit disposition.
func (p *Process) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.status
	st.Stderr = p.capturedStderr()
	return st
}

// Stderr returns the stderr captured so far.
func (p *Process) Stderr() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.capturedStderr()
}

func (p *Process) capturedStderr() string {
	if p.errBuf == nil {
		return ""
	}
	return p.errBuf.String()
}

// processError attaches the process exit status to a failed stdin operation.
// It waits briefly for an in-flight exit to land so the returned status carries
// the exit code, bounded so a still-live process does not stall the test.
func (p *Process) processError(op string, err error) error {
	select {
	case <-p.waited:
	case <-time.After(testutil.WaitShort):
	}
	return &ProcessError{Op: op, Err: err, Status: p.Status()}
}

func clean(s string) string { return stripansi.Strip(s) }

// Shell is a Process plus prompt policy. Shell(ctx) has already blocked until
// the first prompt, so the consumed login output is available via Banner.
type Shell struct {
	*Process
	banner string
}

// Banner returns the login/MOTD output printed before the first prompt.
func (sh *Shell) Banner() string { return sh.banner }

// ReadPrompt reads to the next prompt and returns the text before it.
func (sh *Shell) ReadPrompt(ctx context.Context) string {
	return sh.ReadUntil(ctx, PromptMarker)
}

// Run writes a line and reads to the next prompt. It is not a one-shot exec;
// it drives the interactive shell.
func (sh *Shell) Run(ctx context.Context, line string) (string, error) {
	if err := sh.WriteLine(line); err != nil {
		return "", err
	}
	return sh.ReadPrompt(ctx), nil
}

// Status reports the lifecycle and exit disposition of a Process.
type Status struct {
	Started, Done bool
	ExitCode      int // -1 if unavailable
	Signal        gossh.Signal
	Stderr        string
}

// ProcessError is returned when a stdin operation targets a process that
// already exited. It carries the exit code and captured stderr so the failure
// is diagnosable instead of a bare EOF.
type ProcessError struct {
	Op     string
	Err    error
	Status Status
}

func (e *ProcessError) Error() string {
	// Surface the exit code and stderr so the failure is diagnosable, which is
	// the point of #1560. A bare EOF told you nothing about why the process
	// was gone.
	if e.Status.Done {
		msg := fmt.Sprintf("%s: process exited code %d", e.Op, e.Status.ExitCode)
		if stderr := strings.TrimSpace(e.Status.Stderr); stderr != "" {
			msg += ": " + stderr
		}
		return msg
	}
	return e.Op + ": " + e.Err.Error()
}

func (e *ProcessError) Unwrap() error { return e.Err }
