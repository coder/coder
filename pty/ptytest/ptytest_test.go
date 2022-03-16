package ptytest_test

import (
	"testing"

	"github.com/coder/coder/pty/ptytest"
	"github.com/stretchr/testify/require"
)

func TestPtytest(t *testing.T) {
	t.Parallel()
	t.Run("Echo", func(t *testing.T) {
		pty := ptytest.New(t)
		pty.Output().Write([]byte("write"))
		pty.ExpectMatch("write")
		pty.WriteLine("read")
	})

	t.Run("Newlines", func(t *testing.T) {
		pty := ptytest.New(t)
		pty.WriteLine("echo")
		data := make([]byte, 64)
		read, err := pty.Input().Read(data)
		require.NoError(t, err)
		require.Equal(t, "echo", string(data[:read]))
		read, err = pty.Input().Read(data)
		require.NoError(t, err)
		require.Equal(t, "\r", string(data[:read]))
	})
}
