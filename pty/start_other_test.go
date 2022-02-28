//go:build !windows
// +build !windows

package pty_test

import (
	"os/exec"
	"testing"

	"go.uber.org/goleak"

	"github.com/coder/coder/pty/ptytest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestStart(t *testing.T) {
	t.Parallel()
	t.Run("Echo", func(t *testing.T) {
		t.Parallel()
		pty, _ := ptytest.Start(t, exec.Command("echo", "test"))
		pty.ExpectMatch("test")
	})
}
