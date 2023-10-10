package prometheusmetrics

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

type LicenseMetrics struct {
	db       database.Store
	interval time.Duration
	logger   slog.Logger
	registry *prometheus.Registry

	Entitlements atomic.Pointer[codersdk.Entitlements]
}

type LicenseMetricsOptions struct {
	Interval time.Duration
	Database database.Store
	Logger   slog.Logger
	Registry *prometheus.Registry
}

func NewLicenseMetrics(opts *LicenseMetricsOptions) (*LicenseMetrics, error) {
	if opts.Interval == 0 {
		opts.Interval = 1 * time.Minute
	}
	if opts.Database == nil {
		return nil, xerrors.Errorf("database is required")
	}
	if opts.Registry == nil {
		opts.Registry = prometheus.NewRegistry()
	}

	return &LicenseMetrics{
		db:       opts.Database,
		interval: opts.Interval,
		logger:   opts.Logger,
		registry: opts.Registry,
	}, nil
}

func (lm *LicenseMetrics) Collect(ctx context.Context) (func(), error) {
	licenseLimitGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "license",
		Name:      "user_limit",
		Help:      `The user seats limit based on the current license. "Zero" means unlimited or a disabled feature.`,
	})
	err := lm.registry.Register(licenseLimitGauge)
	if err != nil {
		return nil, err
	}

	activeUsersGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "license",
		Name:      "active_users",
		Help:      "The number of active users.",
	})
	err = lm.registry.Register(activeUsersGauge)
	if err != nil {
		return nil, err
	}

	userLimitGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "license",
		Name:      "user_limit",
		Help:      "The user seats limit based on the active Coder license.",
	})
	err = lm.registry.Register(activeUsersGauge)
	if err != nil {
		return nil, err
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	done := make(chan struct{})
	ticker := time.NewTicker(lm.interval)
	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

		}
	}()
	return func() {
		cancelFunc()
		<-done
	}, nil
}
