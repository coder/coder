package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	prompb "github.com/prometheus/client_model/go"
	"tailscale.com/util/clientmetric"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
)

type agentMetrics struct {
	connectionsTotal      prometheus.Counter
	reconnectingPTYErrors *prometheus.CounterVec
	// startupScriptSeconds is the time in seconds that the start script(s)
	// took to run. This is reported once per agent.
	startupScriptSeconds *prometheus.GaugeVec
}

func newAgentMetrics(registerer prometheus.Registerer) *agentMetrics {
	connectionsTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "reconnecting_pty", Name: "connections_total",
	})
	registerer.MustRegister(connectionsTotal)

	reconnectingPTYErrors := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "agent",
			Subsystem: "reconnecting_pty",
			Name:      "errors_total",
		},
		[]string{"error_type"},
	)
	registerer.MustRegister(reconnectingPTYErrors)

	startupScriptSeconds := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "agentstats",
		Name:      "startup_script_seconds",
		Help:      "Amount of time taken to run the startup script in seconds.",
	}, []string{"success"})
	registerer.MustRegister(startupScriptSeconds)

	return &agentMetrics{
		connectionsTotal:      connectionsTotal,
		reconnectingPTYErrors: reconnectingPTYErrors,
		startupScriptSeconds:  startupScriptSeconds,
	}
}

func (a *agent) collectMetrics(ctx context.Context) []*proto.Stats_Metric {
	var collected []*proto.Stats_Metric

	// Tailscale internal metrics
	metrics := clientmetric.Metrics()
	for _, m := range metrics {
		if isIgnoredMetric(m.Name()) {
			continue
		}

		collected = append(collected, &proto.Stats_Metric{
			Name:  m.Name(),
			Type:  asMetricType(m.Type()),
			Value: float64(m.Value()),
		})
	}

	metricFamilies, err := a.prometheusRegistry.Gather()
	if err != nil {
		a.logger.Error(ctx, "can't gather agent metrics", slog.Error(err))
		return collected
	}

	for _, metricFamily := range metricFamilies {
		for _, metric := range metricFamily.GetMetric() {
			labels := toAgentMetricLabels(metric.Label)

			if metric.Counter != nil {
				collected = append(collected, &proto.Stats_Metric{
					Name:   metricFamily.GetName(),
					Type:   proto.Stats_Metric_COUNTER,
					Value:  metric.Counter.GetValue(),
					Labels: labels,
				})
			} else if metric.Gauge != nil {
				collected = append(collected, &proto.Stats_Metric{
					Name:   metricFamily.GetName(),
					Type:   proto.Stats_Metric_GAUGE,
					Value:  metric.Gauge.GetValue(),
					Labels: labels,
				})
			} else {
				a.logger.Error(ctx, "unsupported metric type", slog.F("type", metricFamily.Type.String()))
			}
		}
	}
	return collected
}

func toAgentMetricLabels(metricLabels []*prompb.LabelPair) []*proto.Stats_Metric_Label {
	if len(metricLabels) == 0 {
		return nil
	}

	labels := make([]*proto.Stats_Metric_Label, 0, len(metricLabels))
	for _, metricLabel := range metricLabels {
		labels = append(labels, &proto.Stats_Metric_Label{
			Name:  metricLabel.GetName(),
			Value: metricLabel.GetValue(),
		})
	}
	return labels
}

// isIgnoredMetric checks if the metric should be ignored, as Coder agent doesn't use related features.
// Expected metric families: magicsock_*, derp_*, tstun_*, netcheck_*, portmap_*, etc.
func isIgnoredMetric(metricName string) bool {
	if strings.HasPrefix(metricName, "dns_") ||
		strings.HasPrefix(metricName, "controlclient_") ||
		strings.HasPrefix(metricName, "peerapi_") ||
		strings.HasPrefix(metricName, "profiles_") ||
		strings.HasPrefix(metricName, "tstun_") {
		return true
	}
	return false
}

func asMetricType(typ clientmetric.Type) proto.Stats_Metric_Type {
	switch typ {
	case clientmetric.TypeGauge:
		return proto.Stats_Metric_GAUGE
	case clientmetric.TypeCounter:
		return proto.Stats_Metric_COUNTER
	default:
		panic(fmt.Sprintf("unknown metric type: %d", typ))
	}
}
