package pubsub

import (
	"bytes"
	"context"
	"fmt"
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
func (lm *LatencyMeasurer) Measure(ctx context.Context, p Pubsub) (send float64, recv float64, err error) {
	var (
		start        time.Time
		res          = make(chan float64, 1)
		subscribeErr = make(chan error, 1)
	)

	msg := []byte(uuid.New().String())
	log := lm.logger.With(slog.F("msg", msg))

	go func() {
		_, err = p.Subscribe(lm.latencyChannelName(), func(ctx context.Context, in []byte) {
			p := p
			_ = p

			if !bytes.Equal(in, msg) {
				log.Warn(ctx, "received unexpected message!", slog.F("in", in))
				return
			}

			res <- time.Since(start).Seconds()
		})
		if err != nil {
			subscribeErr <- xerrors.Errorf("failed to subscribe: %w", err)
		}
	}()

	start = time.Now()
	err = p.Publish(lm.latencyChannelName(), msg)
	if err != nil {
		return -1, -1, xerrors.Errorf("failed to publish: %w", err)
	}

	send = time.Since(start).Seconds()

	select {
	case <-ctx.Done():
		log.Error(ctx, "context canceled before message could be received", slog.Error(ctx.Err()))
		return send, -1, ctx.Err()
	case val := <-res:
		return send, val, nil
	case err = <-subscribeErr:
		return send, -1, err
	}
}

func (lm *LatencyMeasurer) latencyChannelName() string {
	return fmt.Sprintf("latency-measure:%s", lm.channel)
}
