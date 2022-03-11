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
	Output() ReadWriter

	// Input handles TTY input.
	//
	// cmd.SetInput(pty.Input()) would be used to specify a command
	// uses the PTY input for reading.
	//
	// The same stream would be used to provide user input: pty.Input().Write(...)
	Input() ReadWriter

	// Resize sets the size of the PTY.
	Resize(cols uint16, rows uint16) error
}

// New constructs a new Pty.
func New() (PTY, error) {
	return newPty()
}

// ReadWriter implements io.ReadWriter, but is intentionally avoids
// using the interface to allow for direct access to the reader or
// writer. This is to enable a caller to grab file descriptors.
type ReadWriter struct {
	io.Reader
	io.Writer
}
