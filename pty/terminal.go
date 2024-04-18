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
