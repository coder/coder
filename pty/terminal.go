package pty

// TerminalState differs per-platform.
type TerminalState struct {
	state terminalState
}

// MakeInputRaw calls term.MakeRaw on non-Windows platforms. On Windows it sets
// special terminal modes that enable VT100 emulation as well as setting the
// same modes that term.MakeRaw sets.
//
//nolint:revive
func MakeInputRaw(fd uintptr) (*TerminalState, error) {
	return makeInputRaw(fd)
}

// MakeInputRawNoVT puts the terminal into raw mode but, unlike
// MakeInputRaw, does NOT enable VT input processing on Windows.
// This avoids problems with pasted text being wrapped in ANSI
// escape sequences (e.g. bracketed-paste markers) that would
// corrupt password or token input. On non-Windows platforms the
// behavior is identical to MakeInputRaw.
//
//nolint:revive
func MakeInputRawNoVT(fd uintptr) (*TerminalState, error) {
	return makeInputRawNoVT(fd)
}

// MakeOutputRaw does nothing on non-Windows platforms. On Windows it sets
// special terminal modes that enable VT100 emulation as well as setting the
// same modes that term.MakeRaw sets.
//
//nolint:revive
func MakeOutputRaw(fd uintptr) (*TerminalState, error) {
	return makeOutputRaw(fd)
}

// RestoreTerminal restores the terminal back to its original state.
//
//nolint:revive
func RestoreTerminal(fd uintptr, state *TerminalState) error {
	return restoreTerminal(fd, state)
}
