package healthcheck

import (
	"context"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk/healthsdk"
)

// PubsubReport reports on the health of the pubsub backend.
type PubsubReport healthsdk.PubsubReport

// PubsubReportOptions configures the pubsub health check.
type PubsubReportOptions struct {
	Pubsub pubsub.ReportingPubsub

	Dismissed bool
}

// Run populates the report from the pubsub backend's own health report.
func (r *PubsubReport) Run(ctx context.Context, opts *PubsubReportOptions) {
	*r = PubsubReport(opts.Pubsub.HealthReport(ctx))
	r.Dismissed = opts.Dismissed
}
