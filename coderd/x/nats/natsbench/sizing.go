package main

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
	// probeHeadroom pads message-count budgets so in-flight readiness
	// probes from the gate's final iteration cannot consume capacity
	// that the benchmark burst was sized for.
	probeHeadroom = 64
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
// subscriber can absorb its full burst, plus probe headroom, without
// local overflow drops. The cap can defeat that guarantee for huge
// plans; applySizing warns when it binds.
func derivedLocalQueueMsgs(pl plan) int {
	return min(max(maxExpect(pl)+probeHeadroom, minLocalQueueMsgs), maxLocalQueueMsgs)
}

// derivedMaxPending sizes the embedded server's per-client outbound
// pending byte budget so a briefly stalled subscriber connection is not
// disconnected as a slow consumer mid-run. MaxPending is per client
// connection, and one subscribe connection carries every coalesced
// subscription on its node, so the budget is the worst per-node sum of
// subject volumes with local subscribers. With SubscribeConns > 1 this
// is conservative (subjects spread across connections).
func derivedMaxPending(pl plan, payloadSize int) int64 {
	perSubject := perSubjectMsgs(pl)
	nodeBurst := make(map[int]int64)
	// Each coalesced subscription delivers every subject message once
	// per node, regardless of how many local subscribers share it.
	for _, node := range pl.subNodes {
		seen := make(map[int]struct{})
		for j, subNode := range pl.subNode {
			if subNode != node {
				continue
			}
			subject := pl.subSubject[j]
			if _, ok := seen[subject]; ok {
				continue
			}
			seen[subject] = struct{}{}
			nodeBurst[node] += int64(perSubject[subject]+probeHeadroom) * int64(payloadSize+perMessageOverhead)
		}
	}
	burst := int64(0)
	for _, b := range nodeBurst {
		burst = max(burst, b)
	}
	return max(burst, nats.DefaultMaxPending)
}

// derivedQueueBytes sizes the per-subscription NATS pending byte limit
// for the busiest subject's full burst plus probe headroom.
func derivedQueueBytes(pl plan, payloadSize int) int {
	return (maxExpect(pl) + probeHeadroom) * (payloadSize + perMessageOverhead)
}

// perSubjectMsgs returns the total benchmark messages published to each
// subject index.
func perSubjectMsgs(pl plan) map[int]int {
	out := make(map[int]int)
	for i, subject := range pl.pubSubject {
		out[subject] += pl.perPubMsgs[i]
	}
	return out
}

// applySizing fills LocalQueueMsgs and MaxPending from the plan when
// the caller left them unset, and warns loudly when explicit values are
// below the derived sizes because drops then become likely and any drop
// invalidates the run.
func applySizing(ctx context.Context, logger slog.Logger, cfg Config, pl plan) Config {
	wantQueue := derivedLocalQueueMsgs(pl)
	if wantQueue == maxLocalQueueMsgs && maxExpect(pl)+probeHeadroom > maxLocalQueueMsgs {
		logger.Warn(ctx, "derived local queue hit its cap; the burst exceeds queue capacity and drops may invalidate the run",
			slog.F("burst_msgs", maxExpect(pl)),
			slog.F("cap_msgs", maxLocalQueueMsgs),
		)
	}
	switch {
	case cfg.LocalQueueMsgs <= 0:
		cfg.LocalQueueMsgs = wantQueue
	case cfg.LocalQueueMsgs < wantQueue:
		logger.Warn(ctx, "configured local queue is below the derived size; drops are likely and will invalidate the run",
			slog.F("configured_msgs", cfg.LocalQueueMsgs),
			slog.F("derived_msgs", wantQueue),
		)
	}
	if cfg.LocalQueueBytes <= 0 {
		cfg.LocalQueueBytes = derivedQueueBytes(pl, cfg.PayloadSize)
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
