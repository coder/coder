package ptytest

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/acarl005/stripansi"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func New(t *testing.T, opts ...pty.Option) *PTY {
	t.Helper()

	ptty, err := pty.New(opts...)
	require.NoError(t, err)

	e := newExpecter(t, ptty.Output(), "cmd")
	r := &PTY{
		outExpecter: e,
		PTY:         ptty,
	}
	// Ensure pty is cleaned up at the end of test.
	t.Cleanup(func() {
		_ = r.Close()
	})
	return r
}

// Start starts a new process asynchronously and returns a PTYCmd and Process.
// It kills the process and PTYCmd upon cleanup
func Start(t *testing.T, cmd *pty.Cmd, opts ...pty.StartOption) (*PTYCmd, pty.Process) {
	t.Helper()

	ptty, ps, err := pty.Start(cmd, opts...)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Kill()
		_ = ps.Wait()
	})
	ex := newExpecter(t, ptty.OutputReader(), cmd.Args[0])

	r := &PTYCmd{
		outExpecter: ex,
		PTYCmd:      ptty,
	}
	t.Cleanup(func() {
		_ = r.Close()
	})
	return r, ps
}

func newExpecter(t *testing.T, r io.Reader, name string) outExpecter {
	// Use pipe for logging.
	logDone := make(chan struct{})
	logr, logw := io.Pipe()

	// Write to log and output buffer.
	copyDone := make(chan struct{})
	out := newStdbuf()
	w := io.MultiWriter(logw, out)

	ex := outExpecter{
		t:    t,
		out:  out,
		name: name,

		runeReader: bufio.NewReaderSize(out, utf8.UTFMax),
	}

	logClose := func(name string, c io.Closer) {
		ex.logf("closing %s", name)
		err := c.Close()
		ex.logf("closed %s: %v", name, err)
	}
	// Set the actual close function for the outExpecter.
	ex.close = func(reason string) error {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		ex.logf("closing expecter: %s", reason)

		// Caller needs to have closed the PTY so that copying can complete
		select {
		case <-ctx.Done():
			ex.fatalf("close", "copy did not close in time")
		case <-copyDone:
		}

		logClose("logw", logw)
		logClose("logr", logr)
		select {
		case <-ctx.Done():
			ex.fatalf("close", "log pipe did not close in time")
		case <-logDone:
		}

		ex.logf("closed expecter")

		return nil
	}

	go func() {
		defer close(copyDone)
		_, err := io.Copy(w, r)
		ex.logf("copy done: %v", err)
		ex.logf("closing out")
		err = out.closeErr(err)
		ex.logf("closed out: %v", err)
	}()

	// Log all output as part of test for easier debugging on errors.
	go func() {
		defer close(logDone)
		s := bufio.NewScanner(logr)
		for s.Scan() {
			ex.logf("%q", stripansi.Strip(s.Text()))
		}
	}()

	return ex
}

type outExpecter struct {
	t     *testing.T
	close func(reason string) error
	out   *stdbuf
	name  string

	runeReader *bufio.Reader
}

func (e *outExpecter) ExpectMatch(str string) string {
	return e.expectMatchContextFunc(str, e.ExpectMatchContext)
}

func (e *outExpecter) ExpectRegexMatch(str string) string {
	return e.expectMatchContextFunc(str, e.ExpectRegexMatchContext)
}

func (e *outExpecter) expectMatchContextFunc(str string, fn func(ctx context.Context, str string) string) string {
	e.t.Helper()

	timeout, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	return fn(timeout, str)
}

// TODO(mafredri): Rename this to ExpectMatch when refactoring.
func (e *outExpecter) ExpectMatchContext(ctx context.Context, str string) string {
	return e.expectMatcherFunc(ctx, str, strings.Contains)
}

func (e *outExpecter) ExpectRegexMatchContext(ctx context.Context, str string) string {
	return e.expectMatcherFunc(ctx, str, func(src, pattern string) bool {
		return regexp.MustCompile(pattern).MatchString(src)
	})
}

func (e *outExpecter) expectMatcherFunc(ctx context.Context, str string, fn func(src, pattern string) bool) string {
	e.t.Helper()

	var buffer bytes.Buffer
	err := e.doMatchWithDeadline(ctx, "ExpectMatchContext", func(rd *bufio.Reader) error {
		for {
			r, _, err := rd.ReadRune()
			if err != nil {
				return err
			}
			_, err = buffer.WriteRune(r)
			if err != nil {
				return err
			}
			if fn(buffer.String(), str) {
				return nil
			}
		}
	})
	if err != nil {
		e.fatalf("read error", "%v (wanted %q; got %q)", err, str, buffer.String())
		return ""
	}
	e.logf("matched %q = %q", str, buffer.String())
	return buffer.String()
}

