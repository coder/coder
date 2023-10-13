package insights

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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

		// TODO collect iteration time

		parameterRows, err := mc.database.GetTemplateInsights(ctx, database.GetTemplateInsightsParams{
			StartTime: now.Add(-mc.duration),
			EndTime:   now,
		})
		if err != nil {
			mc.logger.Error(ctx, "unable to fetch template insights from database", slog.Error(err))
			return
		}

		mc.logger.Info(ctx, "debug", slog.F("parameter_rows", parameterRows))
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
}
