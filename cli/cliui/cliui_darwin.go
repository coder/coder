//go:build darwin
// +build darwin

package cliui

import (
	"golang.org/x/sys/unix"

	"golang.org/x/xerrors"
)

func removeLineLengthLimit(inputFD int) (func(), error) {
	termios, err := unix.IoctlGetTermios(inputFD, unix.TIOCGETA)
	if err != nil {
		return nil, xerrors.Errorf("get termios: %w", err)
	}
	newState := *termios
	// MacOS has a default line limit of 1024. See:
	// https://unix.stackexchange.com/questions/204815/terminal-does-not-accept-pasted-or-typed-lines-of-more-than-1024-characters
	//
	// This removes canonical input processing, so deletes will not function
	// as expected. This _seems_ fine for most use-cases, but is unfortunate.
	newState.Lflag &^= unix.ICANON
	err = unix.IoctlSetTermios(inputFD, unix.TIOCSETA, &newState)
	if err != nil {
		return nil, xerrors.Errorf("set termios: %w", err)
	}
	return func() {
		_ = unix.IoctlSetTermios(inputFD, unix.TIOCSETA, termios)
	}, nil
}