// ExpectNoMatchBefore validates that `match` does not occur before `before`.
func (e *outExpecter) ExpectNoMatchBefore(ctx context.Context, match, before string) string {
	e.t.Helper()

	var buffer bytes.Buffer
	err := e.doMatchWithDeadline(ctx, "ExpectNoMatchBefore", func(rd *bufio.Reader) error {
		for {
			r, _, err := rd.ReadRune()
			if err != nil {
				return err
			}
			_, err = buffer.WriteRune(r)
			if err != nil {
				return err
			}

			if strings.Contains(buffer.String(), match) {
				return xerrors.Errorf("found %q before %q", match, before)
			}

			if strings.Contains(buffer.String(), before) {
				return nil
			}
		}
	})
	if err != nil {
		e.fatalf("read error", "%v (wanted no %q before %q; got %q)", err, match, before, buffer.String())
		return ""
	}
	e.logf("matched %q = %q", before, stripansi.Strip(buffer.String()))
	return buffer.String()
}

func (e *outExpecter) Peek(ctx context.Context, n int) []byte {
	e.t.Helper()

	var out []byte
	err := e.doMatchWithDeadline(ctx, "Peek", func(rd *bufio.Reader) error {
		var err error
		out, err = rd.Peek(n)
		return err
	})
	if err != nil {
		e.fatalf("read error", "%v (wanted %d bytes; got %d: %q)", err, n, len(out), out)
		return nil
	}
	e.logf("peeked %d/%d bytes = %q", len(out), n, out)
	return slices.Clone(out)
}

//nolint:govet // We don't care about conforming to ReadRune() (rune, int, error).
func (e *outExpecter) ReadRune(ctx context.Context) rune {
	e.t.Helper()

	var r rune
	err := e.doMatchWithDeadline(ctx, "ReadRune", func(rd *bufio.Reader) error {
		var err error
		r, _, err = rd.ReadRune()
		return err
	})
	if err != nil {
		e.fatalf("read error", "%v (wanted rune; got %q)", err, r)
		return 0
	}
	e.logf("matched rune = %q", r)
	return r
}

func (e *outExpecter) ReadLine(ctx context.Context) string {
	e.t.Helper()

	var buffer bytes.Buffer
	err := e.doMatchWithDeadline(ctx, "ReadLine", func(rd *bufio.Reader) error {
		for {
			r, _, err := rd.ReadRune()
			if err != nil {
				return err
			}
			if r == '\n' {
				return nil
			}
			if r == '\r' {
				// Peek the next rune to see if it's an LF and then consume
				// it.

				// Unicode code points can be up to 4 bytes, but the
				// ones we're looking for are only 1 byte.
				b, _ := rd.Peek(1)
				if len(b) == 0 {
					return nil
				}

				r, _ = utf8.DecodeRune(b)
				if r == '\n' {
					_, _, err = rd.ReadRune()
					if err != nil {
						return err
					}
				}

				return nil
			}

			_, err = buffer.WriteRune(r)
			if err != nil {
				return err
			}
		}
	})
	if err != nil {
		e.fatalf("read error", "%v (wanted newline; got %q)", err, buffer.String())
		return ""
	}
	e.logf("matched newline = %q", buffer.String())
	return buffer.String()
}

func (e *outExpecter) ReadAll() []byte {
	e.t.Helper()
	return e.out.ReadAll()
}

