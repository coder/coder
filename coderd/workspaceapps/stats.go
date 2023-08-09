package workspaceapps

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

const (
	DefaultStatsCollectorReportInterval = 30 * time.Second
)

// StatsReport is a report of a workspace app session.
type StatsReport struct {
	UserID           uuid.UUID         `json:"user_id"`
	WorkspaceID      uuid.UUID         `json:"workspace_id"`
	AgentID          uuid.UUID         `json:"agent_id"`
	AccessMethod     AccessMethod      `json:"access_method"`
	SlugOrPort       string            `json:"slug"`
	SessionID        uuid.UUID         `json:"session_id"`
	SessionStartTime time.Time         `json:"session_start_time"`
	SessionEndTime   codersdk.NullTime `json:"session_end_time"`
}

func newStatsReportFromSignedToken(token SignedToken) StatsReport {
	return StatsReport{
		UserID:           token.UserID,
		WorkspaceID:      token.WorkspaceID,
		AgentID:          token.AgentID,
		AccessMethod:     token.AccessMethod,
		SlugOrPort:       token.AppSlugOrPort,
		SessionID:        uuid.New(),
		SessionStartTime: database.Now(),
	}
}

// StatsReporter reports workspace app StatsReports.
type StatsReporter interface {
	Report(context.Context, []StatsReport) error
}

// StatsDBReporter writes workspace app StatsReports to the database.
type StatsDBReporter struct {
	Database database.Store
}

// NewStatsDBReporter returns a new StatsDBReporter.
func NewStatsDBReporter(db database.Store) *StatsDBReporter {
	return &StatsDBReporter{
		Database: db,
	}
}

// Report writes the given StatsReports to the database.
func (r *StatsDBReporter) Report(ctx context.Context, stats []StatsReport) error {
	err := r.Database.InTx(func(tx database.Store) error {
		for _, stat := range stats {
			err := tx.InsertWorkspaceAppStats(ctx, database.InsertWorkspaceAppStatsParams{
				ID:               uuid.New(),
				UserID:           stat.UserID,
				WorkspaceID:      stat.WorkspaceID,
				AgentID:          stat.AgentID,
				AccessMethod:     string(stat.AccessMethod),
				SlugOrPort:       stat.SlugOrPort,
				SessionID:        stat.SessionID,
				SessionStartedAt: stat.SessionStartTime,
				SessionEndedAt:   stat.SessionEndTime.NullTime,
			})
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

// StatsCollector collects workspace app StatsReports and reports them
// in batches, stats compaction is performed for short-lived sessions.
type StatsCollector struct {
	logger   slog.Logger
	reporter StatsReporter

	ctx            context.Context
	cancel         context.CancelFunc
	done           chan struct{}
	reportInterval time.Duration

	mu    sync.Mutex // Protects following.
	stats []StatsReport
}

// Collect the given StatsReport for later reporting (non-blocking).
func (s *StatsCollector) Collect(report StatsReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats = append(s.stats, report)
}

func (s *StatsCollector) flush(ctx context.Context) error {
	s.mu.Lock()
	stats := s.stats
	s.stats = nil
	s.mu.Unlock()

	if len(stats) == 0 {
		return nil
	}

	// Compaction of stats, reduce payload by up to 50%.
	compacted := make([]StatsReport, 0, len(stats))
	m := make(map[StatsReport]int)
	for _, stat := range stats {
		if !stat.SessionEndTime.IsZero() {
			// Zero the time for map key equality.
			cmp := stat
			cmp.SessionEndTime = codersdk.NullTime{}
			if j, ok := m[cmp]; ok {
				compacted[j].SessionEndTime = stat.SessionEndTime
				continue
			}
		}
		m[stat] = len(compacted)
		compacted = append(compacted, stat)
	}

	err := s.reporter.Report(ctx, stats)
	return err
}

func (s *StatsCollector) Close() error {
	s.cancel()
	<-s.done
	return nil
}

func NewStatsCollector(logger slog.Logger, reporter StatsReporter, reportInterval time.Duration) *StatsCollector {
	ctx, cancel := context.WithCancel(context.Background())
	c := &StatsCollector{
		logger:   logger,
		reporter: reporter,

		ctx:            ctx,
		cancel:         cancel,
		done:           make(chan struct{}),
		reportInterval: reportInterval,
	}

	go c.start()
	return c
}

func (s *StatsCollector) start() {
	defer func() {
		close(s.done)
		s.logger.Info(s.ctx, "workspace app stats collector stopped")
	}()
	s.logger.Info(s.ctx, "workspace app stats collector started")

	ticker := time.NewTicker(s.reportInterval)
	defer ticker.Stop()

	done := false
	for !done {
		select {
		case <-s.ctx.Done():
			ticker.Stop()
			done = true
		case <-ticker.C:
		}

		s.logger.Debug(s.ctx, "flushing workspace app stats")

		// Ensure we don't hold up this request for too long.
		ctx, cancel := context.WithTimeout(context.Background(), s.reportInterval+5*time.Second)
		err := s.flush(ctx)
		cancel()
		if err != nil {
			s.logger.Error(ctx, "failed to flush workspace app stats", slog.Error(err))
			continue
		}
	}
}
