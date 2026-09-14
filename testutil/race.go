package testutil

// RaceEnabled returns whether the race detector is enabled.
// This is a constant at compile time. It should be used to
// conditionally skip tests that are known to be sensitive to
// being run with the race detector enabled.
// Please use sparingly and as a last resort.
func RaceEnabled() bool {
	return raceEnabled
}
