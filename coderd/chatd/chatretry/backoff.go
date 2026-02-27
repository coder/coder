package chatretry

import "time"

const (
	// InitialDelay is the backoff duration for the first retry
	// attempt.
	InitialDelay = 1 * time.Second

	// MaxDelay is the upper bound for the exponential backoff
	// duration. Matches the cap used in coder/mux.
	MaxDelay = 60 * time.Second
)

// Delay returns the backoff duration for the given 0-indexed attempt.
// Uses exponential backoff: min(InitialDelay * 2^attempt, MaxDelay).
// Matches the backoff curve used in coder/mux.
func Delay(attempt int) time.Duration {
	d := InitialDelay
	for range attempt {
		d *= 2
		if d >= MaxDelay {
			return MaxDelay
		}
	}
	return d
}
