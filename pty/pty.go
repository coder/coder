package pty

import (
	"io"
	"os"
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
	Resize(height uint16, width uint16) error
}

// WithFlags represents a PTY whose flags can be inspected, in particular
// to determine whether local echo is enabled.
type WithFlags interface {
	PTY

	// EchoEnabled determines whether local echo is currently enabled for this terminal.
	EchoEnabled() (bool, error)
}

// New constructs a new Pty.
func New() (PTY, error) {
	return newPty()
}

// ReadWriter is an implementation of io.ReadWriter that wraps two separate
// underlying file descriptors, one for reading and one for writing, and allows
// them to be accessed separately.
type ReadWriter struct {
	Reader *os.File
	Writer *os.File
}

func (rw ReadWriter) Read(p []byte) (int, error) {
	return rw.Reader.Read(p)
}

func (rw ReadWriter) Write(p []byte) (int, error) {
	return rw.Writer.Write(p)
}
