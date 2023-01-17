//go:build !windows

package testutil

import "time"

// Constants for timing out operations, usable for creating contexts
// that timeout or in require.Eventually.
const (
	WaitShort     = 5 * time.Second
	WaitMedium    = 10 * time.Second
	WaitLong      = 15 * time.Second
	WaitSuperLong = 60 * time.Second
)

// Constants for delaying repeated operations, e.g. in
// require.Eventually.
const (
	IntervalFast   = 25 * time.Millisecond
	IntervalMedium = 250 * time.Millisecond
	IntervalSlow   = time.Second
)
