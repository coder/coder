//go:build windows
// +build windows

package xterminal

import (
	"golang.org/x/sys/windows"
)

// State differs per-platform.
type State struct {
	mode uint32
}

// makeRaw sets the terminal in raw mode and returns the previous state so it can be restored.
func makeRaw(handle windows.Handle, input bool) (uint32, error) {
	var prevState uint32
	if err := windows.GetConsoleMode(handle, &prevState); err != nil {
		return 0, err
	}

	var raw uint32
	if input {
		raw = prevState &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_OUTPUT)
		raw |= windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	} else {
		raw = prevState | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	}

	if err := windows.SetConsoleMode(handle, raw); err != nil {
		return 0, err
	}
	return prevState, nil
}

// MakeInputRaw sets an input terminal to raw and enables VT100 processing.
func MakeInputRaw(handle uintptr) (*State, error) {
	prevState, err := makeRaw(windows.Handle(handle), true)
	if err != nil {
		return nil, err
	}

	return &State{mode: prevState}, nil
}

// MakeOutputRaw sets an output terminal to raw and enables VT100 processing.
func MakeOutputRaw(handle uintptr) (*State, error) {
	prevState, err := makeRaw(windows.Handle(handle), false)
	if err != nil {
		return nil, err
	}

	return &State{mode: prevState}, nil
}

// Restore terminal back to original state.
func Restore(handle uintptr, state *State) error {
	return windows.SetConsoleMode(windows.Handle(handle), state.mode)
}
