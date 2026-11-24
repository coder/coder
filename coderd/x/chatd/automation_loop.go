package chatd

import (
	"context"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

// automationTickInterval is how often each replica looks for automation
// work.
const automationTickInterval = 30 * time.Second

// automationWork is one kind of work that the automation loop runs every
// tick. Each kind claims its own rows, so replicas share the work.
type automationWork struct {
	name string
	tick func(ctx context.Context, now time.Time)
}

// automationLoop runs every kind of automation work at start and then every
// automationTickInterval.
func (w *chatWorker) automationLoop(ctx context.Context) {
	work := []automationWork{
		{name: "schedule", tick: w.server.tickSchedules},
	}
	run := func(now time.Time) {
		for _, kind := range work {
			w.runAutomationWork(ctx, kind, dbtime.Time(now))
		}
	}
	run(w.opts.Clock.Now("chatworker", "automations"))

	ticker := w.opts.Clock.NewTicker(automationTickInterval, "chatworker", "automations")
	defer ticker.Stop("chatworker", "automations")
	for {
		select {
		case now := <-ticker.C:
			run(now)
		case <-ctx.Done():
			return
		}
	}
}

// runAutomationWork keeps a panic in one kind of work from stopping the
// loop and the other kinds.
func (w *chatWorker) runAutomationWork(ctx context.Context, kind automationWork, now time.Time) {
	defer func() {
		if recovered := recover(); recovered != nil {
			w.opts.Logger.Error(ctx, "chat automation work panicked",
				slog.F("work", kind.name),
				slog.F("panic", recovered),
			)
		}
	}()
	kind.tick(ctx, now)
}
