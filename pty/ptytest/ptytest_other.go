//go:build !windows
// +build !windows

package ptytest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/coder/coder/pty"
)

func initializePTY(t *testing.T, ptty pty.PTY) {
	// On non-Windows operating systems, the pseudo-terminal
	// converts a carriage-return into a newline.
	//
	// We send unescaped input, so any terminal translations
	// result in inconsistent behavior.
	ptyFile, valid := ptty.Input().Reader.(*os.File)
	require.True(t, valid, "The pty input must be a file!")
	ptyFileFd := int(ptyFile.Fd())

	state, err := unix.IoctlGetTermios(ptyFileFd, unix.TCGETS)
	require.NoError(t, err)
	state.Iflag &^= unix.ICRNL
	err = unix.IoctlSetTermios(ptyFileFd, unix.TCSETS, state)
	require.NoError(t, err)
}
