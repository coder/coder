package dashboard

import (
	"context"
	"time"

	"cdr.dev/slog"

	"golang.org/x/xerrors"
)

type Config struct {
	// Interval is the minimum interval between fetches.
	Interval time.Duration `json:"interval"`
	// Jitter is the maximum interval between fetches.
	Jitter time.Duration `json:"jitter"`
	// Trace is whether to trace the requests.
	Trace bool `json:"trace"`
	// Logger is the logger to use.
	Logger slog.Logger `json:"-"`
	// Headless controls headless mode for chromedp.
	Headless bool `json:"headless"`
	// ActionFunc is a function that returns an action to run.
	ActionFunc func(ctx context.Context, randIntn func(int) int) (Label, Action, error) `json:"-"`
	// RandIntn is a function that returns a random number between 0 and n-1.
	RandIntn func(int) int `json:"-"`
}

func (c Config) Validate() error {
	if !(c.Interval > 0) {
		return xerrors.Errorf("validate interval: must be greater than zero")
	}

	if !(c.Jitter < c.Interval) {
		return xerrors.Errorf("validate jitter: must be less than interval")
	}

	if c.ActionFunc == nil {
		return xerrors.Errorf("validate action func: must not be nil")
	}

	if c.RandIntn == nil {
		return xerrors.Errorf("validate rand intn: must not be nil")
	}

	return nil
}
