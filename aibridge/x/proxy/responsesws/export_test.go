package responsesws

import "context"

// StateSizes returns the number of entries in each of the session's
// bookkeeping maps and lists that holds any, keyed by name.
func StateSizes(s *Session) map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	sizes := map[string]int{
		"open":       len(s.open),
		"pending":    len(s.pending),
		"active":     len(s.active),
		"responses":  len(s.responses),
		"overloaded": len(s.overloaded),
		"toolCalls":  len(s.toolCalls),
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

// MaxBufferedFrameBytes is the byte bound of frames read ahead of Recv.
const MaxBufferedFrameBytes = maxBufferedFrameBytes

// BufferedFrameBytes returns the bytes of the frames read ahead of Recv.
func BufferedFrameBytes(s *Session) int {
	s.frameBytes.mu.Lock()
	defer s.frameBytes.mu.Unlock()
	return s.frameBytes.used
}
