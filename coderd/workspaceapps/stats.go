package workspaceapps

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

const (
	DefaultStatsCollectorReportInterval = 30 * time.Second
	DefaultStatsCollectorRollupWindow   = 1 * time.Minute
	DefaultStatsDBReporterBatchSize     = 1024
)

// StatsReport is a report of a workspace app session.
type StatsReport struct {
	UserID           uuid.UUID    `json:"user_id"`
	WorkspaceID      uuid.UUID    `json:"workspace_id"`
	AgentID          uuid.UUID    `json:"agent_id"`
	AccessMethod     AccessMethod `json:"access_method"`
	SlugOrPort       string       `json:"slug_or_port"`
	SessionID        uuid.UUID    `json:"session_id"`
	SessionStartedAt time.Time    `json:"session_started_at"`
	SessionEndedAt   time.Time    `json:"session_ended_at"` // Updated periodically while app is in use active and when the last connection is closed.
	Requests         int          `json:"requests"`

	rolledUp bool // Indicates if this report has been rolled up.
}

func newStatsReportFromSignedToken(token SignedToken) StatsReport {
	return StatsReport{
		UserID:           token.UserID,
		WorkspaceID:      token.WorkspaceID,
		AgentID:          token.AgentID,
		AccessMethod:     token.AccessMethod,
		SlugOrPort:       token.AppSlugOrPort,
		SessionID:        uuid.New(),
		SessionStartedAt: dbtime.Now(),
		Requests:         1,
	}
}

// StatsReporter reports workspace app StatsReports.
type StatsReporter interface {
	Report(context.Context, []StatsReport) error
}

var _ StatsReporter = (*StatsDBReporter)(nil)

// StatsDBReporter writes workspace app StatsReports to the database.
type StatsDBReporter struct {
	db        database.Store
	batchSize int
}

// NewStatsDBReporter returns a new StatsDBReporter.
func NewStatsDBReporter(db database.Store, batchSize int) *StatsDBReporter {
	return &StatsDBReporter{
		db:        db,
		batchSize: batchSize,
	}
}

// Report writes the given StatsReports to the database.
func (r *StatsDBReporter) Report(ctx context.Context, stats []StatsReport) error {
	err := r.db.InTx(func(tx database.Store) error {
		maxBatchSize := r.batchSize
		if len(stats) < maxBatchSize {
			maxBatchSize = len(stats)
		}
		batch := database.InsertWorkspaceAppStatsParams{
			UserID:           make([]uuid.UUID, 0, maxBatchSize),
			WorkspaceID:      make([]uuid.UUID, 0, maxBatchSize),
			AgentID:          make([]uuid.UUID, 0, maxBatchSize),
			AccessMethod:     make([]string, 0, maxBatchSize),
			SlugOrPort:       make([]string, 0, maxBatchSize),
			SessionID:        make([]uuid.UUID, 0, maxBatchSize),
			SessionStartedAt: make([]time.Time, 0, maxBatchSize),
			SessionEndedAt:   make([]time.Time, 0, maxBatchSize),
			Requests:         make([]int32, 0, maxBatchSize),
		}
		for _, stat := range stats {
			batch.UserID = append(batch.UserID, stat.UserID)
			batch.WorkspaceID = append(batch.WorkspaceID, stat.WorkspaceID)
			batch.AgentID = append(batch.AgentID, stat.AgentID)
			batch.AccessMethod = append(batch.AccessMethod, string(stat.AccessMethod))
			batch.SlugOrPort = append(batch.SlugOrPort, stat.SlugOrPort)
			batch.SessionID = append(batch.SessionID, stat.SessionID)
			batch.SessionStartedAt = append(batch.SessionStartedAt, stat.SessionStartedAt)
			batch.SessionEndedAt = append(batch.SessionEndedAt, stat.SessionEndedAt)
			batch.Requests = append(batch.Requests, int32(stat.Requests))

			if len(batch.UserID) >= r.batchSize {
				err := tx.InsertWorkspaceAppStats(ctx, batch)
				if err != nil {
					return err
				}

				// Reset batch.
				batch.UserID = batch.UserID[:0]
				batch.WorkspaceID = batch.WorkspaceID[:0]
				batch.AgentID = batch.AgentID[:0]
				batch.AccessMethod = batch.AccessMethod[:0]
				batch.SlugOrPort = batch.SlugOrPort[:0]
				batch.SessionID = batch.SessionID[:0]
				batch.SessionStartedAt = batch.SessionStartedAt[:0]
				batch.SessionEndedAt = batch.SessionEndedAt[:0]
				batch.Requests = batch.Requests[:0]
			}
		}
		if len(batch.UserID) > 0 {
			err := tx.InsertWorkspaceAppStats(ctx, batch)
			if err != nil {
				return err
			}
		}

		return nil
	}, nil)
	if err != nil {
		return xerrors.Errorf("insert workspace app stats failed: %w", err)
	}

	return nil
}

