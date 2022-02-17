package pty_test

import (
	"os/exec"
	"testing"

	"github.com/coder/coder/pty/ptytest"
)

func TestStart(t *testing.T) {
	t.Run("Echo", func(t *testing.T) {
		pty := ptytest.Start(t, exec.Command("echo", "test"))
		pty.ExpectMatch("test")
	})
}
