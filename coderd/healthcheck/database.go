package healthcheck

import (
	"context"
	"time"

	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
)

type DatabaseReport struct {
	Healthy   bool          `json:"healthy"`
	Reachable bool          `json:"reachable"`
	Latency   time.Duration `json:"latency"`
	Error     error         `json:"error"`
}

type DatabaseReportOptions struct {
	DB database.Store
}

func (r *DatabaseReport) Run(ctx context.Context, opts *DatabaseReportOptions) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pingCount := 5
	pings := make([]time.Duration, 0, pingCount)
	// Ping 5 times and average the latency.
	for i := 0; i < pingCount; i++ {
		pong, err := opts.DB.Ping(ctx)
		if err != nil {
			r.Error = xerrors.Errorf("ping: %w", err)
			return
		}
		pings = append(pings, pong)
	}
	slices.Sort(pings)

	// Take the median ping.
	r.Latency = pings[pingCount/2]
	// Somewhat arbitrary, but if the latency is over 15ms, we consider it
	// unhealthy.
	if r.Latency < 15*time.Millisecond {
		r.Healthy = true
	}
	r.Reachable = true
}
