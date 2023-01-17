package pty

import (
	"io"
	"log"
	"os"

	"github.com/gliderlabs/ssh"
)

// PTY is a minimal interface for interacting with a TTY.
type PTY interface {
	io.Closer

	// Name of the TTY. Example on Linux would be "/dev/pts/1".
	Name() string

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

// Process represents a process running in a PTY.  We need to trigger special processing on the PTY
// on process completion, meaning that we will have goroutines calling Wait() on the process.  Since
// the caller will also typically wait for the process, and it is not safe for multiple goroutines
// to Wait() on a process, this abstraction provides a goroutine-safe interface for interacting with
// the process.
type Process interface {
	// Wait for the command to complete.  Returned error is as for exec.Cmd.Wait()
	Wait() error

	// Kill the command process.  Returned error is as for os.Process.Kill()
	Kill() error
}

// WithFlags represents a PTY whose flags can be inspected, in particular
// to determine whether local echo is enabled.
type WithFlags interface {
	PTY

	// EchoEnabled determines whether local echo is currently enabled for this terminal.
	EchoEnabled() (bool, error)
}

// Options represents a an option for a PTY.
type Option func(*ptyOptions)

type ptyOptions struct {
	logger    *log.Logger
	sshReq    *ssh.Pty
	setGPGTTY bool
}

// WithSSHRequest applies the ssh.Pty request to the PTY.
//
// Only partially supported on Windows (e.g. window size).
func WithSSHRequest(req ssh.Pty) Option {
	return func(opts *ptyOptions) {
		opts.sshReq = &req
	}
}

// WithLogger sets a logger for logging errors.
func WithLogger(logger *log.Logger) Option {
	return func(opts *ptyOptions) {
		opts.logger = logger
	}
}

// WithGPGTTY sets the GPG_TTY environment variable to the PTY name. This only
// applies to non-Windows platforms.
func WithGPGTTY() Option {
	return func(opts *ptyOptions) {
		opts.setGPGTTY = true
	}
}

// New constructs a new Pty.
func New(opts ...Option) (PTY, error) {
	return newPty(opts...)
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
