package prometheusmetrics

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/codersdk"
)

type LicenseMetrics struct {
	interval time.Duration
	logger   slog.Logger
	registry *prometheus.Registry

	Entitlements atomic.Pointer[codersdk.Entitlements]
}

type LicenseMetricsOptions struct {
	Interval time.Duration
	Logger   slog.Logger
	Registry *prometheus.Registry
}

func NewLicenseMetrics(opts *LicenseMetricsOptions) (*LicenseMetrics, error) {
	if opts.Interval == 0 {
		opts.Interval = 1 * time.Minute
	}
	if opts.Registry == nil {
		opts.Registry = prometheus.NewRegistry()
	}

	return &LicenseMetrics{
		interval: opts.Interval,
		logger:   opts.Logger,
		registry: opts.Registry,
	}, nil
}

func (lm *LicenseMetrics) Collect(ctx context.Context) (func(), error) {
	activeUsersGauge := NewCachedGaugeVec(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "license",
		Name:      "active_users",
		Help:      `The number of active users.`,
	}, []string{"entitled"}))
	err := lm.registry.Register(activeUsersGauge)
	if err != nil {
		return nil, err
	}

	userLimitGauge := NewCachedGaugeVec(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "license",
		Name:      "user_limit",
		Help:      "The user seats limit based on the active Coder license.",
	}, []string{"entitled"}))
	err = lm.registry.Register(userLimitGauge)
	if err != nil {
		return nil, err
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	done := make(chan struct{})
	ticker := time.NewTicker(time.Nanosecond)

	doTick := func() {
		defer ticker.Reset(lm.interval)

		entitlements := lm.Entitlements.Load()
		userLimitEntitlement, ok := entitlements.Features[codersdk.FeatureUserLimit]
		if !ok {
			lm.logger.Warn(ctx, `"user_limit" entitlement is not present`)
			return
		}

		enabled := fmt.Sprintf("%v", userLimitEntitlement.Enabled)
		if userLimitEntitlement.Actual != nil {
			activeUsersGauge.WithLabelValues(VectorOperationSet, float64(*userLimitEntitlement.Actual), enabled)
		} else {
			activeUsersGauge.WithLabelValues(VectorOperationSet, 0, enabled)
		}

		if userLimitEntitlement.Limit != nil {
			userLimitGauge.WithLabelValues(VectorOperationSet, float64(*userLimitEntitlement.Limit), enabled)
		} else {
			userLimitGauge.WithLabelValues(VectorOperationSet, 0, enabled)
		}

		activeUsersGauge.Commit()
		userLimitGauge.Commit()
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
		cancelFunc()
		<-done
	}, nil
}
