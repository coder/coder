package pty

import (
	"io"
	"log"
	"os"

	"github.com/gliderlabs/ssh"
	"golang.org/x/xerrors"
)

// ErrClosed is returned when a PTY is used after it has been closed.
var ErrClosed = xerrors.New("pty: closed")

// PTYCmd is an interface for interacting with a pseudo-TTY where we control
// only one end, and the other end has been passed to a running os.Process.
// nolint:revive
type PTYCmd interface {
	io.Closer

	// Resize sets the size of the PTY.
	Resize(height uint16, width uint16) error

	// OutputReader returns an io.Reader for reading the output from the process
	// controlled by the pseudo-TTY
	OutputReader() io.Reader

	// InputWriter returns an io.Writer for writing into to the process
	// controlled by the pseudo-TTY
	InputWriter() io.Writer
}

// PTY is a minimal interface for interacting with pseudo-TTY where this
// process retains access to _both_ ends of the pseudo-TTY (i.e. `ptm` & `pts`
// on Linux).
type PTY interface {
	io.Closer

	// Resize sets the size of the PTY.
	Resize(height uint16, width uint16) error

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

	// Signal sends a signal to the command process. On non-windows systems, the
	// returned error is as for os.Process.Signal(), on Windows it's
	// as for os.Process.Kill().
	Signal(sig os.Signal) error
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
	Reader io.Reader
	Writer io.Writer
}

func (rw ReadWriter) Read(p []byte) (int, error) {
	return rw.Reader.Read(p)
}

func (rw ReadWriter) Write(p []byte) (int, error) {
	return rw.Writer.Write(p)
}
