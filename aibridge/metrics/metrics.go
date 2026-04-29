package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var baseLabels = []string{"provider", "model"}

const (
	InterceptionCountStatusFailed    = "failed"
	InterceptionCountStatusCompleted = "completed"
)

type Metrics struct {
	// Interception-related metrics.
	InterceptionDuration  *prometheus.HistogramVec
	InterceptionCount     *prometheus.CounterVec
	InterceptionsInflight *prometheus.GaugeVec
	PassthroughCount      *prometheus.CounterVec

	// Prompt-related metrics.
	PromptCount *prometheus.CounterVec

	// Token-related metrics.
	TokenUseCount *prometheus.CounterVec

	// Tool-related metrics.
	InjectedToolUseCount    *prometheus.CounterVec
	NonInjectedToolUseCount *prometheus.CounterVec

	// Circuit breaker metrics.
	CircuitBreakerState   *prometheus.GaugeVec   // Current state (0=closed, 0.5=half-open, 1=open)
	CircuitBreakerTrips   *prometheus.CounterVec // Total times circuit opened
	CircuitBreakerRejects *prometheus.CounterVec // Requests rejected due to open circuit
}

// NewMetrics creates AND registers metrics. It will panic if a collector has already been registered.
// Note: we are not specifying namespace in the metrics; the provided registerer may specify a "namespace"
// using [prometheus.WrapRegistererWithPrefix].
func NewMetrics(reg prometheus.Registerer) *Metrics {
	return &Metrics{
		// Interception-related metrics.

		// Pessimistic cardinality: 3 providers, 5 models, 2 statuses, 3 routes, 3 methods, 10 clients = up to 2700 PER INITIATOR.
		InterceptionCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: "interceptions",
			Name:      "total",
			Help:      "The count of intercepted requests.",
		}, append(baseLabels, "status", "route", "method", "initiator_id", "client")),
		// Pessimistic cardinality: 3 providers, 5 models, 3 routes = up to 45.
		// NOTE: route is not unbounded because this is only for intercepted routes.
		InterceptionsInflight: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: "interceptions",
			Name:      "inflight",
			Help:      "The number of intercepted requests which are being processed.",
		}, append(baseLabels, "route")),
		// Pessimistic cardinality: 3 providers, 5 models, 7 buckets + 3 extra series (count, sum, +Inf) = up to 150.
		InterceptionDuration: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: "interceptions",
			Name:      "duration_seconds",
			Help: "The total duration of intercepted requests, in seconds. " +
				"The majority of this time will be the upstream processing of the request. " +
				"aibridge has no control over upstream processing time, so it's just an illustrative metric.",
			// TODO: add docs around determining aibridge's *own* latency with distributed traces
			//       once https://github.com/coder/aibridge/issues/26 lands.
			Buckets: []float64{0.5, 2, 5, 15, 30, 60, 120},
		}, baseLabels),

		// Pessimistic cardinality: 3 providers, 10 routes, 3 methods = up to 90.
		// NOTE: route is not unbounded because PassthroughRoutes (see provider.go) is a static list.
		PassthroughCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: "passthrough",
			Name:      "total",
			Help:      "The count of requests which were not intercepted but passed through to the upstream.",
		}, []string{"provider", "route", "method"}),

		// Prompt-related metrics.

		// Pessimistic cardinality: 3 providers, 5 models, 10 clients = up to 150 PER INITIATOR.
		PromptCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: "prompts",
			Name:      "total",
			Help:      "The number of prompts issued by users (initiators).",
		}, append(baseLabels, "initiator_id", "client")),

		// Token-related metrics.

		// Pessimistic cardinality: 3 providers, 5 models, 10 types, 10 clients = up to 1500 PER INITIATOR.
		TokenUseCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: "tokens",
			Name:      "total",
			Help:      "The number of tokens used by intercepted requests.",
		}, append(baseLabels, "type", "initiator_id", "client")),

		// Tool-related metrics.

		// Pessimistic cardinality: 3 providers, 5 models, 3 servers, 30 tools = up to 1350.
		InjectedToolUseCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: "injected_tool_invocations",
			Name:      "total",
			Help:      "The number of times an injected MCP tool was invoked by aibridge.",
		}, append(baseLabels, "server", "name")),
		// Pessimistic cardinality: 3 providers, 5 models, 30 tools = up to 450.
		NonInjectedToolUseCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: "non_injected_tool_selections",
			Name:      "total",
			Help:      "The number of times an AI model selected a tool to be invoked by the client.",
		}, append(baseLabels, "name")),

		// Circuit breaker metrics.

		// Pessimistic cardinality: 3 providers, 2 endpoints, 5 models = up to 30.
		CircuitBreakerState: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: "circuit_breaker",
			Name:      "state",
			Help:      "Current state of the circuit breaker (0=closed, 0.5=half-open, 1=open).",
		}, []string{"provider", "endpoint", "model"}),
		// Pessimistic cardinality: 3 providers, 2 endpoints, 5 models = up to 30.
		CircuitBreakerTrips: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: "circuit_breaker",
			Name:      "trips_total",
			Help:      "Total number of times the circuit breaker transitioned to open state.",
		}, []string{"provider", "endpoint", "model"}),
		// Pessimistic cardinality: 3 providers, 2 endpoints, 5 models = up to 30.
		CircuitBreakerRejects: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: "circuit_breaker",
			Name:      "rejects_total",
			Help:      "Total number of requests rejected due to open circuit breaker.",
		}, []string{"provider", "endpoint", "model"}),
	}
}
