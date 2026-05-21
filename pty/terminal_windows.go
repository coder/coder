//go:build windows
// +build windows

package pty

import "golang.org/x/sys/windows"

type terminalState uint32

// This is adapted from term.MakeRaw, but adds
// ENABLE_VIRTUAL_TERMINAL_PROCESSING to the output mode and
// ENABLE_VIRTUAL_TERMINAL_INPUT to the input mode.
//
// See: https://github.com/golang/term/blob/5b15d269ba1f54e8da86c8aa5574253aea0c2198/term_windows.go#L23
//
// Copyright 2019 The Go Authors. BSD-3-Clause license. See:
// https://github.com/golang/term/blob/master/LICENSE
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

//nolint:revive
func makeInputRaw(handle uintptr) (*TerminalState, error) {
	prevState, err := makeRaw(windows.Handle(handle), true)
	if err != nil {
		return nil, err
	}

	return &TerminalState{
		state: terminalState(prevState),
	}, nil
}

//nolint:revive
func makeOutputRaw(handle uintptr) (*TerminalState, error) {
	prevState, err := makeRaw(windows.Handle(handle), false)
	if err != nil {
		return nil, err
	}

	return &TerminalState{
		state: terminalState(prevState),
	}, nil
}

//nolint:revive
func restoreTerminal(handle uintptr, state *TerminalState) error {
	return windows.SetConsoleMode(windows.Handle(handle), uint32(state.state))
}
