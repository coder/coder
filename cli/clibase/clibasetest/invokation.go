package clibasetest

import (
	"bytes"

	"github.com/coder/coder/cli/clibase"
)

// IO is the standard input, output, and error for a command.
type IO struct {
	Stdin  *bytes.Buffer
	Stdout *bytes.Buffer
	Stderr *bytes.Buffer
}

// FakeIO sets Stdin, Stdout, and Stderr to buffers.
func FakeIO(i *clibase.Invokation) *IO {
	io := &IO{
		Stdin:  bytes.NewBuffer(nil),
		Stdout: bytes.NewBuffer(nil),
		Stderr: bytes.NewBuffer(nil),
	}
	i.Stdout = io.Stdout
	i.Stderr = io.Stderr
	i.Stdin = io.Stdin
	return io
}

// Invoke creates a fake invokation and IO.
func Invoke(cmd *clibase.Command, args ...string) (*clibase.Invokation, *IO) {
	i := clibase.Invokation{
		Args: args,
	}
	return &i, FakeIO(&i)
}
