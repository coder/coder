//go:build windows
// +build windows

package pty_test

import (
	"bytes"
	"context"
	"io"
	"os/exec"
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
	assert.Equal(t, "test", b.String())

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