// This should match the database unique constraint.
type statsGroupKey struct {
	StartTimeTrunc time.Time
	UserID         uuid.UUID
	WorkspaceID    uuid.UUID
	AgentID        uuid.UUID
	AccessMethod   AccessMethod
	SlugOrPort     string
}

func (s StatsReport) groupKey(windowSize time.Duration) statsGroupKey {
	return statsGroupKey{
		StartTimeTrunc: s.SessionStartedAt.Truncate(windowSize),
		UserID:         s.UserID,
		WorkspaceID:    s.WorkspaceID,
		AgentID:        s.AgentID,
		AccessMethod:   s.AccessMethod,
		SlugOrPort:     s.SlugOrPort,
	}
}

// StatsCollector collects workspace app StatsReports and reports them
// in batches, stats compaction is performed for short-lived sessions.
type StatsCollector struct {
	opts StatsCollectorOptions

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu               sync.Mutex                       // Protects following.
	statsBySessionID map[uuid.UUID]*StatsReport       // Track unique sessions.
	groupedStats     map[statsGroupKey][]*StatsReport // Rolled up stats for sessions in close proximity.
	backlog          []StatsReport                    // Stats that have not been reported yet (due to error).
}

type StatsCollectorOptions struct {
	Logger   *slog.Logger
	Reporter StatsReporter
	// ReportInterval is the interval at which stats are reported, both partial
	// and fully formed stats.
	ReportInterval time.Duration
	// RollupWindow is the window size for rolling up stats, session shorter
	// than this will be rolled up and longer than this will be tracked
	// individually.
	RollupWindow time.Duration

	// Options for tests.
	Flush <-chan chan<- struct{}
	Now   func() time.Time
}

func NewStatsCollector(opts StatsCollectorOptions) *StatsCollector {
	if opts.Logger == nil {
		opts.Logger = &slog.Logger{}
	}
	if opts.ReportInterval == 0 {
		opts.ReportInterval = DefaultStatsCollectorReportInterval
	}
	if opts.RollupWindow == 0 {
		opts.RollupWindow = DefaultStatsCollectorRollupWindow
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	ctx, cancel := context.WithCancel(context.Background())
	sc := &StatsCollector{
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		opts:   opts,

		statsBySessionID: make(map[uuid.UUID]*StatsReport),
		groupedStats:     make(map[statsGroupKey][]*StatsReport),
	}

	go sc.start()
	return sc
}

// Collect the given StatsReport for later reporting (non-blocking).
func (sc *StatsCollector) Collect(report StatsReport) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	r := &report
	if _, ok := sc.statsBySessionID[report.SessionID]; !ok {
		groupKey := r.groupKey(sc.opts.RollupWindow)
		sc.groupedStats[groupKey] = append(sc.groupedStats[groupKey], r)
	}

	if r.SessionEndedAt.IsZero() {
		sc.statsBySessionID[report.SessionID] = r
	} else {
		if stat, ok := sc.statsBySessionID[report.SessionID]; ok {
			// Update in-place.
			*stat = *r
		}
		delete(sc.statsBySessionID, report.SessionID)
	}
}

