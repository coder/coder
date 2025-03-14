package pubsub
import (
	"errors"
	"bytes"
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
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
func (lm *LatencyMeasurer) Measure(ctx context.Context, p Pubsub) (send, recv time.Duration, err error) {
	var (
		start time.Time
		res   = make(chan time.Duration, 1)
	)
	msg := []byte(uuid.New().String())
	lm.logger.Debug(ctx, "performing measurement", slog.F("msg", msg))
	cancel, err := p.Subscribe(lm.latencyChannelName(), func(ctx context.Context, in []byte) {
		if !bytes.Equal(in, msg) {
			lm.logger.Warn(ctx, "received unexpected message", slog.F("got", in), slog.F("expected", msg))
			return
		}
		res <- time.Since(start)
	})
	if err != nil {
		return -1, -1, fmt.Errorf("failed to subscribe: %w", err)
	}
	defer cancel()
	start = time.Now()
	err = p.Publish(lm.latencyChannelName(), msg)
	if err != nil {
		return -1, -1, fmt.Errorf("failed to publish: %w", err)
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
func (lm *LatencyMeasurer) latencyChannelName() string {
	return fmt.Sprintf("latency-measure:%s", lm.channel)
}
