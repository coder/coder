package pubsub

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// LatencyMeasurer is used to measure the send & receive latencies of the underlying Pubsub implementation. We use these
// measurements to export metrics which can indicate when a Pubsub implementation's queue is overloaded and/or full.
type LatencyMeasurer struct {
	// Create unique pubsub channel names so that multiple coderd replicas do not clash when performing latency measurements.
	channel uuid.UUID
	logger  slog.Logger

	collections atomic.Int64
	last        atomic.Value
}

type LatencyMeasurement struct {
	Send, Recv time.Duration
	Err        error
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
func (lm *LatencyMeasurer) Measure(ctx context.Context, p Pubsub) LatencyMeasurement {
	var (
		start time.Time
		res   = make(chan time.Duration, 1)
	)

	msg := []byte(uuid.New().String())

	cancel, err := p.Subscribe(lm.latencyChannelName(), func(ctx context.Context, in []byte) {
		if !bytes.Equal(in, msg) {
			lm.logger.Warn(ctx, "received unexpected message", slog.F("got", in), slog.F("expected", msg))
			return
		}

		res <- time.Since(start)
	})
	if err != nil {
		return LatencyMeasurement{Send: -1, Recv: -1, Err: xerrors.Errorf("failed to subscribe: %w", err)}
	}
	defer cancel()

	start = time.Now()
	err = p.Publish(lm.latencyChannelName(), msg)
	if err != nil {
		return LatencyMeasurement{Send: -1, Recv: -1, Err: xerrors.Errorf("failed to publish: %w", err)}
	}

	send := time.Since(start)

	select {
	case <-ctx.Done():
		lm.logger.Error(ctx, "context canceled before message could be received", slog.Error(ctx.Err()), slog.F("msg", msg))
		return LatencyMeasurement{Send: send, Recv: -1, Err: ctx.Err()}
	case recv := <-res:
		return LatencyMeasurement{Send: send, Recv: recv}
	}
}

// MeasureAsync runs latency measurements asynchronously on a given interval.
// This function is expected to be run in a goroutine and will exit when the context is canceled.
func (lm *LatencyMeasurer) MeasureAsync(ctx context.Context, p Pubsub, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()

	for ; true; <-tick.C { // tick immediately
		select {
		case <-ctx.Done():
			return
		default:
			if p == nil {
				lm.logger.Error(ctx, "given pubsub is nil")
				return
			}
		}

		lm.collections.Add(1)
		measure := lm.Measure(ctx, p)
		lm.last.Store(&measure)
	}
}

func (lm *LatencyMeasurer) LastMeasurement() *LatencyMeasurement {
	val := lm.last.Load()
	if val == nil {
		return nil
	}

	// nolint:forcetypeassert // Unnecessary type check.
	return val.(*LatencyMeasurement)
}

func (lm *LatencyMeasurer) MeasurementCount() int64 {
	return lm.collections.Load()
}

func (lm *LatencyMeasurer) latencyChannelName() string {
	return fmt.Sprintf("latency-measure:%s", lm.channel)
}
