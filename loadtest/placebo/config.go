package placebo

import (
	"time"

	"golang.org/x/xerrors"
)

type Config struct {
	// Sleep is how long to sleep for. If unspecified, the test run will finish
	// instantly.
	Sleep time.Duration `json:"sleep"`
	// Jitter is the maximum amount of jitter to add to the sleep duration. The
	// sleep value will be increased by a random value between 0 and jitter if
	// jitter is greater than 0.
	Jitter time.Duration `json:"jitter"`
}

func (c Config) Validate() error {
	if c.Sleep < 0 {
		return xerrors.New("sleep must be set to a positive value")
	}
	if c.Jitter < 0 {
		return xerrors.New("jitter must be set to a positive value")
	}
	if c.Jitter > 0 && c.Sleep == 0 {
		return xerrors.New("jitter must be 0 if sleep is 0")
	}

	return nil
}
