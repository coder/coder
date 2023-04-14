//go:build windows
// +build windows

package pty_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coder/coder/pty"
	"github.com/coder/coder/pty/ptytest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestStart(t *testing.T) {
	t.Parallel()
	t.Run("Echo", func(t *testing.T) {
		t.Parallel()
		ptty, ps := ptytest.Start(t, exec.Command("cmd.exe", "/c", "echo", "test"))
		ptty.ExpectMatch("test")
		err := ps.Wait()
		require.NoError(t, err)
		err = ptty.Close()
		require.NoError(t, err)
	})
	t.Run("Resize", func(t *testing.T) {
		t.Parallel()
		ptty, _ := ptytest.Start(t, exec.Command("cmd.exe"))
		err := ptty.Resize(100, 50)
		require.NoError(t, err)
		err = ptty.Close()
		require.NoError(t, err)
	})
	t.Run("Kill", func(t *testing.T) {
		t.Parallel()
		ptty, ps := ptytest.Start(t, exec.Command("cmd.exe"))
		err := ps.Kill()
		assert.NoError(t, err)
		err = ps.Wait()
		var exitErr *exec.ExitError
		require.True(t, xerrors.As(err, &exitErr))
		assert.NotEqual(t, 0, exitErr.ExitCode())
		err = ptty.Close()
		require.NoError(t, err)
	})
}

func Test_Start_copy(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	pc, cmd, err := pty.Start(exec.CommandContext(ctx, "cmd.exe", "/c", "echo", "test"))
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

const countEnd = 1000

func Test_Start_trucation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1000)
	defer cancel()

	pc, cmd, err := pty.Start(exec.CommandContext(ctx,
		"cmd.exe", "/c",
		fmt.Sprintf("for /L %%n in (1,1,%d) do @echo %%n", countEnd)))
	require.NoError(t, err)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		// avoid buffered IO so that we can precisely control how many bytes to read.
		n := 1
		for n < countEnd-25 {
			want := fmt.Sprintf("%d\r\n", n)
			// the output also contains virtual terminal sequences
			// so just read until we see the number we want.
			err := readUntil(ctx, want, pc.OutputReader())
			require.NoError(t, err, "want: %s", want)
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
			want := fmt.Sprintf("%d\r\n", n)
			err := readUntil(ctx, want, pc.OutputReader())
			if n < countEnd {
				require.NoError(t, err, "want: %s", want)
			} else {
				require.ErrorIs(t, err, io.EOF)
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
}

// readUntil reads one byte at a time until we either see the string we want, or the context expires
func readUntil(ctx context.Context, want string, r io.Reader) error {
	got := ""
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
				return err
			}
			got = got + string(b)
		case <-ctx.Done():
			return ctx.Err()
		}
		if strings.Contains(got, want) {
			return nil
		}
	}
}
