package healthcheck

import (
	"context"

	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/codersdk/healthsdk"
)

type UsagePublishingReport healthsdk.UsagePublishingReport

type UsagePublishingReportOptions struct {
	// Entitlements is the cached entitlements set, which carries the usage
	// publishing status computed during entitlements refresh. It may be nil,
	// in which case the report is healthy with publishing disabled.
	Entitlements *entitlements.Set

	Dismissed bool
}

// Run computes the usage publishing report from the cached entitlements
// snapshot. It performs no database or network access. Deployments without a
// license that enables usage publishing (e.g. AGPL or air-gapped deployments)
// always report as healthy with publishing disabled.
func (r *UsagePublishingReport) Run(_ context.Context, opts *UsagePublishingReportOptions) {
	r.Severity = health.SeverityOK
	r.Warnings = []health.Message{}
	r.Dismissed = opts.Dismissed
	r.Healthy = true

	if opts.Entitlements == nil {
		return
	}

	status := opts.Entitlements.UsagePublishing()
	r.PublishingEnabled = status.PublishingEnabled
	r.LastPublishedAt = status.LastPublishedAt
	r.FailingSince = status.FailingSince
	r.StatusUnavailable = status.StatusUnavailable
	if status.StatusUnavailable {
		// The status is unknown, not healthy; without this, a failed
		// status query would erase an active failure warning from the
		// health report.
		r.Severity = health.SeverityWarning
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeUnknown,
			"unable to determine usage publishing status; check the coderd logs"))
		return
	}
	if status.FailingSince != nil {
		r.Severity = health.SeverityWarning
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeUsagePublishingFailing,
			"usage events have failed to publish to Coder's servers since %s",
			status.FailingSince.UTC().Format("2006-01-02 15:04:05 MST")))
	}
}
