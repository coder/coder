package pty

import (
	"context"
	"io"
	"log"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gliderlabs/ssh"
	"golang.org/x/xerrors"
)

const (
	defaultPTYMaxTotal   = 4096
	testSemaphoreTimeout = 30 * time.Second
)

// ErrClosed is returned when a PTY is used after it has been closed.
var ErrClosed = xerrors.New("pty: closed")

// testSemaphore limits the number of PTYs that can be created concurrently in tests.
// It is set in init() when testing.Testing() is true (go test); production builds leave it nil.
var (
	testSemaphore   chan struct{}
	testSemaphoreMu sync.RWMutex
)

func init() {
	// Only limit PTYs when running under go test; production builds never set the semaphore.
	if !testing.Testing() {
		return
	}
	maxTotal := defaultPTYMaxTotal
	if s := os.Getenv("PTY_MAX_TOTAL"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxTotal = n
		}
	}
	parallelPkgs := 8
	if s := os.Getenv("TEST_NUM_PARALLEL_PACKAGES"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			parallelPkgs = n
		}
	}
	capacity := maxTotal / parallelPkgs
	if capacity < 1 {
		capacity = 1
	}
	SetTestSemaphore(capacity)
}

// SetTestSemaphore configures a semaphore to limit concurrent PTY creation in tests.
// It is called from pty's init() when testing.Testing() is true;
// capacity is the maximum number of PTYs that can be created concurrently.
// If capacity is 0 or negative, the semaphore is disabled.
func SetTestSemaphore(capacity int) {
	testSemaphoreMu.Lock()
	defer testSemaphoreMu.Unlock()

	if capacity <= 0 {
		testSemaphore = nil
		return
	}

	// Create new semaphore with the specified capacity
	// If one already exists, it will be garbage collected
	testSemaphore = make(chan struct{}, capacity)
}

// acquireTestSemaphore attempts to acquire a slot from the test semaphore.
// Returns an error if the semaphore is set and acquisition times out (in tests).
// In production, the semaphore is nil and this returns immediately with no error.
// The caller must call releaseTestSemaphore in a defer.
func acquireTestSemaphore(ctx context.Context) error {
	testSemaphoreMu.RLock()
	sem := testSemaphore
	testSemaphoreMu.RUnlock()

	if sem == nil {
		// No semaphore set - production mode, no limit
		return nil
	}

	//nolint:gocritic // Not a test file; we do not want to import testutil here.
	timeoutCtx, cancel := context.WithTimeout(ctx, testSemaphoreTimeout)
	defer cancel()

	select {
	case sem <- struct{}{}:
		return nil
	case <-timeoutCtx.Done():
		return xerrors.Errorf("timeout waiting for PTY semaphore: %w", timeoutCtx.Err())
	}
}

// releaseTestSemaphore releases a slot back to the test semaphore.
func releaseTestSemaphore() {
	testSemaphoreMu.RLock()
	sem := testSemaphore
	testSemaphoreMu.RUnlock()

	if sem == nil {
		return
	}

	select {
	case <-sem:
	default:
		// Should not happen if acquire/release are paired correctly
	}
}

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
