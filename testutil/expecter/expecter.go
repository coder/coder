package expecter

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/acarl005/stripansi"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// logTee forwards a best-effort copy of each write to lines for
// line-oriented debug logging, in addition to the real write to out.
// The forward is non-blocking: a full channel just drops the chunk.
// Debug logging is a convenience, not a correctness requirement, and
// must never apply backpressure to the command's actual output pipe.
//
// A prior version of this package logged output by teeing writes
// through a second unbuffered io.Pipe read by a bufio.Scanner. That
// put a second zero-buffer hop in series with the command's own
// stdout/stderr pipe: if the scanner goroutine was ever delayed (GC
// pause, scheduler contention under heavy parallel test load), the
// tee write would block, which stopped the drain loop from reading the
// command's pipe, which permanently deadlocked the command's own
// Write call. See PLAT-251.
type logTee struct {
	out   io.Writer
	lines chan<- []byte
}

func (t logTee) Write(p []byte) (int, error) {
	n, err := t.out.Write(p)
	if n > 0 {
		select {
		case t.lines <- append([]byte(nil), p[:n]...):
		default:
		}
	}
	return n, err
}

func New(t *testing.T, r io.Reader, name string) *Expecter {
	copyDone := make(chan struct{})
	logDone := make(chan struct{})
	out := newStdbuf()
	logLines := make(chan []byte, 256)
	// logLines is buffered, so a send succeeds up to its capacity
	// regardless of whether the drain goroutine below has started
	// receiving yet. logReady lets the copy goroutine wait for the
	// drain goroutine to reach its receive loop first, so a burst of
	// output at startup can't fill the buffer and get silently dropped
	// before anything is listening.
	logReady := make(chan struct{})

	ex := &Expecter{
		t:    t,
		out:  out,
		name: atomic.NewString(name),

		runeReader: bufio.NewReaderSize(out, utf8.UTFMax),
		logDone:    logDone,
		copyDone:   copyDone,
	}

	go func() {
		defer close(copyDone)
		defer close(logLines)
		<-logReady
		_, err := io.Copy(logTee{out: out, lines: logLines}, r)
		ex.Logf("copy done: %v", err)
		ex.Logf("closing out")
		err = out.closeErr(err)
		ex.Logf("closed out: %v", err)
	}()

	// Log all output as part of test for easier debugging on errors.
	go func() {
		defer close(logDone)
		close(logReady)
		var buf []byte
		for chunk := range logLines {
			buf = append(buf, chunk...)
			for {
				idx := bytes.IndexByte(buf, '\n')
				if idx < 0 {
					break
				}
				ex.Logf("%q", stripansi.Strip(string(buf[:idx])))
				buf = buf[idx+1:]
			}
		}
		if len(buf) > 0 {
			ex.Logf("%q", stripansi.Strip(string(buf)))
		}
	}()

	return ex
}

func NewAttachedToInvocation(t *testing.T, invocation *serpent.Invocation) *Expecter {
	r, w := io.Pipe()
	invocation.Stdout = w
	invocation.Stderr = w
	e := New(t, r, "cmd")

	t.Cleanup(func() {
		// Serpent doesn't handle closing stdout after running the Invocation; normally the OS does that automatically when
		// the process exits. Close it here at the end of the test to ensure we don't leak goroutines reading from the
		// stdout/stderr.
		_ = w.Close()
		e.Close("test end")
	})
	return e
}

func NewPiped(t *testing.T) (*Expecter, io.Writer) {
	r, w := io.Pipe()
	e := New(t, r, "cmd")

	t.Cleanup(func() {
		// Close writer here at the end of the test to ensure we don't leak goroutines reading from the pipe.
		_ = w.Close()
		e.Close("test end")
	})
	return e, w
}

type Expecter struct {
	t    *testing.T
	out  *stdbuf
	name *atomic.String

	runeReader        *bufio.Reader
	copyDone, logDone chan struct{}
}

// Rename the expecter. Make sure you set this before anything starts writing to the
// stream, or it may not be named consistently.
func (e *Expecter) Rename(name string) {
	e.name.Store(name)
}

func (e *Expecter) Close(reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	e.Logf("closing expecter: %s", reason)

	// Caller needs to have closed the stream so that copying can complete
	select {
	case <-ctx.Done():
		e.fatalf("close", "copy did not close in time")
		return
	case <-e.copyDone:
	}

	select {
	case <-ctx.Done():
		e.fatalf("close", "log drain did not finish in time")
		return
	case <-e.logDone:
	}

	e.Logf("closed expecter")
}

func (e *Expecter) ExpectMatch(ctx context.Context, str string) string {
	return e.expectMatcherFunc(ctx, str, strings.Contains)
}

func (e *Expecter) ExpectRegexMatch(ctx context.Context, str string) string {
	return e.expectMatcherFunc(ctx, str, func(src, pattern string) bool {
		return regexp.MustCompile(pattern).MatchString(src)
	})
}

func (e *Expecter) expectMatcherFunc(ctx context.Context, str string, fn func(src, pattern string) bool) string {
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
	e.Logf("matched %q = %q", str, buffer.String())
	return buffer.String()
}

// ExpectNoMatchBefore validates that `match` does not occur before `before`.
func (e *Expecter) ExpectNoMatchBefore(ctx context.Context, match, before string) string {
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
	e.Logf("matched %q = %q", before, stripansi.Strip(buffer.String()))
	return buffer.String()
}

func (e *Expecter) Peek(ctx context.Context, n int) []byte {
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
	e.Logf("peeked %d/%d bytes = %q", len(out), n, out)
	return slices.Clone(out)
}

//nolint:govet // We don't care about conforming to ReadRune() (rune, int, error).
func (e *Expecter) ReadRune(ctx context.Context) rune {
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
	e.Logf("matched rune = %q", r)
	return r
}

func (e *Expecter) ReadLine(ctx context.Context) string {
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
	e.Logf("matched newline = %q", buffer.String())
	return buffer.String()
}

func (e *Expecter) ReadAll() []byte {
	e.t.Helper()
	return e.out.ReadAll()
}

func (e *Expecter) doMatchWithDeadline(ctx context.Context, name string, fn func(*bufio.Reader) error) error {
	e.t.Helper()

	// A timeout is mandatory, caller can decide by passing a context
	// that times out.
	if _, ok := ctx.Deadline(); !ok {
		timeout := testutil.WaitMedium
		e.Logf("%s ctx has no deadline, using %s", name, timeout)
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

func (e *Expecter) Logf(format string, args ...interface{}) {
	e.t.Helper()

	// Match regular logger timestamp format, we seem to be logging in
	// UTC in other places as well, so match here.
	e.t.Logf("%s: %s: %s", time.Now().UTC().Format("2006-01-02 15:04:05.000"), e.name.Load(), fmt.Sprintf(format, args...))
}

func (e *Expecter) fatalf(reason string, format string, args ...interface{}) {
	e.t.Helper()

	// Ensure the message is part of the normal log stream before
	// failing the test.
	e.Logf("%s: %s", reason, fmt.Sprintf(format, args...))

	require.FailNowf(e.t, reason, format, args...)
}
