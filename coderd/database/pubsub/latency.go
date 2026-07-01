package pubsub

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// LatencyMeasurer is used to measure the send & receive latencies of the underlying Pubsub implementation. We use these
// measurements to export metrics which can indicate when a Pubsub implementation's queue is overloaded and/or full.
type LatencyMeasurer struct {
	logger slog.Logger
}

// LatencyMessageLength is the length of a UUIDv4 encoded to hex.
const LatencyMessageLength = 36

func NewLatencyMeasurer(logger slog.Logger) *LatencyMeasurer {
	return &LatencyMeasurer{
		logger: logger,
	}
}

// Measure takes a given Pubsub implementation, publishes a message & immediately receives it, and returns the observed latency.
func (lm *LatencyMeasurer) Measure(ctx context.Context, p Pubsub) (send, recv time.Duration, err error) {
	var (
		start time.Time
		res   = make(chan time.Duration, 1)
	)

	// Use a fresh, unique channel name for every measurement. A stable
	// per-measurer channel lets an asynchronous unsubscribe from a prior
	// measurement overlap with the next one on the same channel, which
	// delivers the latency probe to the stale subscription and double-counts
	// received-message metrics. Generating the name per measurement also
	// keeps multiple coderd replicas from clashing when measuring latency.
	channel := latencyChannelName(uuid.New())

	msg := []byte(uuid.New().String())
	lm.logger.Debug(ctx, "performing measurement", slog.F("msg", msg))

	cancel, err := p.Subscribe(channel, func(ctx context.Context, in []byte) {
		if !bytes.Equal(in, msg) {
			lm.logger.Warn(ctx, "received unexpected message", slog.F("got", in), slog.F("expected", msg))
			return
		}

		res <- time.Since(start)
	})
	if err != nil {
		return -1, -1, xerrors.Errorf("failed to subscribe: %w", err)
	}
	defer cancel()

	start = time.Now()
	err = p.Publish(channel, msg)
	if err != nil {
		return -1, -1, xerrors.Errorf("failed to publish: %w", err)
	}

	send = time.Since(start)
	select {
	case <-ctx.Done():
		lm.logger.Error(ctx, "context canceled before message could be received", slog.Error(ctx.Err()), slog.F("msg", msg))
		return send, -1, ctx.Err()
	case recv = <-res:
		return send, recv, nil
	}
}

// latencyChannelName builds the pubsub channel used for a single latency
// measurement. The format is kept to "latency-measure:" plus one UUID (52
// bytes) so the name stays within Postgres' 63-byte identifier limit for the
// LISTEN/NOTIFY-backed pubsub, which shares this measurer.
func latencyChannelName(id uuid.UUID) string {
	return fmt.Sprintf("latency-measure:%s", id)
}
