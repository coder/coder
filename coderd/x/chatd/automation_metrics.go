package chatd

import (
	"github.com/prometheus/client_golang/prometheus"
)

// AutomationWebhookRejectedReason is why the webhook endpoint rejected a
// request before it became a run.
type AutomationWebhookRejectedReason string

const (
	// AutomationWebhookRejectedInvalidSecret is a request for an existing
	// webhook automation without its secret.
	AutomationWebhookRejectedInvalidSecret AutomationWebhookRejectedReason = "invalid_secret"
	// AutomationWebhookRejectedUnknownAutomation is a request for an
	// automation that does not exist or is not a webhook.
	AutomationWebhookRejectedUnknownAutomation AutomationWebhookRejectedReason = "unknown_automation"
	// AutomationWebhookRejectedRateLimited is a request the rate limiter
	// refused.
	AutomationWebhookRejectedRateLimited AutomationWebhookRejectedReason = "rate_limited"
)

// automationMetrics are the automation metrics. They carry no
// per-automation or per-user labels, so their cardinality stays fixed.
type automationMetrics struct {
	runs            *prometheus.CounterVec
	webhookRejected *prometheus.CounterVec
	scanDuration    prometheus.Histogram
}

// newAutomationMetrics creates the automation metrics and registers them
// with registerer when it is set. Unregistered metrics still count, so
// callers need no nil checks.
func newAutomationMetrics(registerer prometheus.Registerer) *automationMetrics {
	m := &automationMetrics{
		runs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agents",
			Name:      "automation_runs_total",
			Help:      "Chat automation runs by trigger (schedule, webhook, run_now), outcome (accepted, skipped, refused), and reason (none for accepted runs; missed for schedule occurrences older than the grace window).",
		}, []string{"trigger", "outcome", "reason"}),
		webhookRejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agents",
			Name:      "automation_webhook_rejected_total",
			Help:      "Chat automation webhook requests rejected before they became a run, by reason (invalid_secret, unknown_automation, rate_limited).",
		}, []string{"reason"}),
		scanDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "agents",
			Name:      "automation_scan_duration_seconds",
			Help:      "Duration of one scan for due chat automation schedules on this replica, including publishing the due occurrences.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 2.5, 5, 10, 30, 60},
		}),
	}
	if registerer != nil {
		registerer.MustRegister(m.runs, m.webhookRejected, m.scanDuration)
	}
	return m
}

func (m *automationMetrics) recordRun(run automationRun) {
	// A Server built without New, as some tests do, has no metrics.
	if m == nil {
		return
	}
	reason := string(run.reason)
	if reason == "" {
		reason = "none"
	}
	m.runs.WithLabelValues(string(run.trigger), string(run.outcome), reason).Inc()
}

// RecordAutomationWebhookRejected counts a webhook request that was
// rejected before it became a run.
func (p *Server) RecordAutomationWebhookRejected(reason AutomationWebhookRejectedReason) {
	if p.automationMetrics == nil {
		return
	}
	p.automationMetrics.webhookRejected.WithLabelValues(string(reason)).Inc()
}
