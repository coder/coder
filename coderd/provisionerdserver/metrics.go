package provisionerdserver

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog"
)

type Metrics struct {
	logger                   slog.Logger
	workspaceCreationTimings *prometheus.HistogramVec
	workspaceClaimTimings    *prometheus.HistogramVec
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

type UpdateWorkspaceTimingMetricsFn func(
	workspaceTimingType WorkspaceTimingType,
	organizationName string,
	templateName string,
	presetName string,
	buildTime float64,
)

func NewMetrics(logger slog.Logger) *Metrics {
	log := logger.Named("provisionerd_server_metrics")

	return &Metrics{
		logger: log,
		workspaceCreationTimings: prometheus.NewHistogramVec(prometheus.HistogramOpts{
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
		workspaceClaimTimings: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Name:      "prebuilt_workspace_claim_duration_seconds",
			Help:      "Time to claim a prebuilt workspace by organization, template, and preset.",
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
	}
}

func (m *Metrics) Register(reg prometheus.Registerer) error {
	if err := reg.Register(m.workspaceCreationTimings); err != nil {
		return err
	}
	return reg.Register(m.workspaceClaimTimings)
}

// getWorkspaceTimingType returns the type of the workspace build:
//   - isPrebuild: if the workspace build corresponds to the creation of a prebuilt workspace
//   - isClaim: if the workspace build corresponds to the claim of a prebuilt workspace
//   - isWorkspaceFirstBuild: if the workspace build corresponds to the creation of a regular workspace
//     (not created from the prebuild pool)
func getWorkspaceTimingType(isPrebuild bool, isClaim bool, isWorkspaceFirstBuild bool) WorkspaceTimingType {
	switch {
	case isPrebuild:
		return PrebuildCreation
	case isClaim:
		return PrebuildClaim
	case isWorkspaceFirstBuild:
		return WorkspaceCreation
	default:
		return Unsupported
	}
}

// UpdateWorkspaceTimingsMetrics updates the workspace timing metrics based on the workspace build type
func (m *Metrics) UpdateWorkspaceTimingsMetrics(
	isPrebuild bool,
	isClaim bool,
	isWorkspaceFirstBuild bool,
	organizationName string,
	templateName string,
	presetName string,
	buildTime float64,
) {
	workspaceTimingType := getWorkspaceTimingType(isPrebuild, isClaim, isWorkspaceFirstBuild)
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
	}
}
