package pubsub

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

var channelID uuid.UUID

// Create a new pubsub channel UUID per coderd instance so that multiple replicas do not clash when performing latency
// measurements, and only create one UUID per instance (and not request) to limit the number of notification channels
// that need to be maintained by the Pubsub implementation.
func init() {
	channelID = uuid.New()
}

// MeasureLatency takes a given Pubsub implementation, publishes a message & immediately receives it, and returns the
// observed latency.
func MeasureLatency(ctx context.Context, p Pubsub) (send float64, recv float64, err error) {
	var (
		start time.Time
		res   = make(chan float64, 1)
	)

	cancel, err := p.Subscribe(latencyChannelName(), func(ctx context.Context, _ []byte) {
		res <- time.Since(start).Seconds()
	})
	if err != nil {
		return -1, -1, xerrors.Errorf("failed to subscribe: %w", err)
	}
	defer cancel()

	start = time.Now()
	err = p.Publish(latencyChannelName(), []byte{})
	if err != nil {
		return -1, -1, xerrors.Errorf("failed to publish: %w", err)
	}

	send = time.Since(start).Seconds()

	select {
	case <-ctx.Done():
		return send, -1, ctx.Err()
	case val := <-res:
		return send, val, nil
	}
}

func latencyChannelName() string {
	return fmt.Sprintf("latency-measure:%s", channelID.String())
}
