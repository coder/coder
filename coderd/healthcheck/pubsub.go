package healthcheck

import (
	"context"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/codersdk/healthsdk"
)

// PubsubReport checks the primary pubsub backend's connection state.
type PubsubReport healthsdk.PubsubReport

// PubsubReportOptions supplies the primary pubsub backend.
type PubsubReportOptions struct {
	Pubsub    pubsub.Pubsub
	Dismissed bool
}

// Run collects the primary backend's in-process health report.
func (r *PubsubReport) Run(_ context.Context, opts *PubsubReportOptions) {
	*r = PubsubReport{BaseReport: healthsdk.BaseReport{
		Severity:  health.SeverityOK,
		Warnings:  []health.Message{},
		Dismissed: opts.Dismissed,
	}}
	if opts.Pubsub == nil {
		r.Severity = health.SeverityError
		r.Error = health.Errorf(health.CodeUnknown, "pubsub is not configured")
		return
	}
	reporter, ok := opts.Pubsub.(pubsub.HealthReporter)
	if !ok {
		r.Severity = health.SeverityWarning
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeUnknown, "pubsub backend does not support health reporting"))
		return
	}

	report := reporter.ReportHealth()
	r.Backend = report.Backend
	r.Connected = &report.Connected
	if !report.LastConnectionStateChange.IsZero() {
		r.LastConnectionStateChange = &report.LastConnectionStateChange
	}
	if !report.Connected {
		r.Severity = health.SeverityError
		r.Error = health.Errorf(health.CodeUnknown, "pubsub backend %q is disconnected", report.Backend)
	}
}
