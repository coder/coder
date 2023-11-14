package healthcheck

import (
	"context"
	"time"

	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

const (
	DatabaseDefaultThreshold = 15 * time.Millisecond
)

// @typescript-generate DatabaseReport
type DatabaseReport struct {
	Healthy     bool    `json:"healthy"`
	Reachable   bool    `json:"reachable"`
	Latency     string  `json:"latency"`
	LatencyMS   int64   `json:"latency_ms"`
	ThresholdMS int64   `json:"threshold_ms"`
	Error       *string `json:"error"`
}

type DatabaseReportOptions struct {
	DB        database.Store
	Threshold time.Duration
}

func (r *DatabaseReport) Run(ctx context.Context, opts *DatabaseReportOptions) {
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
			r.Error = convertError(xerrors.Errorf("ping: %w", err))
			return
		}
		pings = append(pings, pong)
	}
	slices.Sort(pings)

	// Take the median ping.
	latency := pings[pingCount/2]
	r.Latency = latency.String()
	r.LatencyMS = latency.Milliseconds()
	if r.LatencyMS < r.ThresholdMS {
		r.Healthy = true
	}
	r.Reachable = true
}
