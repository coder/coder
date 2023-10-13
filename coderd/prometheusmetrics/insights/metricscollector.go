package insights

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/coderd/database"
)

var (
	activeUsersDesc              = prometheus.NewDesc("coderd_insights_active_users", "The number of active users of the template.", []string{"template_name"}, nil)
	applicationsUsageSecondsDesc = prometheus.NewDesc("coderd_insights_applications_usage_seconds", "The application usage per template.", []string{"application_name", "template_name"}, nil)
	parametersDesc               = prometheus.NewDesc("coderd_insights_parameters", "The parameter usage per template.", []string{"template_name", "name", "value"}, nil)
)

type MetricsCollector struct {
	database database.Store
	duration time.Duration
}

var _ prometheus.Collector = new(MetricsCollector)

func NewMetricsCollector(db database.Store, duration time.Duration) *MetricsCollector {
	if duration == 0 {
		duration = 5 * time.Minute
	}

	return &MetricsCollector{
		database: db,
		duration: duration,
	}
}

func (mc *MetricsCollector) Run(ctx context.Context) (func(), error) {
	ctx, closeFunc := context.WithCancel(ctx)
	done := make(chan struct{})

	// Use time.Nanosecond to force an initial tick. It will be reset to the
	// correct duration after executing once.
	ticker := time.NewTicker(time.Nanosecond)
	doTick := func() {
		defer ticker.Reset(mc.duration)
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
