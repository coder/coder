//go:build !windows
// +build !windows

package pty

import "golang.org/x/term"

type terminalState *term.State

//nolint:revive
func makeInputRaw(fd uintptr) (*TerminalState, error) {
	s, err := term.MakeRaw(int(fd))
	if err != nil {
		return nil, err
	}
	return &TerminalState{
		state: s,
	}, nil
}

//nolint:revive
func makeOutputRaw(_ uintptr) (*TerminalState, error) {
	// Does nothing. makeInputRaw does enough for both input and output.
	return &TerminalState{
		state: nil,
	}, nil
}

//nolint:revive
func restoreTerminal(fd uintptr, state *TerminalState) error {
	if state == nil || state.state == nil {
		return nil
	}

	return term.Restore(int(fd), state.state)
}
