package provisionerdserver

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"cdr.dev/slog"
)

type Metrics struct {
	logger                         slog.Logger
	workspaceCreationTimings       *prometheus.HistogramVec
	workspaceClaimTimings          *prometheus.HistogramVec
	workspaceCreationOutcomesTotal *prometheus.CounterVec
}

type WorkspaceTimingType int

const (
	Unsupported WorkspaceTimingType = iota
	WorkspaceCreation
	PrebuildCreation
	PrebuildClaim
)

const (
	workspaceTypeRegular  = "regular"
	workspaceTypePrebuild = "prebuild"
)

type WorkspaceTimingFlags struct {
	IsPrebuild   bool
	IsClaim      bool
	IsFirstBuild bool
}

func NewMetrics(logger slog.Logger, reg prometheus.Registerer) *Metrics {
	log := logger.Named("provisionerd_server_metrics")
	factory := promauto.With(reg)

	return &Metrics{
		logger: log,
		workspaceCreationTimings: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Name:      "workspace_creation_duration_seconds",
			Help:      "Time to create a workspace by organization, template, preset, and type (regular or prebuild).",
			Buckets: []float64{
				1, // 1s
				10,
				30,
				60, // 1min
				60 * 5,
				60 * 10,
				60 * 30, // 30min
				60 * 60, // 1hr
			},
			NativeHistogramBucketFactor: 1.1,
			// Max number of native buckets kept at once to bound memory.
			NativeHistogramMaxBucketNumber: 100,
			// Merge/flush small buckets periodically to control churn.
			NativeHistogramMinResetDuration: time.Hour,
			// Treat tiny values as zero (helps with noisy near-zero latencies).
			NativeHistogramZeroThreshold:    0,
			NativeHistogramMaxZeroThreshold: 0,
		}, []string{"organization_name", "template_name", "preset_name", "type"}),
		workspaceClaimTimings: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Name:      "prebuilt_workspace_claim_duration_seconds",
			Help:      "Time to claim a prebuilt workspace by organization, template, and preset.",
			// Higher resolution between 1–5m to show typical prebuild claim times.
			// Cap at 5m since longer claims diminish prebuild value.
			Buckets: []float64{
				1, // 1s
				5,
				10,
				20,
				30,
				60,  // 1m
				120, // 2m
				180, // 3m
				240, // 4m
				300, // 5m
			},
			NativeHistogramBucketFactor: 1.1,
			// Max number of native buckets kept at once to bound memory.
			NativeHistogramMaxBucketNumber: 100,
			// Merge/flush small buckets periodically to control churn.
			NativeHistogramMinResetDuration: time.Hour,
			// Treat tiny values as zero (helps with noisy near-zero latencies).
			NativeHistogramZeroThreshold:    0,
			NativeHistogramMaxZeroThreshold: 0,
		}, []string{"organization_name", "template_name", "preset_name"}),
		workspaceCreationOutcomesTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coderd",
				Subsystem: "provisionerd",
				Name:      "workspace_creation_outcomes_total",
				Help:      "Outcomes of regular (non-prebuilt) workspace first builds by organization, template, preset, and status.",
			},
			[]string{"organization_name", "template_name", "preset_name", "status"},
		),
	}
}

// IsTrackable returns true if the workspace build should be tracked in metrics.
// This includes workspace creation, prebuild creation, and prebuild claims.
func (f WorkspaceTimingFlags) IsTrackable() bool {
	return f.IsPrebuild || f.IsClaim || f.IsFirstBuild
}

// getWorkspaceTimingType classifies a workspace build:
//   - PrebuildCreation: creation of a prebuilt workspace
//   - PrebuildClaim: claim of an existing prebuilt workspace
//   - WorkspaceCreation: first build of a regular (non-prebuilt) workspace
//
// Note: order matters. Creating a prebuilt workspace is also a first build
// (IsPrebuild && IsFirstBuild). We check IsPrebuild before IsFirstBuild so
// prebuilds take precedence. This is the only case where two flags can be true.
func getWorkspaceTimingType(flags WorkspaceTimingFlags) WorkspaceTimingType {
	switch {
	case flags.IsPrebuild:
		return PrebuildCreation
	case flags.IsClaim:
		return PrebuildClaim
	case flags.IsFirstBuild:
		return WorkspaceCreation
	default:
		return Unsupported
	}
}

// UpdateWorkspaceTimingsMetrics updates the workspace timing metrics based on the workspace build type
func (m *Metrics) UpdateWorkspaceTimingsMetrics(
	ctx context.Context,
	flags WorkspaceTimingFlags,
	organizationName string,
	templateName string,
	presetName string,
	buildTime float64,
) {
	m.logger.Debug(ctx, "update workspace timings metrics",
		"organizationName", organizationName,
		"templateName", templateName,
		"presetName", presetName,
		"isPrebuild", flags.IsPrebuild,
		"isClaim", flags.IsClaim,
		"isWorkspaceFirstBuild", flags.IsFirstBuild)

	workspaceTimingType := getWorkspaceTimingType(flags)
	switch workspaceTimingType {
	case WorkspaceCreation:
		// Regular workspace creation (without prebuild pool)
		m.workspaceCreationTimings.
			WithLabelValues(organizationName, templateName, presetName, workspaceTypeRegular).Observe(buildTime)
	case PrebuildCreation:
		// Prebuilt workspace creation duration
		m.workspaceCreationTimings.
			WithLabelValues(organizationName, templateName, presetName, workspaceTypePrebuild).Observe(buildTime)
	case PrebuildClaim:
		// Prebuilt workspace claim duration
		m.workspaceClaimTimings.
			WithLabelValues(organizationName, templateName, presetName).Observe(buildTime)
	default:
		// Not a trackable build type (e.g. restart, stop, subsequent builds)
	}
}
