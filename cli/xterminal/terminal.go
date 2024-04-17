//go:build !windows
// +build !windows

package xterminal

import (
	"golang.org/x/term"
)

// State differs per-platform.
type State struct {
	s *term.State
}

// MakeInputRaw calls term.MakeRaw on non-Windows platforms.
func MakeInputRaw(fd uintptr) (*State, error) {
	s, err := term.MakeRaw(int(fd))
	if err != nil {
		return nil, err
	}
	return &State{
		s: s,
	}, nil
}

// MakeOutputRaw does nothing on non-Windows platforms.
func MakeOutputRaw(_ uintptr) (*State, error) {
	return &State{
		s: nil,
	}, nil
}

// Restore terminal back to original state.
func Restore(fd uintptr, state *State) error {
	if state == nil || state.s == nil {
		return nil
	}

	return term.Restore(int(fd), state.s)
}
