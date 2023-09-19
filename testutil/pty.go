package testutil

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/hinshun/vt10x"
)

// ReadUntilString emulates a terminal and reads one byte at a time until we
// either see the string we want, or the context expires.  The PTY must be sized
// to 80x80 or there could be unexpected results.
func ReadUntilString(ctx context.Context, t *testing.T, want string, r io.Reader) error {
	return ReadUntil(ctx, t, r, func(line string) bool {
		return strings.TrimSpace(line) == want
	})
}

// ReadUntil emulates a terminal and reads one byte at a time until the matcher
// returns true or the context expires.  If the matcher is nil, read until EOF.
// The PTY must be sized to 80x80 or there could be unexpected results.
func ReadUntil(ctx context.Context, t *testing.T, r io.Reader, matcher func(line string) bool) (retErr error) {
	// output can contain virtual terminal sequences, so we need to parse these
	// to correctly interpret getting what we want.
	readBytes := make([]byte, 0)
	term := vt10x.New(vt10x.WithSize(80, 80))
	readErrs := make(chan error, 1)
	defer func() {
		if retErr != nil {
			// Dump the terminal contents since they can be helpful for debugging, but
			// trim empty lines since much of the terminal will usually be blank.
			got := term.String()
			// trimmed := strings.Trim(got, "\n")
			t.Logf("Terminal contents:\n%q", got)
			t.Logf("Bytes Read: %q", string(readBytes))
		}
	}()
	for {
		b := make([]byte, 1)
		go func() {
			_, err := r.Read(b)
			readErrs <- err
		}()
		select {
		case err := <-readErrs:
			if err != nil {
				return err
			}
			readBytes = append(readBytes, b...)
			_, err = term.Write(b)
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
		got := term.String()
		lines := strings.Split(got, "\n")
		for _, line := range lines {
			if matcher(line) {
				return nil
			}
		}
	}
}
