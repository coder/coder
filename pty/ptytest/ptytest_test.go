package ptytest_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/pty/ptytest"
)

func TestPtytest(t *testing.T) {
	t.Parallel()
	t.Run("Echo", func(t *testing.T) {
		t.Parallel()
		pty := ptytest.New(t)
		pty.Output().Write([]byte("write"))
		pty.ExpectMatch("write")
		pty.WriteLine("read")
	})
	// nolint:paralleltest
	t.Run("Do not hang on Intel macOS", func(t *testing.T) {
		cmd := exec.Command("sh", "-c", "for i in $(seq 1 1000); do echo $i; done")
		pty := ptytest.New(t)
		cmd.Stdin = pty.Input()
		cmd.Stdout = pty.Output()
		err := cmd.Run()
		require.NoError(t, err)
	})
}
