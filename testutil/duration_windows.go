package testutil

import "time"

// Constants for timing out operations, usable for creating contexts
// that timeout or in require.Eventually.
//
// Windows durations are adjusted for slow CI workers.
const (
	WaitShort     = 10 * time.Second
	WaitMedium    = 20 * time.Second
	WaitLong      = 30 * time.Second
	WaitSuperLong = 60 * time.Second
)

// Constants for delaying repeated operations, e.g. in
// require.Eventually.
//
// Windows durations are adjusted for slow CI workers.
const (
	IntervalFast   = 50 * time.Millisecond
	IntervalMedium = 500 * time.Millisecond
	IntervalSlow   = 2 * time.Second
)
