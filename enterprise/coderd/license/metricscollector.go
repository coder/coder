package license

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/codersdk"
)

type MetricsCollector struct {
	Entitlements atomic.Pointer[codersdk.Entitlements]

	activeUsersGauge      prometheus.Gauge
	limitUsersGauge       prometheus.Gauge
	userLimitEnabledGauge prometheus.Gauge
}

var _ prometheus.Collector = new(MetricsCollector)

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		activeUsersGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "license",
			Name:      "active_users",
			Help:      `The number of active users.`,
		}),
		limitUsersGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "license",
			Name:      "limit_users",
			Help:      "The user seats limit based on the active Coder license.",
		}),
		userLimitEnabledGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "license",
			Name:      "user_limit_enabled",
			Help:      "Returns 1 if the current license enforces the user limit.",
		}),
	}
}

func (mc *MetricsCollector) Describe(descCh chan<- *prometheus.Desc) {
	descCh <- mc.activeUsersGauge.Desc()
	descCh <- mc.limitUsersGauge.Desc()
	descCh <- mc.userLimitEnabledGauge.Desc()
}

func (mc *MetricsCollector) Collect(metricsCh chan<- prometheus.Metric) {
	entitlements := mc.Entitlements.Load()
	if entitlements == nil || entitlements.Features == nil {
		return
	}

	userLimitEntitlement, ok := entitlements.Features[codersdk.FeatureUserLimit]
	if !ok {
		return
	}

	var enabled float64
	if userLimitEntitlement.Enabled {
		enabled = 1
	}
	mc.userLimitEnabledGauge.Set(enabled)

	if userLimitEntitlement.Actual != nil {
		mc.activeUsersGauge.Set(float64(*userLimitEntitlement.Actual))
	}

	if userLimitEntitlement.Limit != nil {
		mc.limitUsersGauge.Set(float64(*userLimitEntitlement.Limit))
	}

	metricsCh <- mc.activeUsersGauge
	metricsCh <- mc.limitUsersGauge
	metricsCh <- mc.userLimitEnabledGauge
}
