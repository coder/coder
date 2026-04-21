package testutil

import "time"

// Shared test timeout and interval constants.
// Using named constants avoids magic numbers and makes timeout policy
// easy to adjust across the entire test suite.
const (
	// WaitLong is the default timeout for test operations that may take a while
	// (e.g. integration tests with HTTP round-trips).
	WaitLong = 30 * time.Second

	// WaitMedium is a timeout for moderately slow operations.
	WaitMedium = 10 * time.Second

	// WaitShort is a timeout for operations expected to complete quickly.
	WaitShort = 5 * time.Second

	// IntervalFast is a short polling interval for require.Eventually and similar.
	IntervalFast = 50 * time.Millisecond
)
