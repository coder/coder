package healthcheck

import (
	"context"
	"slices"
	"time"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/codersdk/healthsdk"
)

const (
	DatabaseDefaultThreshold = 15 * time.Millisecond
)

type DatabaseReport healthsdk.DatabaseReport

type DatabaseReportOptions struct {
	DB        database.Store
	Threshold time.Duration

	// Pubsub is the pubsub backend currently in use. If it implements
	// pubsub.ConnectionStatusReporter (currently only the embedded NATS pubsub
	// does), the report's Pubsub.Connected field reflects its live
	// connection state.
	Pubsub pubsub.Pubsub
	// PubsubNATSEnabled indicates whether the embedded NATS pubsub backend is
	// enabled for this deployment (i.e. the no_nats_pubsub experiment is not
	// set). It is independent of Pubsub, since a deployment can be configured
	// to use NATS and still be running PostgreSQL pubsub because it fell back
	// (e.g. missing cluster host). NATS is optional, so neither field affects
	// this report's severity.
	PubsubNATSEnabled bool

	Dismissed bool
}

func (r *DatabaseReport) Run(ctx context.Context, opts *DatabaseReportOptions) {
	r.Warnings = []health.Message{}
	r.Severity = health.SeverityOK
	r.Dismissed = opts.Dismissed

	r.Pubsub.Enabled = opts.PubsubNATSEnabled
	if checker, ok := opts.Pubsub.(pubsub.ConnectionStatusReporter); ok {
		r.Pubsub.Connected = checker.Connected()
	}

	r.ThresholdMS = opts.Threshold.Milliseconds()
	if r.ThresholdMS == 0 {
		r.ThresholdMS = DatabaseDefaultThreshold.Milliseconds()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pingCount := 5
	pings := make([]time.Duration, 0, pingCount)
	// Ping 5 times and average the latency.
	for i := 0; i < pingCount; i++ {
		pong, err := opts.DB.Ping(ctx)
		if err != nil {
			r.Error = health.Errorf(health.CodeDatabasePingFailed, "ping database: %s", err)
			r.Severity = health.SeverityError

			return
		}
		pings = append(pings, pong)
	}
	slices.Sort(pings)

	// Take the median ping.
	latency := pings[pingCount/2]
	r.Latency = latency.String()
	r.LatencyMS = latency.Milliseconds()
	if r.LatencyMS >= r.ThresholdMS {
		r.Severity = health.SeverityWarning
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeDatabasePingSlow, "median database ping above threshold"))
	}
	r.Healthy = true
	r.Reachable = true
}
