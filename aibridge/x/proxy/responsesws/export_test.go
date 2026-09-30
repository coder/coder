package responsesws

import "context"

// StateSizes returns the number of entries in each of the session's
// bookkeeping maps that holds any, keyed by map name.
func StateSizes(s *Session) map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	sizes := map[string]int{
		"open":      len(s.open),
		"pending":   len(s.pending),
		"active":    len(s.active),
		"responses": len(s.responses),
	}
	for name, n := range sizes {
		if n == 0 {
			delete(sizes, name)
		}
	}
	return sizes
}

// QueuedEvents returns the number of synthesized events waiting for Recv.
func QueuedEvents(s *Session) int {
	return len(s.errors)
}

// Drain waits until the accountant processed every accounting job queued
// before the call, or exited.
func Drain(ctx context.Context, s *Session) error {
	done := make(chan struct{})
	s.queue.pushBarrier(done)
	select {
	case <-done:
		return nil
	case <-s.accountDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SetQueueBounds sets the accounting queue bounds. Call it before traffic.
func SetQueueBounds(s *Session, jobs, bytes int) {
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	s.queue.maxJobs, s.queue.maxBytes = jobs, bytes
}
