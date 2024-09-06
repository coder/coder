package reports

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/quartz"
)

const (
	delay = 5 * time.Minute
)

func NewReportGenerator(ctx context.Context, logger slog.Logger, db database.Store, clk quartz.Clock) io.Closer {
	closed := make(chan struct{})

	ctx, cancelFunc := context.WithCancel(ctx)
	//nolint:gocritic // The system generates periodic reports without direct user input.
	ctx = dbauthz.AsSystemRestricted(ctx)

	// Start the ticker with the initial delay.
	ticker := clk.NewTicker(delay)
	doTick := func(start time.Time) {
		defer ticker.Reset(delay)
	}

	go func() {
		defer close(closed)
		defer ticker.Stop()
		// Force an initial tick.
		doTick(dbtime.Time(clk.Now()).UTC())
		for {
			select {
			case <-ctx.Done():
				return
			case tick := <-ticker.C:
				ticker.Stop()
				doTick(dbtime.Time(tick).UTC())
			}
		}
	}()
	return &reportGenerator{
		cancel: cancelFunc,
		closed: closed,
	}
}

type reportGenerator struct {
	cancel context.CancelFunc
	closed chan struct{}
}

func (i *reportGenerator) Close() error {
	i.cancel()
	<-i.closed
	return nil
}
