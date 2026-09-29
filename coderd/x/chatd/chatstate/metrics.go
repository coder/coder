package chatstate

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics records one observation per [ChatMachine.Update] so operators
// can see how many transactions each transition shape costs, how long
// they hold the chat row lock, and how long they wait to acquire it.
// A nil *Metrics is a no-op.
type Metrics struct {
	duration *prometheus.HistogramVec
	lockWait *prometheus.HistogramVec
}

// NewMetrics creates and registers the transition metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	buckets := prometheus.ExponentialBuckets(0.0005, 2, 15)
	m := &Metrics{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "transition_duration_seconds",
			Help:      "Wall time of one ChatMachine.Update transaction from lock attempt to commit or rollback, including the lock wait. The transition label lists the transitions the callback ran, joined by '+'.",
			Buckets:   buckets,
		}, []string{"transition", "outcome"}),
		lockWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "transition_lock_wait_seconds",
			Help:      "Time ChatMachine.Update spent acquiring the chat row lock, which is time another transition on the same chat held it.",
			Buckets:   buckets,
		}, []string{"transition", "outcome"}),
	}
	reg.MustRegister(m.duration, m.lockWait)
	return m
}

func (m *Metrics) observe(transitions []Transition, err error, duration, lockWait time.Duration) {
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
	m.duration.WithLabelValues(label, outcome).Observe(duration.Seconds())
	m.lockWait.WithLabelValues(label, outcome).Observe(lockWait.Seconds())
}
