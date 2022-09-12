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
	t.Cleanup(func() {
		_ = logw.Close()
		_ = logr.Close()
		<-logDone // Guard against logging after test.
	})

	// Write to log and output buffer.
	copyDone := make(chan struct{})
	out := newStdbuf()
	w := io.MultiWriter(logw, out)
	go func() {
		defer close(copyDone)
		_, err := io.Copy(w, ptty.Output())
		_ = out.closeErr(err)
	}()
	t.Cleanup(func() {
		_ = out.Close()
		_ = ptty.Close()
		<-copyDone
	})

	tpty := &PTY{
		t:    t,
		PTY:  ptty,
		out:  out,
		name: name,

		runeReader: bufio.NewReaderSize(out, utf8.UTFMax),
	}

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
	t    *testing.T
	out  *stdbuf
	name string

	runeReader *bufio.Reader
}

func (p *PTY) ExpectMatch(str string) string {
	p.t.Helper()

	timeout, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	var buffer bytes.Buffer
	match := make(chan error, 1)
	go func() {
		defer close(match)
		match <- func() error {
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
		}()
	}()

	select {
	case err := <-match:
		if err != nil {
			p.fatalf("read error", "%v (wanted %q; got %q)", err, str, buffer.String())
			return ""
		}
		p.logf("matched %q = %q", str, buffer.String())
		return buffer.String()
	case <-timeout.Done():
		// Ensure gorouine is cleaned up before test exit.
		_ = p.out.closeErr(p.Close())
		<-match

		p.fatalf("match exceeded deadline", "wanted %q; got %q", str, buffer.String())
		return ""
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
