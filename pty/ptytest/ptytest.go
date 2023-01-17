package ptytest

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/pty"
	"github.com/coder/coder/testutil"
)

func New(t *testing.T, opts ...pty.Option) *PTY {
	t.Helper()

	ptty, err := pty.New(opts...)
	require.NoError(t, err)

	return create(t, ptty, "cmd")
}

func Start(t *testing.T, cmd *exec.Cmd, opts ...pty.StartOption) (*PTY, pty.Process) {
	t.Helper()

	ptty, ps, err := pty.Start(cmd, opts...)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Kill()
		_ = ps.Wait()
	})
	return create(t, ptty, cmd.Args[0]), ps
}

func create(t *testing.T, ptty pty.PTY, name string) *PTY {
	// Use pipe for logging.
	logDone := make(chan struct{})
	logr, logw := io.Pipe()

	// Write to log and output buffer.
	copyDone := make(chan struct{})
	out := newStdbuf()
	w := io.MultiWriter(logw, out)

	tpty := &PTY{
		t:    t,
		PTY:  ptty,
		out:  out,
		name: name,

		runeReader: bufio.NewReaderSize(out, utf8.UTFMax),
	}
	// Ensure pty is cleaned up at the end of test.
	t.Cleanup(func() {
		_ = tpty.Close()
	})

	logClose := func(name string, c io.Closer) {
		tpty.logf("closing %s", name)
		err := c.Close()
		tpty.logf("closed %s: %v", name, err)
	}
	// Set the actual close function for the tpty.
	tpty.close = func(reason string) error {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		tpty.logf("closing tpty: %s", reason)

		// Close pty only so that the copy goroutine can consume the
		// remainder of it's buffer and then exit.
		logClose("pty", ptty)
		select {
		case <-ctx.Done():
			tpty.fatalf("close", "copy did not close in time")
		case <-copyDone:
		}

		logClose("logw", logw)
		logClose("logr", logr)
		select {
		case <-ctx.Done():
			tpty.fatalf("close", "log pipe did not close in time")
		case <-logDone:
		}

		tpty.logf("closed tpty")

		return nil
	}

	go func() {
		defer close(copyDone)
		_, err := io.Copy(w, ptty.Output())
		tpty.logf("copy done: %v", err)
		tpty.logf("closing out")
		err = out.closeErr(err)
		tpty.logf("closed out: %v", err)
	}()

	// Log all output as part of test for easier debugging on errors.
	go func() {
		defer close(logDone)
		s := bufio.NewScanner(logr)
		for s.Scan() {
			// Quote output to avoid terminal escape codes, e.g. bell.
			tpty.logf("stdout: %q", s.Text())
		}
	}()

	return tpty
}

type PTY struct {
	pty.PTY
	t     *testing.T
	close func(reason string) error
	out   *stdbuf
	name  string

	runeReader *bufio.Reader
}

func (p *PTY) Close() error {
	p.t.Helper()

	return p.close("close")
}

func (p *PTY) ExpectMatch(str string) string {
	p.t.Helper()

	timeout, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	return p.ExpectMatchContext(timeout, str)
}

// TODO(mafredri): Rename this to ExpectMatch when refactoring.
func (p *PTY) ExpectMatchContext(ctx context.Context, str string) string {
	p.t.Helper()

	var buffer bytes.Buffer
	err := p.doMatchWithDeadline(ctx, "ExpectMatchContext", func() error {
		for {
			r, _, err := p.runeReader.ReadRune()
			if err != nil {
				return err
			}
			_, err = buffer.WriteRune(r)
			if err != nil {
				return err
			}
			if strings.Contains(buffer.String(), str) {
				return nil
			}
		}
	})
	if err != nil {
		p.fatalf("read error", "%v (wanted %q; got %q)", err, str, buffer.String())
		return ""
	}
	p.logf("matched %q = %q", str, buffer.String())
	return buffer.String()
}

func (p *PTY) Peek(ctx context.Context, n int) []byte {
	p.t.Helper()

	var out []byte
	err := p.doMatchWithDeadline(ctx, "Peek", func() error {
		var err error
		out, err = p.runeReader.Peek(n)
		return err
	})
	if err != nil {
		p.fatalf("read error", "%v (wanted %d bytes; got %d: %q)", err, n, len(out), out)
		return nil
	}
	p.logf("peeked %d/%d bytes = %q", len(out), n, out)
	return slices.Clone(out)
}

func (p *PTY) ReadRune(ctx context.Context) rune {
	p.t.Helper()

	var r rune
	err := p.doMatchWithDeadline(ctx, "ReadRune", func() error {
		var err error
		r, _, err = p.runeReader.ReadRune()
		return err
	})
	if err != nil {
		p.fatalf("read error", "%v (wanted rune; got %q)", err, r)
		return 0
	}
	p.logf("matched rune = %q", r)
	return r
}

func (p *PTY) ReadLine(ctx context.Context) string {
	p.t.Helper()

	var buffer bytes.Buffer
	err := p.doMatchWithDeadline(ctx, "ReadLine", func() error {
		for {
			r, _, err := p.runeReader.ReadRune()
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
				b, _ := p.runeReader.Peek(1)
				if len(b) == 0 {
					return nil
				}

				r, _ = utf8.DecodeRune(b)
				if r == '\n' {
					_, _, err = p.runeReader.ReadRune()
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
		p.fatalf("read error", "%v (wanted newline; got %q)", err, buffer.String())
		return ""
	}
	p.logf("matched newline = %q", buffer.String())
	return buffer.String()
}

func (p *PTY) doMatchWithDeadline(ctx context.Context, name string, fn func() error) error {
	p.t.Helper()

	// A timeout is mandatory, caller can decide by passing a context
	// that times out.
	if _, ok := ctx.Deadline(); !ok {
		timeout := testutil.WaitMedium
		p.logf("%s ctx has no deadline, using %s", name, timeout)
		var cancel context.CancelFunc
		//nolint:gocritic // Rule guard doesn't detect that we're using testutil.Wait*.
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	match := make(chan error, 1)
	go func() {
		defer close(match)
		match <- fn()
	}()
	select {
	case err := <-match:
		return err
	case <-ctx.Done():
		// Ensure goroutine is cleaned up before test exit.
		_ = p.close("match deadline exceeded")
		<-match

		return xerrors.Errorf("match deadline exceeded: %w", ctx.Err())
	}
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

func (p *PTY) logf(format string, args ...interface{}) {
	p.t.Helper()

	// Match regular logger timestamp format, we seem to be logging in
	// UTC in other places as well, so match here.
	p.t.Logf("%s: %s: %s", time.Now().UTC().Format("2006-01-02 15:04:05.000"), p.name, fmt.Sprintf(format, args...))
}

func (p *PTY) fatalf(reason string, format string, args ...interface{}) {
	p.t.Helper()

	// Ensure the message is part of the normal log stream before
	// failing the test.
	p.logf("%s: %s", reason, fmt.Sprintf(format, args...))

	require.FailNowf(p.t, reason, format, args...)
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
