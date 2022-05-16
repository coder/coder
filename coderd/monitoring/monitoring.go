package monitoring

import (
	"context"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
)

type Telemetry string

const (
	TelemetryAll  Telemetry = "all"
	TelemetryCore Telemetry = "core"
	TelemetryNone Telemetry = "none"
)

// ParseTelemetry returns a valid Telemetry or error if input is not a valid.
func ParseTelemetry(t string) (Telemetry, error) {
	ok := []string{
		string(TelemetryAll),
		string(TelemetryCore),
		string(TelemetryNone),
	}

	for _, a := range ok {
		if strings.EqualFold(a, t) {
			return Telemetry(a), nil
		}
	}

	return "", xerrors.Errorf(`invalid telemetry level: %s, must be one of: %s`, t, strings.Join(ok, ","))
}

type Options struct {
	Database        database.Store
	Logger          slog.Logger
	RefreshInterval time.Duration
	Telemetry       Telemetry
}

// Monitor provides Prometheus registries on which to register metric
// collectors. Depending on the level these metrics may also be sent to Coder.
// Monitor automatically registers a collector that collects statistics from the
// database.
type Monitor struct {
	// allRegistry registers metrics that will be sent when the telemetry level
	// is `all`.
	allRegistry *prometheus.Registry
	// coreRegistry registers metrics that will be sent when the telemetry level
	// is `core` or `all`.
	coreRegistry *prometheus.Registry
	// internalRegistry registers metrics that will never be sent.
	internalRegistry *prometheus.Registry
	// Telemetry determines which metrics are sent to Coder.
	Telemetry Telemetry
}

func New(ctx context.Context, options *Options) *Monitor {
	monitor := Monitor{
		allRegistry:      prometheus.NewRegistry(),
		coreRegistry:     prometheus.NewRegistry(),
		internalRegistry: prometheus.NewRegistry(),
		Telemetry:        options.Telemetry,
	}

	monitor.MustRegister(TelemetryAll, NewCollector(ctx, options.Database))

	return &monitor
}

// MustRegister registers collectors at the specified level.
func (t Monitor) MustRegister(level Telemetry, cs ...prometheus.Collector) {
	switch level {
	case TelemetryAll:
		t.allRegistry.MustRegister(cs...)
	case TelemetryCore:
		t.coreRegistry.MustRegister(cs...)
	case TelemetryNone:
		t.internalRegistry.MustRegister(cs...)
	}
}
