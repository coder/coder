package monitoring

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
)

type TelemetryLevel string

const (
	TelemetryLevelAll  TelemetryLevel = "all"
	TelemetryLevelCore TelemetryLevel = "core"
	TelemetryLevelNone TelemetryLevel = "none"
)

// ParseTelemetryLevel returns a valid TelemetryLevel or error if input is not a valid.
func ParseTelemetryLevel(t string) (TelemetryLevel, error) {
	ok := []string{
		string(TelemetryLevelAll),
		string(TelemetryLevelCore),
		string(TelemetryLevelNone),
	}

	for _, a := range ok {
		if strings.EqualFold(a, t) {
			return TelemetryLevel(a), nil
		}
	}

	return "", xerrors.Errorf(`invalid telemetry level: %s, must be one of: %s`, t, strings.Join(ok, ","))
}

type Options struct {
	Database        database.Store
	Logger          slog.Logger
	RefreshInterval time.Duration
	TelemetryLevel  TelemetryLevel
}

type Monitor struct {
	// allRegistry registers metrics that will be sent when the telemetry level is
	// `all`.
	allRegistry *prometheus.Registry
	// db is the database from which to pull stats.
	db  database.Store
	ctx context.Context
	// coreRegistry registers metrics that will be sent when the telemetry level
	// is `core` or `all`.
	coreRegistry *prometheus.Registry
	// internalRegisry registers metrics that will never be sent.
	internalRegistry *prometheus.Registry
	// refreshMutex is used to prevent multiple refreshes at a time.
	refreshMutex *sync.Mutex
	// stats are internally registered metrics that update via Refresh.
	stats Stats
	// TelemetryLevel determines which metrics are sent to Coder.
	TelemetryLevel TelemetryLevel
}

type Stats struct {
	Users              *prometheus.GaugeVec
	Workspaces         *prometheus.GaugeVec
	WorkspaceResources *prometheus.GaugeVec
}

func New(ctx context.Context, options *Options) *Monitor {
	monitor := Monitor{
		allRegistry:      prometheus.NewRegistry(),
		db:               options.Database,
		ctx:              ctx,
		coreRegistry:     prometheus.NewRegistry(),
		internalRegistry: prometheus.NewRegistry(),
		refreshMutex:     &sync.Mutex{},
		stats: Stats{
			Users: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "coder",
				Name:      "users",
				Help:      "The users in a Coder deployment.",
			}, []string{
				"user_id",
				"user_name",
			}),
			Workspaces: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "coder",
				Name:      "workspaces",
				Help:      "The workspaces in a Coder deployment.",
			}, []string{
				"workspace_id",
				"workspace_name",
			}),
			WorkspaceResources: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "coder",
				Name:      "workspace_resources",
				Help:      "The workspace resources in a Coder deployment.",
			}, []string{
				"workspace_resource_id",
				"workspace_resource_name",
				"workspace_resource_type",
			}),
		},
		TelemetryLevel: options.TelemetryLevel,
	}

	monitor.MustRegister(
		TelemetryLevelAll,
		monitor.stats.Users,
		monitor.stats.Workspaces,
		monitor.stats.WorkspaceResources,
	)

	ticker := time.NewTicker(options.RefreshInterval)
	go func() {
		defer ticker.Stop()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := monitor.Refresh()
			if err != nil {
				options.Logger.Error(ctx, "failed to refresh stats", slog.Error(err))
			}
		}
	}()

	return &monitor
}

// MustRegister registers collectors at the specified level.
func (t Monitor) MustRegister(level TelemetryLevel, cs ...prometheus.Collector) {
	switch level {
	case TelemetryLevelAll:
		t.allRegistry.MustRegister(cs...)
	case TelemetryLevelCore:
		t.coreRegistry.MustRegister(cs...)
	case TelemetryLevelNone:
		t.internalRegistry.MustRegister(cs...)
	}
}

// Gather returns all gathered metrics.
func (t Monitor) Gather() ([]*dto.MetricFamily, error) {
	allMetrics, err := t.allRegistry.Gather()
	if err != nil {
		return nil, err
	}

	coreMetrics, err := t.coreRegistry.Gather()
	if err != nil {
		return nil, err
	}

	internalMetrics, err := t.internalRegistry.Gather()
	if err != nil {
		return nil, err
	}

	return append(append(allMetrics, coreMetrics...), internalMetrics...), nil
}

// Refresh populates internal stats with the latest data.
func (t Monitor) Refresh() error {
	t.refreshMutex.Lock()
	defer t.refreshMutex.Unlock()

	errGroup, ctx := errgroup.WithContext(t.ctx)

	errGroup.Go(func() error {
		dbUsers, err := t.db.GetUsers(ctx, database.GetUsersParams{})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return err
		}

		t.stats.Users.Reset()
		for _, dbu := range dbUsers {
			t.stats.Users.With(prometheus.Labels{
				"user_id":   dbu.ID.String(),
				"user_name": dbu.Username,
			}).Add(1)
		}

		return nil
	})

	errGroup.Go(func() error {
		dbWorkspaces, err := t.db.GetWorkspaces(ctx, false)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return err
		}

		t.stats.Workspaces.Reset()
		for _, dbw := range dbWorkspaces {
			t.stats.Workspaces.With(prometheus.Labels{
				"workspace_id":   dbw.ID.String(),
				"workspace_name": dbw.Name,
			}).Add(1)
		}

		return nil
	})

	errGroup.Go(func() error {
		dbWorkspaceResources, err := t.db.GetWorkspaceResources(ctx)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return err
		}

		t.stats.WorkspaceResources.Reset()
		for _, dbwr := range dbWorkspaceResources {
			t.stats.WorkspaceResources.With(prometheus.Labels{
				"workspace_resource_id":   dbwr.ID.String(),
				"workspace_resource_name": dbwr.Name,
				"workspace_resource_type": dbwr.Type,
			}).Add(1)
		}

		return nil
	})

	return errGroup.Wait()
}
