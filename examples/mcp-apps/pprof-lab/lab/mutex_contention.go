package lab

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// holdDuration is how long holdAndRelease keeps the mutex. It is far above
// the mutex profile's sampling granularity so every unlock is a candidate
// for recording.
const holdDuration = 300 * time.Microsecond

// mutexContention makes N goroutines fight over one sync.Mutex. The holder
// does busy work while locked, so waiters accumulate delay that the mutex
// profile attributes to the Unlock in holdAndRelease.
type mutexContention struct {
	runner
	mu           sync.Mutex
	scratch      uint64
	goroutines   atomic.Int64
	acquisitions atomic.Int64
}

func newMutexContention() *mutexContention {
	m := &mutexContention{}
	m.name = m.Name()
	return m
}

func (*mutexContention) Name() string { return "mutex-contention" }

func (*mutexContention) Description() string {
	return "N goroutines contend for one sync.Mutex that each holder keeps " +
		"for about 300 microseconds. The mutex profile shows the delay at " +
		"the Unlock in holdAndRelease."
}

func (*mutexContention) Knobs() []Knob {
	return []Knob{{Name: "goroutines", Unit: "goroutines", Default: 64, Min: 2, Max: 1024}}
}

func (m *mutexContention) Start(ctx context.Context, knobs map[string]any) error {
	vals, err := parseKnobs(knobs, m.Knobs())
	if err != nil {
		return err
	}
	n := vals["goroutines"]
	m.restart(ctx, n, func() {
		m.goroutines.Store(int64(n))
		m.acquisitions.Store(0)
	}, func(ctx context.Context, _ int) {
		for ctx.Err() == nil {
			m.holdAndRelease()
		}
	})
	return nil
}

func (m *mutexContention) Stop() {
	m.stop(func() { m.goroutines.Store(0) })
}

func (m *mutexContention) State() map[string]any {
	return map[string]any{
		"goroutines":   m.goroutines.Load(),
		"acquisitions": m.acquisitions.Load(),
	}
}

// holdAndRelease acquires the mutex, spins for holdDuration, and releases
// it. The mutex profile records the contention at the Unlock call.
//
//go:noinline
func (m *mutexContention) holdAndRelease() {
	m.mu.Lock()
	deadline := time.Now().Add(holdDuration)
	x := m.scratch
	for time.Now().Before(deadline) {
		x = x*6364136223846793005 + 1442695040888963407
	}
	m.scratch = x
	m.mu.Unlock()
	m.acquisitions.Add(1)
}
