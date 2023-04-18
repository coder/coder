package pty_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/hinshun/vt10x"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/pty"
	"github.com/coder/coder/testutil"
)

// Test_Start_copy tests that we can use io.Copy() on command output
// without deadlocking.
func Test_Start_copy(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	pc, cmd, err := pty.Start(exec.CommandContext(ctx, cmdEcho, argEcho...))
	require.NoError(t, err)
	b := &bytes.Buffer{}
	readDone := make(chan error)
	go func() {
		_, err := io.Copy(b, pc.OutputReader())
		readDone <- err
	}()

	select {
	case err := <-readDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Error("read timed out")
	}
	assert.Contains(t, b.String(), "test")

	cmdDone := make(chan error)
	go func() {
		cmdDone <- cmd.Wait()
	}()

	select {
	case err := <-cmdDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Error("cmd.Wait() timed out")
	}
}

// Test_Start_truncation tests that we can read command output without truncation
// even after the command has exited.
func Test_Start_trucation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	pc, cmd, err := pty.Start(exec.CommandContext(ctx, cmdCount, argCount...))

	require.NoError(t, err)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		// avoid buffered IO so that we can precisely control how many bytes to read.
		n := 1
		for n < countEnd-25 {
			want := fmt.Sprintf("%d", n)
			err := readUntil(ctx, t, want, pc.OutputReader())
			assert.NoError(t, err, "want: %s", want)
			if err != nil {
				return
			}
			n++
		}
	}()

	select {
	case <-readDone:
		// OK!
	case <-ctx.Done():
		t.Error("read timed out")
	}

	cmdDone := make(chan error)
	go func() {
		cmdDone <- cmd.Wait()
	}()

	select {
	case err := <-cmdDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Error("cmd.Wait() timed out")
	}

	// do our final 25 reads, to make sure the output wasn't lost
	readDone = make(chan struct{})
	go func() {
		defer close(readDone)
		// avoid buffered IO so that we can precisely control how many bytes to read.
		n := countEnd - 25
		for n <= countEnd {
			want := fmt.Sprintf("%d", n)
			err := readUntil(ctx, t, want, pc.OutputReader())
			assert.NoError(t, err, "want: %s", want)
			if err != nil {
				return
			}
			n++
		}
		// ensure we still get to EOF
		endB := &bytes.Buffer{}
		_, err := io.Copy(endB, pc.OutputReader())
		assert.NoError(t, err)
	}()

	select {
	case <-readDone:
		// OK!
	case <-ctx.Done():
		t.Error("read timed out")
	}
}

// readUntil reads one byte at a time until we either see the string we want, or the context expires
func readUntil(ctx context.Context, t *testing.T, want string, r io.Reader) error {
	// output can contain virtual terminal sequences, so we need to parse these
	// to correctly interpret getting what we want.
	term := vt10x.New(vt10x.WithSize(80, 80))
	readErrs := make(chan error, 1)
	for {
		b := make([]byte, 1)
		go func() {
			_, err := r.Read(b)
			readErrs <- err
		}()
		select {
		case err := <-readErrs:
			if err != nil {
				t.Logf("err: %v\ngot: %v", err, term)
				return err
			}
			term.Write(b)
		case <-ctx.Done():
			return ctx.Err()
		}
		got := term.String()
		lines := strings.Split(got, "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == want {
				t.Logf("want: %v\n got:%v", want, line)
				return nil
			}
		}
	}
}
