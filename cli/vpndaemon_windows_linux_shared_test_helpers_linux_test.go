//go:build linux

package cli_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func dupHandle(t *testing.T, f *os.File) uintptr {
	t.Helper()

	dupFD, err := unix.Dup(int(f.Fd()))
	require.NoError(t, err)
	return uintptr(dupFD)
}
