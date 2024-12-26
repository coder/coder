package agent

import (
	"context"
	"maps"
	"sync"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/types/netlogtype"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
)

const maxConns = 2048

type networkStatsSource interface {
	SetConnStatsCallback(maxPeriod time.Duration, maxConns int, dump func(start, end time.Time, virtual, physical map[netlogtype.Connection]netlogtype.Counts))
}

type statsCollector interface {
	Collect(ctx context.Context, networkStats map[netlogtype.Connection]netlogtype.Counts) *proto.Stats
}

type statsDest interface {
	UpdateStats(ctx context.Context, req *proto.UpdateStatsRequest) (*proto.UpdateStatsResponse, error)
}

// statsReporter is a subcomponent of the agent that handles registering the stats callback on the
// networkStatsSource (tailnet.Conn in prod), handling the callback, calling back to the
// statsCollector (agent in prod) to collect additional stats, then sending the update to the
// statsDest (agent API in prod)
type statsReporter struct {
	*sync.Cond
	networkStats map[netlogtype.Connection]netlogtype.Counts
	unreported   bool
	lastInterval time.Duration

	source    networkStatsSource
	collector statsCollector
	logger    slog.Logger
}

func newStatsReporter(logger slog.Logger, source networkStatsSource, collector statsCollector) *statsReporter {
	return &statsReporter{
		Cond:      sync.NewCond(&sync.Mutex{}),
		logger:    logger,
		source:    source,
		collector: collector,
	}
}

func (s *statsReporter) callback(_, _ time.Time, virtual, _ map[netlogtype.Connection]netlogtype.Counts) {
	s.L.Lock()
	defer s.L.Unlock()
	s.logger.Debug(context.Background(), "got stats callback")
	// Accumulate stats until they've been reported.
	if s.unreported && len(s.networkStats) > 0 {
		for k, v := range virtual {
			s.networkStats[k] = s.networkStats[k].Add(v)
		}
	} else {
		s.networkStats = maps.Clone(virtual)
		s.unreported = true
	}
	s.Broadcast()
}

// reportLoop programs the source (tailnet.Conn) to send it stats via the
// callback, then reports them to the dest.
//
// It's intended to be called within the larger retry loop that establishes a
// connection to the agent API, then passes that connection to go routines like
// this that use it.  There is no retry and we fail on the first error since
// this will be inside a larger retry loop.
func (s *statsReporter) reportLoop(ctx context.Context, dest statsDest) error {
	// send an initial, blank report to get the interval
	resp, err := dest.UpdateStats(ctx, &proto.UpdateStatsRequest{})
	if err != nil {
		return xerrors.Errorf("initial update: %w", err)
	}
	s.lastInterval = resp.ReportInterval.AsDuration()
	s.source.SetConnStatsCallback(s.lastInterval, maxConns, s.callback)

	// use a separate goroutine to monitor the context so that we notice immediately, rather than
	// waiting for the next callback (which might never come if we are closing!)
	ctxDone := false
	go func() {
		<-ctx.Done()
		s.L.Lock()
		defer s.L.Unlock()
		ctxDone = true
		s.Broadcast()
	}()
	defer s.logger.Debug(ctx, "reportLoop exiting")

	s.L.Lock()
	defer s.L.Unlock()
	for {
		for !s.unreported && !ctxDone {
			s.Wait()
		}
		if ctxDone {
			return nil
		}
		s.unreported = false
		if err = s.reportLocked(ctx, dest, s.networkStats); err != nil {
			return xerrors.Errorf("report stats: %w", err)
		}
	}
}

func (s *statsReporter) reportLocked(
	ctx context.Context, dest statsDest, networkStats map[netlogtype.Connection]netlogtype.Counts,
) error {
	// here we want to do our collecting/reporting while it is unlocked, but then relock
	// when we return to reportLoop.
	s.L.Unlock()
	defer s.L.Lock()
	stats := s.collector.Collect(ctx, networkStats)
	resp, err := dest.UpdateStats(ctx, &proto.UpdateStatsRequest{Stats: stats})
	if err != nil {
		return err
	}
	interval := resp.GetReportInterval().AsDuration()
	if interval != s.lastInterval {
		s.logger.Info(ctx, "new stats report interval", slog.F("interval", interval))
		s.lastInterval = interval
		s.source.SetConnStatsCallback(s.lastInterval, maxConns, s.callback)
	}
	return nil
}
