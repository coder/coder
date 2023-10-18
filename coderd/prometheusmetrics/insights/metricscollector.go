package insights

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
)

var (
	activeUsersDesc              = prometheus.NewDesc("coderd_insights_active_users", "The number of active users of the template.", []string{"template_name"}, nil)
	applicationsUsageSecondsDesc = prometheus.NewDesc("coderd_insights_applications_usage_seconds", "The application usage per template.", []string{"application_name", "template_name"}, nil)
	parametersDesc               = prometheus.NewDesc("coderd_insights_parameters", "The parameter usage per template.", []string{"template_name", "name", "value"}, nil)
)

type MetricsCollector struct {
	database database.Store
	logger   slog.Logger
	duration time.Duration
}

var _ prometheus.Collector = new(MetricsCollector)

func NewMetricsCollector(db database.Store, logger slog.Logger, duration time.Duration) (*MetricsCollector, error) {
	if duration == 0 {
		duration = 5 * time.Minute
	}
	if duration < 5*time.Minute {
		return nil, xerrors.Errorf("refresh interval must be at least 5 mins")
	}

	return &MetricsCollector{
		database: db,
		logger:   logger.Named("insights_metrics_collector"),
		duration: duration,
	}, nil
}

func (mc *MetricsCollector) Run(ctx context.Context) (func(), error) {
	ctx, closeFunc := context.WithCancel(ctx)
	done := make(chan struct{})

	// Use time.Nanosecond to force an initial tick. It will be reset to the
	// correct duration after executing once.
	ticker := time.NewTicker(time.Nanosecond)
	doTick := func() {
		defer ticker.Reset(mc.duration)

		now := time.Now()
		startTime := now.Add(-mc.duration)
		endTime := now

		// TODO collect iteration time

		var userActivity []database.GetUserActivityInsightsRow
		var appInsights []database.GetTemplateAppInsightsRow
		var parameterInsights []database.GetTemplateParameterInsightsRow

		// Phase I: Fetch insights from database
		eg, egCtx := errgroup.WithContext(ctx)
		eg.SetLimit(3)

		eg.Go(func() error {
			var err error
			userActivity, err = mc.database.GetUserActivityInsights(egCtx, database.GetUserActivityInsightsParams{
				StartTime: startTime,
				EndTime:   endTime,
			})
			if err != nil {
				mc.logger.Error(ctx, "unable to fetch template insights from database", slog.Error(err))
			}
			return err
		})
		eg.Go(func() error {
			var err error
			appInsights, err = mc.database.GetTemplateAppInsights(ctx, database.GetTemplateAppInsightsParams{
				StartTime: startTime,
				EndTime:   endTime,
			})
			if err != nil {
				mc.logger.Error(ctx, "unable to fetch app insights from database", slog.Error(err))
			}
			return err
		})
		eg.Go(func() error {
			var err error
			parameterInsights, err = mc.database.GetTemplateParameterInsights(ctx, database.GetTemplateParameterInsightsParams{
				StartTime: startTime,
				EndTime:   endTime,
			})
			if err != nil {
				mc.logger.Error(ctx, "unable to fetch parameter insights from database", slog.Error(err))
			}
			return err
		})

		err := eg.Wait()
		if err != nil {
			return
		}

		// Phase 2: Collect template IDs, and fetch relevant details
		templateIDs := uniqueTemplateIDs(userActivity, appInsights, parameterInsights)
		templates, err := mc.database.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
			IDs: templateIDs,
		})
		if err != nil {
			mc.logger.Error(ctx, "unable to fetch template details from database", slog.Error(err))
			return
		}

		/*templateNames := */
		onlyTemplateNames(templates)
	}

	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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
	descCh <- activeUsersDesc
	descCh <- applicationsUsageSecondsDesc
	descCh <- parametersDesc
}

func (mc *MetricsCollector) Collect(metricsCh chan<- prometheus.Metric) {
	// Phase 3: Collect metrics

	// TODO
}

// Helper functions below.

func uniqueTemplateIDs(userActivity []database.GetUserActivityInsightsRow, appInsights []database.GetTemplateAppInsightsRow, parameterInsights []database.GetTemplateParameterInsightsRow) []uuid.UUID {
	tids := map[uuid.UUID]bool{}
	for _, t := range userActivity {
		for _, tid := range t.TemplateIDs {
			tids[tid] = true
		}
	}

	for _, a := range appInsights {
		for _, tid := range a.TemplateIDs {
			tids[tid] = true
		}
	}

	for _, p := range parameterInsights {
		for _, tid := range p.TemplateIDs {
			tids[tid] = true
		}
	}

	uniqueUUIDs := make([]uuid.UUID, len(tids))
	var i int
	for t := range tids {
		uniqueUUIDs[i] = t
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
