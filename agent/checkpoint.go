package agent

import (
	"context"
	"runtime"
	"sync"

	"cdr.dev/slog"
)

// checkpoint allows a goroutine to communicate when it is OK to proceed beyond some async condition
// to other dependent goroutines.
type checkpoint struct {
	logger slog.Logger
	mu     sync.Mutex
	called bool
	done   chan struct{}
	err    error
}

// complete the checkpoint.  Pass nil to indicate the checkpoint was ok.  It is an error to call this
// more than once.
func (c *checkpoint) complete(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.called {
		b := make([]byte, 2048)
		n := runtime.Stack(b, false)
		c.logger.Critical(context.Background(), "checkpoint complete called more than once", slog.F("stacktrace", b[:n]))
		return
	}
	c.called = true
	c.err = err
	close(c.done)
}

func (c *checkpoint) wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.err
	}
}

func newCheckpoint(logger slog.Logger) *checkpoint {
	return &checkpoint{
		logger: logger,
		done:   make(chan struct{}),
	}
}
