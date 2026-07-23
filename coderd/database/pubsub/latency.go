package pubsub

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// LatencyMeasurer is used to measure the send & receive latencies of the underlying Pubsub implementation. We use these
// measurements to export metrics which can indicate when a Pubsub implementation's queue is overloaded and/or full.
type LatencyMeasurer struct {
	// Create unique pubsub channel names so that multiple coderd replicas do not clash when performing latency measurements.
	channel uuid.UUID
	logger  slog.Logger
	// seq distinguishes consecutive measurements from each other so that a
	// subscription whose teardown is still in flight cannot receive (and
	// count) the next measurement's message.
	seq atomic.Int64
}

// LatencyMessageLength is the length of a UUIDv4 encoded to hex.
const LatencyMessageLength = 36

func NewLatencyMeasurer(logger slog.Logger) *LatencyMeasurer {
	return &LatencyMeasurer{
		channel: uuid.New(),
		logger:  logger,
	}
}

// Measure takes a given Pubsub implementation, publishes a message & immediately receives it, and returns the observed latency.
func (lm *LatencyMeasurer) Measure(ctx context.Context, p Pubsub) (send, recv time.Duration, err error) {
	var (
		start time.Time
		res   = make(chan time.Duration, 1)
	)

	msg := []byte(uuid.New().String())
	lm.logger.Debug(ctx, "performing measurement", slog.F("msg", msg))

	channel := lm.nextChannelName()
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

// nextChannelName returns a channel name unique to this measurement.
// Uniqueness across replicas comes from the channel UUID; uniqueness
// across consecutive measurements of the same replica comes from the
// sequence number. The name must stay within Postgres's 63-byte
// identifier limit: 16 (prefix) + 36 (UUID) + 1 (dot) leaves 10 digits
// for the sequence.
func (lm *LatencyMeasurer) nextChannelName() string {
	return fmt.Sprintf("latency-measure:%s.%d", lm.channel, lm.seq.Add(1))
}
