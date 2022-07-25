package ptytest

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
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
)

func New(t *testing.T) *PTY {
	ptty, err := pty.New()
	require.NoError(t, err)

	return create(t, ptty, "cmd")
}

func Start(t *testing.T, cmd *exec.Cmd) (*PTY, *os.Process) {
	ptty, ps, err := pty.Start(cmd)
	require.NoError(t, err)
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
	go func() {
		defer close(logDone)
		s := bufio.NewScanner(logr)
		for s.Scan() {
			// Quote output to avoid terminal escape codes, e.g. bell.
			t.Logf("%s: stdout: %q", name, s.Text())
		}
	}()

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
		_ = out.Close
		_ = ptty.Close()
		<-copyDone
	})

	return &PTY{
		t:   t,
		PTY: ptty,
		out: out,

		runeReader: bufio.NewReaderSize(out, utf8.UTFMax),
	}
}

type PTY struct {
	t *testing.T
	pty.PTY
	out *stdbuf

	runeReader *bufio.Reader
}

func (p *PTY) ExpectMatch(str string) string {
	p.t.Helper()

	timeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
			p.t.Fatalf("%s: read error: %v (wanted %q; got %q)", time.Now(), err, str, buffer.String())
			return ""
		}
		p.t.Logf("%s: matched %q = %q", time.Now(), str, buffer.String())
		return buffer.String()
	case <-timeout.Done():
		// Ensure goroutine is cleaned up before test exit.
		_ = p.out.closeErr(p.Close())
		<-match

		p.t.Fatalf("%s: match exceeded deadline: wanted %q; got %q", time.Now(), str, buffer.String())
		return ""
	}
}

func (p *PTY) Write(r rune) {
	p.t.Helper()

	_, err := p.Input().Write([]byte{byte(r)})
	require.NoError(p.t, err)
}

func (p *PTY) WriteLine(str string) {
	p.t.Helper()

	newline := []byte{'\r'}
	if runtime.GOOS == "windows" {
		newline = append(newline, '\n')
	}
	_, err := p.Input().Write(append([]byte(str), newline...))
	require.NoError(p.t, err)
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
