package usage

import (
	"sync"
	"time"
)

// PublishHealth tracks publisher outcomes observed by this process.
type PublishHealth struct {
	mu                      sync.RWMutex
	epoch                   uint64
	lastPublishedAt         time.Time
	failureStartedAt        time.Time
	postPublishUpdateFailed bool
}

// PublishHealthSnapshot is a point-in-time copy of publisher health.
type PublishHealthSnapshot struct {
	LastPublishedAt  time.Time
	FailureStartedAt time.Time
}

func (h *PublishHealth) currentEpoch() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.epoch
}

func (h *PublishHealth) recordCycleFailure(epoch uint64, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.epoch != epoch {
		return
	}
	if h.failureStartedAt.IsZero() {
		h.failureStartedAt = now
	}
}

func (h *PublishHealth) recordCycleHealthy(epoch uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.epoch == epoch && !h.postPublishUpdateFailed {
		h.failureStartedAt = time.Time{}
	}
}

func (h *PublishHealth) recordCyclePostPublishUpdateFailure(epoch uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.epoch == epoch {
		h.postPublishUpdateFailed = true
	}
}

func (h *PublishHealth) recordCyclePostPublishUpdateSuccess(epoch uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.epoch == epoch {
		h.postPublishUpdateFailed = false
	}
}

func (h *PublishHealth) recordCyclePublished(epoch uint64, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.epoch == epoch {
		h.lastPublishedAt = now
	}
}

// RecordFailure starts a failure streak if one is not already active.
func (h *PublishHealth) RecordFailure(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.failureStartedAt.IsZero() {
		h.failureStartedAt = now
	}
}

// RecordHealthy clears the active failure streak.
func (h *PublishHealth) RecordHealthy() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failureStartedAt = time.Time{}
	h.postPublishUpdateFailed = false
}

// RecordPublished records a successfully persisted publish.
func (h *PublishHealth) RecordPublished(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastPublishedAt = now
}

// Reset clears all publisher outcomes observed by this process.
func (h *PublishHealth) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.epoch++
	h.lastPublishedAt = time.Time{}
	h.failureStartedAt = time.Time{}
	h.postPublishUpdateFailed = false
}

// Snapshot returns a copy of the current publisher health.
func (h *PublishHealth) Snapshot() PublishHealthSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return PublishHealthSnapshot{
		LastPublishedAt:  h.lastPublishedAt,
		FailureStartedAt: h.failureStartedAt,
	}
}
