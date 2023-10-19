package insights

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
)

var templatesActiveUsersDesc = prometheus.NewDesc("coderd_insights_templates_active_users", "The number of active users of the template.", []string{"template_name"}, nil)

type MetricsCollector struct {
	database     database.Store
	logger       slog.Logger
	timeWindow   time.Duration
	tickInterval time.Duration

	data atomic.Pointer[insightsData]
}

type insightsData struct {
	templates []database.GetTemplateInsightsByTemplateRow

	templateNames map[uuid.UUID]string
}

var _ prometheus.Collector = new(MetricsCollector)

func NewMetricsCollector(db database.Store, logger slog.Logger, timeWindow time.Duration, tickInterval time.Duration) (*MetricsCollector, error) {
	if timeWindow == 0 {
		timeWindow = 5 * time.Minute
	}
	if timeWindow < 5*time.Minute {
		return nil, xerrors.Errorf("time window must be at least 5 mins")
	}
	if tickInterval == 0 {
		tickInterval = timeWindow
	}

	return &MetricsCollector{
		database:     db,
		logger:       logger.Named("insights_metrics_collector"),
		timeWindow:   timeWindow,
		tickInterval: tickInterval,
	}, nil
}

func (mc *MetricsCollector) Run(ctx context.Context) (func(), error) {
	ctx, closeFunc := context.WithCancel(ctx)
	done := make(chan struct{})

	// Use time.Nanosecond to force an initial tick. It will be reset to the
	// correct duration after executing once.
	ticker := time.NewTicker(time.Nanosecond)
	doTick := func() {
		defer ticker.Reset(mc.tickInterval)

		now := time.Now()
		startTime := now.Add(-mc.timeWindow)
		endTime := now

		// Phase 1: Fetch insights from database
		// FIXME errorGroup will be used to fetch insights for apps and parameters
		eg, egCtx := errgroup.WithContext(ctx)
		eg.SetLimit(1)

		var templateInsights []database.GetTemplateInsightsByTemplateRow

		eg.Go(func() error {
			var err error
			templateInsights, err = mc.database.GetTemplateInsightsByTemplate(egCtx, database.GetTemplateInsightsByTemplateParams{
				StartTime: startTime,
				EndTime:   endTime,
			})
			if err != nil {
				mc.logger.Error(ctx, "unable to fetch template insights from database", slog.Error(err))
			}
			return err
		})
		err := eg.Wait()
		if err != nil {
			return
		}

		// Phase 2: Collect template IDs, and fetch relevant details
		templateIDs := uniqueTemplateIDs(templateInsights)

		templateNames := make(map[uuid.UUID]string, len(templateIDs))
		if len(templateIDs) > 0 {
			templates, err := mc.database.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
				IDs: templateIDs,
			})
			if err != nil {
				mc.logger.Error(ctx, "unable to fetch template details from database", slog.Error(err))
				return
			}
			templateNames = onlyTemplateNames(templates)
		}

		// Refresh the collector state
		mc.data.Store(&insightsData{
			templates:     templateInsights,
			templateNames: templateNames,
		})
	}

	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ticker.Stop()
				doTick()
			}
		}
	}()
	return func() {
		closeFunc()
		<-done
	}, nil
}

func (*MetricsCollector) Describe(descCh chan<- *prometheus.Desc) {
	descCh <- templatesActiveUsersDesc
}

func (mc *MetricsCollector) Collect(metricsCh chan<- prometheus.Metric) {
	// Phase 3: Collect metrics

	data := mc.data.Load()
	if data == nil {
		return // insights data not loaded yet
	}

	for _, templateRow := range data.templates {
		metricsCh <- prometheus.MustNewConstMetric(templatesActiveUsersDesc, prometheus.GaugeValue, float64(templateRow.ActiveUsers), data.templateNames[templateRow.TemplateID])
	}
}

// Helper functions below.

func uniqueTemplateIDs(templateInsights []database.GetTemplateInsightsByTemplateRow) []uuid.UUID {
	tids := map[uuid.UUID]bool{}
	for _, t := range templateInsights {
		tids[t.TemplateID] = true
	}

	uniqueUUIDs := make([]uuid.UUID, len(tids))
	var i int
	for t := range tids {
		uniqueUUIDs[i] = t
		i++
	}
	return uniqueUUIDs
}

func onlyTemplateNames(templates []database.Template) map[uuid.UUID]string {
	m := map[uuid.UUID]string{}
	for _, t := range templates {
		m[t.ID] = t.Name
	}
	return m
}
