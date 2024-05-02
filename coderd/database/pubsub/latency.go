package pubsub

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// LatencyMeasurer is used to measure the send & receive latencies of the underlying Pubsub implementation. We use these
// measurements to export metrics which can indicate when a Pubsub implementation's queue is overloaded and/or full.
type LatencyMeasurer struct {
	// Create unique pubsub channel names so that multiple replicas do not clash when performing latency measurements,
	// and only create one UUID per Pubsub impl (and not request) to limit the number of notification channels that need
	// to be maintained by the Pubsub impl.
	channelIDs map[Pubsub]uuid.UUID
}

func NewLatencyMeasurer() *LatencyMeasurer {
	return &LatencyMeasurer{
		channelIDs: make(map[Pubsub]uuid.UUID),
	}
}

// Measure takes a given Pubsub implementation, publishes a message & immediately receives it, and returns the observed latency.
func (lm *LatencyMeasurer) Measure(ctx context.Context, p Pubsub) (send float64, recv float64, err error) {
	var (
		start time.Time
		res   = make(chan float64, 1)
	)

	cancel, err := p.Subscribe(lm.latencyChannelName(p), func(ctx context.Context, _ []byte) {
		res <- time.Since(start).Seconds()
	})
	if err != nil {
		return -1, -1, xerrors.Errorf("failed to subscribe: %w", err)
	}
	defer cancel()

	start = time.Now()
	err = p.Publish(lm.latencyChannelName(p), []byte{})
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

func (lm *LatencyMeasurer) latencyChannelName(p Pubsub) string {
	cid, found := lm.channelIDs[p]
	if !found {
		cid = uuid.New()
		lm.channelIDs[p] = cid
	}

	return fmt.Sprintf("latency-measure:%s", cid.String())
}
