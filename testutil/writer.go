package testutil

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"gvisor.dev/gvisor/pkg/context"

	"cdr.dev/slog/v3"
	"github.com/coder/serpent"
)

// Writer wraps an underlying io.Writer and provides friendlier methods to write to it, including logging.
type Writer struct {
	t *testing.T
	w io.Writer
	l slog.Logger
}

func NewWriterAttachedToInvocation(t *testing.T, logger slog.Logger, invocation *serpent.Invocation) *Writer {
	r, w := io.Pipe()
	invocation.Stdin = r
	// Close the pipe at the end of the test to ensure any goroutine in the Invocation that reads from stdin won't leak.
	t.Cleanup(func() {
		_ = w.Close()
	})
	return &Writer{
		t: t,
		w: w,
		l: logger,
	}
}

func (w *Writer) Write(r rune) {
	w.t.Helper()
	_, err := w.w.Write([]byte{byte(r)})
	if assert.NoError(w.t, err, "write failed") {
		w.l.Debug(context.Background(), "wrote rune", slog.F("rune", r))
	}
}

func (w *Writer) WriteLine(str string) {
	w.t.Helper()

	// Always write Windows style endings since our CLI prompt readers trim both out. Note this is *different* than what
	// PTY-based tests do. On Unix-like operating systems we write a single carriage-return (\r) to delimit a line
	// and the PTY translates it to a line feed (\n) for the CLI command to read. Here there is no translation.
	newline := []byte{'\r', '\n'}

	_, err := w.w.Write(append([]byte(str), newline...))
	if assert.NoError(w.t, err, "write line failed") {
		w.l.Debug(context.Background(), "wrote line", slog.F("line", str+string(newline)))
	}
}
