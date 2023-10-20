package testutil

import (
	"context"
	"io"
	"strings"
	"testing"

	"golang.org/x/xerrors"

	"github.com/hinshun/vt10x"
)

// TerminalReader emulates a terminal and allows matching output.  It's important in cases where we
// can get control sequences to parse them correctly, and keep the state of the terminal across the
// lifespan of the PTY, since some control sequences are relative to the current cursor position.
type TerminalReader struct {
	t    *testing.T
	r    io.Reader
	term vt10x.Terminal
}

func NewTerminalReader(t *testing.T, r io.Reader) *TerminalReader {
	return &TerminalReader{
		t:    t,
		r:    r,
		term: vt10x.New(vt10x.WithSize(80, 80)),
	}
}

// ReadUntilString emulates a terminal and reads one byte at a time until we
// either see the string we want, or the context expires.  The PTY must be sized
// to 80x80 or there could be unexpected results.
func (tr *TerminalReader) ReadUntilString(ctx context.Context, want string) error {
	return tr.ReadUntil(ctx, func(line string) bool {
		return strings.TrimSpace(line) == want
	})
}

// ReadUntil emulates a terminal and reads one byte at a time until the matcher
// returns true or the context expires.  If the matcher is nil, read until EOF.
// The PTY must be sized to 80x80 or there could be unexpected results.
func (tr *TerminalReader) ReadUntil(ctx context.Context, matcher func(line string) bool) (retErr error) {
	readBytes := make([]byte, 0)
	readErrs := make(chan error, 1)
	defer func() {
		// Dump the terminal contents since they can be helpful for debugging, but
		// trim empty lines since much of the terminal will usually be blank.
		got := tr.term.String()
		lines := strings.Split(got, "\n")
		for i := range lines {
			if strings.TrimSpace(lines[i]) != "" {
				lines = lines[i:]
				break
			}
		}
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) != "" {
				lines = lines[:i+1]
				break
			}
		}
		gotTrimmed := strings.Join(lines, "\n")
		tr.t.Logf("Terminal contents:\n%s", gotTrimmed)
		// EOF is expected when matcher == nil
		if retErr != nil && !(xerrors.Is(retErr, io.EOF) && matcher == nil) {
			tr.t.Logf("Bytes Read: %q", string(readBytes))
		}
	}()
	for {
		b := make([]byte, 1)
		go func() {
			_, err := tr.r.Read(b)
			readErrs <- err
		}()
		select {
		case err := <-readErrs:
			if err != nil {
				return err
			}
			readBytes = append(readBytes, b...)
			_, err = tr.term.Write(b)
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		if matcher == nil {
			// A nil matcher means to read until EOF.
			continue
		}
		got := tr.term.String()
		lines := strings.Split(got, "\n")
		for _, line := range lines {
			if matcher(line) {
				return nil
			}
		}
	}
}