func (e *outExpecter) doMatchWithDeadline(ctx context.Context, name string, fn func(*bufio.Reader) error) error {
	e.t.Helper()

	// A timeout is mandatory, caller can decide by passing a context
	// that times out.
	if _, ok := ctx.Deadline(); !ok {
		timeout := testutil.WaitMedium
		e.logf("%s ctx has no deadline, using %s", name, timeout)
		var cancel context.CancelFunc
		//nolint:gocritic // Rule guard doesn't detect that we're using testutil.Wait*.
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	match := make(chan error, 1)
	go func() {
		defer close(match)
		match <- fn(e.runeReader)
	}()
	select {
	case err := <-match:
		return err
	case <-ctx.Done():
		// Ensure goroutine is cleaned up before test exit, do not call
		// (*outExpecter).close here to let the caller decide.
		_ = e.out.Close()
		<-match

		return xerrors.Errorf("match deadline exceeded: %w", ctx.Err())
	}
}

func (e *outExpecter) logf(format string, args ...interface{}) {
	e.t.Helper()

	// Match regular logger timestamp format, we seem to be logging in
	// UTC in other places as well, so match here.
	e.t.Logf("%s: %s: %s", time.Now().UTC().Format("2006-01-02 15:04:05.000"), e.name, fmt.Sprintf(format, args...))
}

func (e *outExpecter) fatalf(reason string, format string, args ...interface{}) {
	e.t.Helper()

	// Ensure the message is part of the normal log stream before
	// failing the test.
	e.logf("%s: %s", reason, fmt.Sprintf(format, args...))

	require.FailNowf(e.t, reason, format, args...)
}

type PTY struct {
	outExpecter
	pty.PTY
	closeOnce sync.Once
	closeErr  error
}

func (p *PTY) Close() error {
	p.t.Helper()
	p.closeOnce.Do(func() {
		pErr := p.PTY.Close()
		if pErr != nil {
			p.logf("PTY: Close failed: %v", pErr)
		}
		eErr := p.outExpecter.close("PTY close")
		if eErr != nil {
			p.logf("PTY: close expecter failed: %v", eErr)
		}
		if pErr != nil {
			p.closeErr = pErr
			return
		}
		p.closeErr = eErr
	})
	return p.closeErr
}

func (p *PTY) Attach(inv *serpent.Invocation) *PTY {
	p.t.Helper()

	inv.Stdout = p.Output()
	inv.Stderr = p.Output()
	inv.Stdin = p.Input()
	return p
}

func (p *PTY) Write(r rune) {
	p.t.Helper()

	p.logf("stdin: %q", r)
	_, err := p.Input().Write([]byte{byte(r)})
	require.NoError(p.t, err, "write failed")
}

func (p *PTY) WriteLine(str string) {
	p.t.Helper()

	newline := []byte{'\r'}
	if runtime.GOOS == "windows" {
		newline = append(newline, '\n')
	}
	p.logf("stdin: %q", str+string(newline))
	_, err := p.Input().Write(append([]byte(str), newline...))
	require.NoError(p.t, err, "write line failed")
}

type PTYCmd struct {
	outExpecter
	pty.PTYCmd
}

func (p *PTYCmd) Close() error {
	p.t.Helper()
	pErr := p.PTYCmd.Close()
	if pErr != nil {
		p.logf("PTYCmd: Close failed: %v", pErr)
	}
	eErr := p.outExpecter.close("PTYCmd close")
	if eErr != nil {
		p.logf("PTYCmd: close expecter failed: %v", eErr)
	}
	if pErr != nil {
		return pErr
	}
	return eErr
}

// stdbuf is like a buffered stdout, it buffers writes until read.
type stdbuf struct {
	r io.Reader

	mu   sync.Mutex // Protects following.
	b    []byte
	more chan struct{}
	err  error
}

func newStdbuf() *stdbuf {
	return &stdbuf{more: make(chan struct{}, 1)}
}

func (b *stdbuf) ReadAll() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.err != nil {
		return nil
	}
	p := append([]byte(nil), b.b...)
	b.b = b.b[len(b.b):]
	return p
}

func (b *stdbuf) Read(p []byte) (int, error) {
	if b.r == nil {
		return b.readOrWaitForMore(p)
	}

	n, err := b.r.Read(p)
	if xerrors.Is(err, io.EOF) {
		b.r = nil
		err = nil
		if n == 0 {
			return b.readOrWaitForMore(p)
		}
	}
	return n, err
}

func (b *stdbuf) readOrWaitForMore(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Deplete channel so that more check
	// is for future input into buffer.
	select {
	case <-b.more:
	default:
	}

	if len(b.b) == 0 {
		if b.err != nil {
			return 0, b.err
		}

		b.mu.Unlock()
		<-b.more
		b.mu.Lock()
	}

	b.r = bytes.NewReader(b.b)
	b.b = b.b[len(b.b):]

	return b.r.Read(p)
}

func (b *stdbuf) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.err != nil {
		return 0, b.err
	}

	b.b = append(b.b, p...)

	select {
	case b.more <- struct{}{}:
	default:
	}

	return len(p), nil
}

func (b *stdbuf) Close() error {
	return b.closeErr(nil)
}

func (b *stdbuf) closeErr(err error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return err
	}
	if err == nil {
		b.err = io.EOF
	} else {
		b.err = err
	}
	close(b.more)
	return err
}
