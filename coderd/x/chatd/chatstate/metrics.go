package chatstate

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics records one observation per [ChatMachine.Update] so operators
// can see how many transactions each transition shape costs and where the
// time goes: waiting for a pooled connection, waiting for the chat row
// lock, running the callback while holding the row, and committing.
// A nil *Metrics is a no-op.
type Metrics struct {
	duration *prometheus.HistogramVec
	phase    *prometheus.HistogramVec
}

// NewMetrics creates and registers the transition metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	buckets := prometheus.ExponentialBuckets(0.0005, 2, 15)
	m := &Metrics{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "transition_duration_seconds",
			Help:      "Wall time of one ChatMachine.Update from before BeginTx to after commit or rollback. The transition label lists the transitions the callback ran, joined by '+'.",
			Buckets:   buckets,
		}, []string{"transition", "outcome"}),
		phase: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "transition_phase_seconds",
			Help:      "Wall time of each phase of one ChatMachine.Update: pre_lock (BeginTx, including waiting for a pooled connection), lock_wait (the row lock statement), callback (statements run while holding the row), commit (COMMIT or ROLLBACK).",
			Buckets:   buckets,
		}, []string{"transition", "outcome", "phase"}),
	}
	reg.MustRegister(m.duration, m.phase)
	return m
}

// transitionPhases are the boundaries Update records for one transaction.
// A zero time means the phase did not start (for example when BeginTx
// failed), and its duration is not observed.
type transitionPhases struct {
	start       time.Time
	lockStart   time.Time
	lockEnd     time.Time
	callbackEnd time.Time
	end         time.Time
}

func (m *Metrics) observe(transitions []Transition, err error, phases transitionPhases) {
	if m == nil {
		return
	}
	label := "none"
	if len(transitions) > 0 {
		names := make([]string, len(transitions))
		for i, t := range transitions {
			names[i] = string(t)
		}
		label = strings.Join(names, "+")
	}
	outcome := "committed"
	if err != nil {
		outcome = "rolled_back"
	}
	m.duration.WithLabelValues(label, outcome).Observe(phases.end.Sub(phases.start).Seconds())
	observePhase := func(phase string, from, to time.Time) {
		if from.IsZero() || to.IsZero() {
			return
		}
		m.phase.WithLabelValues(label, outcome, phase).Observe(to.Sub(from).Seconds())
	}
	observePhase("pre_lock", phases.start, phases.lockStart)
	observePhase("lock_wait", phases.lockStart, phases.lockEnd)
	observePhase("callback", phases.lockEnd, phases.callbackEnd)
	observePhase("commit", phases.callbackEnd, phases.end)
}
