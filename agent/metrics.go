package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cdr.dev/slog"
	"github.com/prometheus/client_golang/prometheus"
	prompb "github.com/prometheus/client_model/go"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/clientmetric"
	"tailscale.com/net/netcheck"

	proto "github.com/coder/coder/v2/agent/proto"
)

type agentMetrics struct {
	startupScriptSeconds *prometheus.GaugeVec

	// I/O metrics
	bytesSent     prometheus.Counter
	bytesReceived prometheus.Counter

	// Session metrics
	sessionsTotal         prometheus.Counter
	sessionsClosed        prometheus.Counter
	sessionsActive        prometheus.Gauge
	sessionReconnectCount *prometheus.CounterVec

	// Connection metrics
	currentConnections *prometheus.GaugeVec

	// Tailscale Peer metrics
	peerStatus         *prometheus.GaugeVec
	magicsockLoss      prometheus.Gauge
	magicsockLatency   prometheus.Gauge
	networkMapPingCost *prometheus.GaugeVec

	// Prometheus Registry
	prometheusRegistry *prometheus.Registry
	logger             slog.Logger
}

func (a *Agent) newMetrics() error {
	startupScriptSeconds := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "startup_script_seconds",
		Help: "Total number of seconds spent executing a startup script.",
	}, []string{"status"})

	// Connection metrics
	currentConnections := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "current_connections",
		Help: "Current active connection count.",
	}, []string{"type"})

	a.metrics = &agentMetrics{
		startupScriptSeconds: startupScriptSeconds,
		currentConnections:   currentConnections,
	}
	return nil
}

func (a *Agent) collectMetrics(ctx context.Context) []*proto.Stats_Metric {
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

	metricFamilies, err := a.metrics.prometheusRegistry.Gather()
	if err != nil {
		a.logger.Error(ctx, "can't gather agent metrics", slog.Error(err))
		return collected
	}

	for _, metricFamily := range metricFamilies {
		for _, metric := range metricFamily.GetMetric() {
			labels := toAgentMetricLabels(metric.Label)

			switch {
			case metric.Counter != nil:
				collected = append(collected, &proto.Stats_Metric{
					Name:   metricFamily.GetName(),
					Type:   proto.Stats_Metric_COUNTER,
					Value:  metric.Counter.GetValue(),
					Labels: labels,
				})
			case metric.Gauge != nil:
				collected = append(collected, &proto.Stats_Metric{
					Name:   metricFamily.GetName(),
					Type:   proto.Stats_Metric_GAUGE,
					Value:  metric.Gauge.GetValue(),
					Labels: labels,
				})
			default:
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

// parseNetInfoToMetrics parses Tailscale's netcheck data into a list of metrics
func parseNetInfoToMetrics(data *netcheck.Report) []apitype.TailnetDERPRegionProbe {
	if data == nil {
		return nil
	}

	var res []apitype.TailnetDERPRegionProbe
	for id, region := range data.DERPRegionLatency {
		res = append(res, apitype.TailnetDERPRegionProbe{
			RegionID:   int(id),
			RegionCode: data.RegionV4Servers[id],
			LatencyMs:  float64(region.Milliseconds()),
		})
	}
	return res
}