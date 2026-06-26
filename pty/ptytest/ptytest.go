package ptytest

import (
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/testutil/expecter"
	"github.com/coder/serpent"
)

func New(t *testing.T, opts ...pty.Option) *PTY {
	t.Helper()

	ptty, err := newTestPTY(opts...)
	require.NoError(t, err)

	e := expecter.New(t, ptty.Output(), "cmd")
	r := &PTY{
		t:        t,
		Expecter: *e,
		PTY:      ptty,
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
	ex := expecter.New(t, ptty.OutputReader(), cmd.Args[0])

	r := &PTYCmd{
		Expecter: *ex,
		PTYCmd:   ptty,
		t:        t,
	}
	t.Cleanup(func() {
		_ = r.Close()
	})
	return r, ps
}

//nolint:govet // We don't care about conforming to ReadRune() (rune, int, error).

type PTY struct {
	expecter.Expecter
	pty.PTY
	t         *testing.T
	closeOnce sync.Once
	closeErr  error
}

func (p *PTY) Close() error {
	p.t.Helper()
	p.closeOnce.Do(func() {
		pErr := p.PTY.Close()
		if pErr != nil {
			p.Logf("PTY: Close failed: %v", pErr)
		}
		p.Expecter.Close("PTY close")
		if pErr != nil {
			p.closeErr = pErr
		}
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

	p.Logf("stdin: %q", r)
	_, err := p.Input().Write([]byte{byte(r)})
	require.NoError(p.t, err, "write failed")
}

func (p *PTY) WriteLine(str string) {
	p.t.Helper()

	newline := []byte{'\r'}
	if runtime.GOOS == "windows" {
		newline = append(newline, '\n')
	}
	p.Logf("stdin: %q", str+string(newline))
	_, err := p.Input().Write(append([]byte(str), newline...))
	require.NoError(p.t, err, "write line failed")
}

// Named sets the PTY name in the logs. Defaults to "cmd". Make sure you set this before anything starts writing to the
// pty, or it may not be named consistently. E.g.
//
// p := New(t).Named("myCmd")
func (p *PTY) Named(name string) *PTY {
	p.Rename(name)
	return p
}

type PTYCmd struct {
	expecter.Expecter
	pty.PTYCmd
	t *testing.T
}

func (p *PTYCmd) Close() error {
	p.t.Helper()
	pErr := p.PTYCmd.Close()
	if pErr != nil {
		p.Logf("PTYCmd: Close failed: %v", pErr)
	}
	p.Expecter.Close("PTYCmd close")
	return pErr
}
