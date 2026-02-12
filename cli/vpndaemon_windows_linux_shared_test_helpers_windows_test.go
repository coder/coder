//go:build windows

package cli_test

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func dupHandle(t *testing.T, f *os.File) uintptr {
	t.Helper()

	src := syscall.Handle(f.Fd())
	var dup syscall.Handle
	err := syscall.DuplicateHandle(
		syscall.GetCurrentProcess(),
		src,
		syscall.GetCurrentProcess(),
		&dup,
		0,
		false,
		syscall.DUPLICATE_SAME_ACCESS,
	)
	require.NoError(t, err)
	return uintptr(dup)
}
