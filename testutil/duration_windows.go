package testutil

import "time"

// Constants for timing out operations, usable for creating contexts
// that timeout or in require.Eventually.
//
// Windows durations are adjusted for slow CI workers.
const (
	WaitShort     = 30 * time.Second
	WaitMedium    = 40 * time.Second
	WaitLong      = 70 * time.Second
	WaitSuperLong = 240 * time.Second
)

// Constants for delaying repeated operations, e.g. in
// require.Eventually.
//
// Windows durations are adjusted for slow CI workers.
const (
	IntervalFast   = 100 * time.Millisecond
	IntervalMedium = 1000 * time.Millisecond
	IntervalSlow   = 4 * time.Second
)
