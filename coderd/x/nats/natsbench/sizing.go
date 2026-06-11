package natsbench

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/nats"
)

const (
	// minLocalQueueMsgs matches the nats package's default listener
	// queue capacity.
	minLocalQueueMsgs = 1024
	// maxLocalQueueMsgs caps derived listener queue capacity so a
	// misconfigured plan cannot request absurd allocations.
	maxLocalQueueMsgs = 1 << 20
	// perMessageOverhead approximates per-message NATS protocol
	// overhead (subject, headers, framing) on top of the payload when
	// deriving byte budgets.
	perMessageOverhead = 64
)

// maxExpect returns the largest per-subscriber expected delivery count.
func maxExpect(pl plan) int {
	maxCount := 0
	for _, expect := range pl.expectPerSub {
		maxCount = max(maxCount, expect)
	}
	return maxCount
}

// derivedLocalQueueMsgs sizes the per-listener queue so the busiest
// subscriber can absorb its full burst without local overflow drops.
func derivedLocalQueueMsgs(pl plan) int {
	return min(max(maxExpect(pl), minLocalQueueMsgs), maxLocalQueueMsgs)
}

// derivedMaxPending sizes the embedded server's per-client outbound
// pending byte budget for the worst-case per-subscriber burst, so a
// briefly stalled subscriber connection is not disconnected as a slow
// consumer mid-run.
func derivedMaxPending(pl plan, payloadSize int) int64 {
	burst := int64(maxExpect(pl)) * int64(payloadSize+perMessageOverhead)
	return max(burst, nats.DefaultMaxPending)
}

// applySizing fills LocalQueueMsgs and MaxPending from the plan when
// the caller left them unset, and warns loudly when explicit values are
// below the derived sizes because drops then become likely and any drop
// invalidates the run.
func applySizing(ctx context.Context, logger slog.Logger, cfg Config, pl plan) Config {
	wantQueue := derivedLocalQueueMsgs(pl)
	switch {
	case cfg.LocalQueueMsgs <= 0:
		cfg.LocalQueueMsgs = wantQueue
	case cfg.LocalQueueMsgs < wantQueue:
		logger.Warn(ctx, "configured local queue is below the derived size; drops are likely and will invalidate the run",
			slog.F("configured_msgs", cfg.LocalQueueMsgs),
			slog.F("derived_msgs", wantQueue),
		)
	}

	wantPending := derivedMaxPending(pl, cfg.PayloadSize)
	switch {
	case cfg.MaxPending <= 0:
		cfg.MaxPending = wantPending
	case cfg.MaxPending < wantPending:
		logger.Warn(ctx, "configured max pending is below the derived size; slow-consumer drops are likely and will invalidate the run",
			slog.F("configured_bytes", cfg.MaxPending),
			slog.F("derived_bytes", wantPending),
		)
	}
	return cfg
}