// rollup performs stats rollup for sessions that fall within the
// configured rollup window. For sessions longer than the window,
// we report them individually.
func (sc *StatsCollector) rollup(now time.Time) []StatsReport {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	var report []StatsReport

	for g, group := range sc.groupedStats {
		if len(group) == 0 {
			// Safety check, this should not happen.
			sc.opts.Logger.Error(sc.ctx, "empty stats group", "group", g)
			delete(sc.groupedStats, g)
			continue
		}

		var rolledUp *StatsReport
		if group[0].rolledUp {
			rolledUp = group[0]
			group = group[1:]
		} else {
			rolledUp = &StatsReport{
				UserID:           g.UserID,
				WorkspaceID:      g.WorkspaceID,
				AgentID:          g.AgentID,
				AccessMethod:     g.AccessMethod,
				SlugOrPort:       g.SlugOrPort,
				SessionStartedAt: g.StartTimeTrunc,
				SessionEndedAt:   g.StartTimeTrunc.Add(sc.opts.RollupWindow),
				Requests:         0,
				rolledUp:         true,
			}
		}
		rollupChanged := false
		newGroup := []*StatsReport{rolledUp} // Must be first in slice for future iterations (see group[0] above).
		for _, stat := range group {
			if !stat.SessionEndedAt.IsZero() && stat.SessionEndedAt.Sub(stat.SessionStartedAt) <= sc.opts.RollupWindow {
				// This is a short-lived session, roll it up.
				if rolledUp.SessionID == uuid.Nil {
					rolledUp.SessionID = stat.SessionID // Borrow the first session ID, useful in tests.
				}
				rolledUp.Requests += stat.Requests
				rollupChanged = true
				continue
			}
			if stat.SessionEndedAt.IsZero() && now.Sub(stat.SessionStartedAt) <= sc.opts.RollupWindow {
				// This is an incomplete session, wait and see if it'll be rolled up or not.
				newGroup = append(newGroup, stat)
				continue
			}

			// This is a long-lived session, report it individually.
			// Make a copy of stat for reporting.
			r := *stat
			if r.SessionEndedAt.IsZero() {
				// Report an end time for incomplete sessions, it will
				// be updated later. This ensures that data in the DB
				// will have an end time even if the service is stopped.
				r.SessionEndedAt = now.UTC() // Use UTC like dbtime.Now().
			}
			report = append(report, r) // Report it (ended or incomplete).
			if stat.SessionEndedAt.IsZero() {
				newGroup = append(newGroup, stat) // Keep it for future updates.
			}
		}
		if rollupChanged {
			report = append(report, *rolledUp)
		}

		// Future rollups should only consider the compacted group.
		sc.groupedStats[g] = newGroup

		// Keep the group around until the next rollup window has passed
		// in case data was collected late.
		if len(newGroup) == 1 && rolledUp.SessionEndedAt.Add(sc.opts.RollupWindow).Before(now) {
			delete(sc.groupedStats, g)
		}
	}

	return report
}

func (sc *StatsCollector) flush(ctx context.Context) (err error) {
	sc.opts.Logger.Debug(ctx, "flushing workspace app stats")
	defer func() {
		if err != nil {
			sc.opts.Logger.Error(ctx, "failed to flush workspace app stats", "error", err)
		} else {
			sc.opts.Logger.Debug(ctx, "flushed workspace app stats")
		}
	}()

	// We keep the backlog as a simple slice so that we don't need to
	// attempt to merge it with the stats we're about to report. This
	// is because the rollup is a one-way operation and the backlog may
	// contain stats that are still in the statsBySessionID map and will
	// be reported again in the future. It is possible to merge the
	// backlog and the stats we're about to report, but it's not worth
	// the complexity.
	if len(sc.backlog) > 0 {
		err = sc.opts.Reporter.Report(ctx, sc.backlog)
		if err != nil {
			return xerrors.Errorf("report workspace app stats from backlog failed: %w", err)
		}
		sc.backlog = nil
	}

	now := sc.opts.Now()
	stats := sc.rollup(now)
	if len(stats) == 0 {
		return nil
	}

	err = sc.opts.Reporter.Report(ctx, stats)
	if err != nil {
		sc.backlog = stats
		return xerrors.Errorf("report workspace app stats failed: %w", err)
	}

	return nil
}

func (sc *StatsCollector) Close() error {
	sc.cancel()
	<-sc.done
	return nil
}

func (sc *StatsCollector) start() {
	defer func() {
		close(sc.done)
		sc.opts.Logger.Debug(sc.ctx, "workspace app stats collector stopped")
	}()
	sc.opts.Logger.Debug(sc.ctx, "workspace app stats collector started")

	t := time.NewTimer(sc.opts.ReportInterval)
	defer t.Stop()

	var reportFlushDone chan<- struct{}
	done := false
	for !done {
		select {
		case <-sc.ctx.Done():
			t.Stop()
			done = true
		case <-t.C:
		case reportFlushDone = <-sc.opts.Flush:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		//nolint:gocritic // Inserting app stats is a system function.
		_ = sc.flush(dbauthz.AsSystemRestricted(ctx))
		cancel()

		if !done {
			t.Reset(sc.opts.ReportInterval)
		}

		// For tests.
		if reportFlushDone != nil {
			reportFlushDone <- struct{}{}
			reportFlushDone = nil
		}
	}
}
