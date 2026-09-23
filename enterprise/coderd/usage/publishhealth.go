package usage

import (
	"sync"
	"time"
)

// PublishHealth tracks publisher outcomes observed by this process.
type PublishHealth struct {
	mu                            sync.RWMutex
	epoch                         uint64
	lastPublishedAt               time.Time
	publishFailureStartedAt       time.Time
	localDatabaseFailureStartedAt time.Time
}

// PublishHealthSnapshot is a point-in-time copy of publisher health.
type PublishHealthSnapshot struct {
	LastPublishedAt time.Time
	FailureStartedAt time.Time
	LocalDatabaseFailureStartedAt time.Time
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
	if h.publishFailureStartedAt.IsZero() {
		h.publishFailureStartedAt = now
	}
}

func (h *PublishHealth) recordCycleHealthy(epoch uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.epoch == epoch {
		h.publishFailureStartedAt = time.Time{}
	}
}

func (h *PublishHealth) recordLocalDatabaseFailure(epoch uint64, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.epoch != epoch {
		return
	}
	if h.localDatabaseFailureStartedAt.IsZero() {
		h.localDatabaseFailureStartedAt = now
	}
}

func (h *PublishHealth) recordLocalDatabaseSuccess(epoch uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.epoch == epoch {
		h.localDatabaseFailureStartedAt = time.Time{}
	}
}

func (h *PublishHealth) recordCyclePublished(epoch uint64, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.epoch == epoch {
		h.lastPublishedAt = now
	}
}

// Reset clears all publisher outcomes observed by this process.
func (h *PublishHealth) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.epoch++
	h.lastPublishedAt = time.Time{}
	h.publishFailureStartedAt = time.Time{}
	h.localDatabaseFailureStartedAt = time.Time{}
}

// Snapshot returns a copy of the current publisher health.
func (h *PublishHealth) Snapshot() PublishHealthSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return PublishHealthSnapshot{
		LastPublishedAt:               h.lastPublishedAt,
		FailureStartedAt:              earliestNonZeroTime(h.publishFailureStartedAt, h.localDatabaseFailureStartedAt),
		LocalDatabaseFailureStartedAt: h.localDatabaseFailureStartedAt,
	}
}

func earliestNonZeroTime(a, b time.Time) time.Time {
	switch {
	case a.IsZero():
		return b
	case b.IsZero(), !b.Before(a):
		return a
	default:
		return b
	}
}
