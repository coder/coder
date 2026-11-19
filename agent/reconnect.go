package agent

import (
	"math/rand/v2"
	"time"
)

const (
	// reconnectBackoffFloor is the upper bound of the first delay after a
	// failed attempt or a lost connection. It is short so that a brief
	// interruption barely delays the agent.
	reconnectBackoffFloor = 100 * time.Millisecond
	// reconnectBackoffCeil is the upper bound the delays grow to. During a
	// long outage every agent retries on average once per half of it, and
	// when coderd returns the agents arrive spread over about this long.
	reconnectBackoffCeil = 30 * time.Second
	// reconnectHealthyAfter is how long a connection must last before the
	// next disconnect starts again from the shortest delays. A connection
	// that drops sooner counts as another failure, so that an overloaded
	// coderd that keeps dropping agents is not hit by immediate reconnects.
	reconnectHealthyAfter = time.Minute
	// reconnectMaxRetryAfter caps the delay a server can ask for, so that a
	// misconfigured proxy cannot keep the agent away for long.
	reconnectMaxRetryAfter = time.Minute
)

// reconnectBackoff picks how long the agent waits before each attempt to
// connect to coderd. Every delay is drawn uniformly between zero and a limit
// that doubles with each consecutive failure. Agents that lose coderd at the
// same moment therefore come back spread over time rather than together,
// which is what lets coderd recover from an outage without being overwhelmed.
type reconnectBackoff struct {
	// limit is the upper bound of the next delay. It is zero until the first
	// attempt, so that a starting agent connects immediately.
	limit time.Duration
}

// next returns how long to wait before the next attempt. connectedFor is how
// long the previous attempt stayed connected, and retryAfter is the delay the
// server asked for when it turned the previous attempt away. Both are zero if
// they do not apply.
func (b *reconnectBackoff) next(connectedFor, retryAfter time.Duration) time.Duration {
	if connectedFor >= reconnectHealthyAfter {
		b.limit = reconnectBackoffFloor
	}

	// The requested delay is a floor, with the random delay added on top.
	// Otherwise agents that were all told to wait the same time would
	// return together.
	delay := min(max(retryAfter, 0), reconnectMaxRetryAfter)
	if b.limit > 0 {
		// #nosec G404 - backoff jitter does not need a cryptographic source.
		delay += rand.N(b.limit + 1)
	}

	b.limit = min(reconnectBackoffCeil, max(reconnectBackoffFloor, 2*b.limit))
	return delay
}
