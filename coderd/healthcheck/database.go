package healthcheck

import (
	"context"
	"slices"
	"time"

	"github.com/coder/coder/v2/coderd/database"
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

	Dismissed bool
	// ServerVersionNum is the PostgreSQL server_version_num reported by the
	// connected server, if known (0 otherwise). A positive value below
	// 140000 (major version < 14) triggers an end-of-life warning.
	ServerVersionNum int
	// Builtin indicates whether Coder manages the PostgreSQL server.
	Builtin bool
}

func (r *DatabaseReport) Run(ctx context.Context, opts *DatabaseReportOptions) {
	r.Warnings = []health.Message{}
	r.Severity = health.SeverityOK
	r.Dismissed = opts.Dismissed

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
	// Surface a warning, without downgrading an existing error, when the
	// connected PostgreSQL server is running an end-of-life major version.
	if opts.ServerVersionNum > 0 && opts.ServerVersionNum < 140000 {
		if r.Severity == health.SeverityOK {
			r.Severity = health.SeverityWarning
		}
		message := "PostgreSQL version is end-of-life; upgrade your PostgreSQL server to a supported version (14+)."
		if opts.Builtin {
			message = "Built-in PostgreSQL version is end-of-life; migrate to an external PostgreSQL database using the migration guide linked in the EDB03 documentation."
		}
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeDatabasePostgresVersionEOL, "%s", message))
	}
	r.Healthy = true
	r.Reachable = true
}
