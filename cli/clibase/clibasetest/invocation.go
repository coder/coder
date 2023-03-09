package clibasetest

import (
	"bytes"
	"io"
	"testing"

	"github.com/coder/coder/cli/clibase"
)

// IO is the standard input, output, and error for a command.
type IO struct {
	Stdin  bytes.Buffer
	Stdout bytes.Buffer
	Stderr bytes.Buffer
}

// FakeIO sets Stdin, Stdout, and Stderr to buffers.
func FakeIO(i *clibase.Invocation) *IO {
	var b IO
	i.Stdout = &b.Stdout
	i.Stderr = &b.Stderr
	i.Stdin = &b.Stdin
	return &b
}

type testWriter struct {
	prefix string
	t      *testing.T
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.t.Helper()
	w.t.Log(w.prefix, string(p))
	return len(p), nil
}

func TestWriter(t *testing.T, prefix string) io.Writer {
	return &testWriter{prefix: prefix, t: t}
}

// Invoke creates a fake invocation and IO.
func Invoke(cmd *clibase.Cmd, args ...string) (*clibase.Invocation, *IO) {
	i := cmd.Invoke(args...)
	return i, FakeIO(i)
}
