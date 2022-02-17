package ptytest_test

import (
	"testing"

	"github.com/coder/coder/pty/ptytest"
)

func TestPtytest(t *testing.T) {
	t.Parallel()
	pty := ptytest.New(t)
	pty.Output().Write([]byte("write"))
	pty.ExpectMatch("write")
	pty.WriteLine("read")
}
