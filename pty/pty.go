package pty

import (
	"io"
)

// PTY is a minimal interface for interacting with a TTY.
type PTY interface {
	io.Closer

	// Output handles TTY output.
	//
	// cmd.SetOutput(pty.Output()) would be used to specify a command
	// uses the output stream for writing.
	//
	// The same stream could be read to validate output.
	Output() io.ReadWriter

	// Input handles TTY input.
	//
	// cmd.SetInput(pty.Input()) would be used to specify a command
	// uses the PTY input for reading.
	//
	// The same stream would be used to provide user input: pty.Input().Write(...)
	Input() io.ReadWriter

	// Resize sets the size of the PTY.
	Resize(height uint16, width uint16) error
}

// New constructs a new Pty.
func New() (PTY, error) {
	return newPty()
}

type readWriter struct {
	io.Reader
	io.Writer
}
