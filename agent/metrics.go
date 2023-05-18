package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"tailscale.com/util/clientmetric"

	"cdr.dev/slog"

	"github.com/coder/coder/codersdk/agentsdk"
)

type agentMetrics struct {
	handler            prometheus.Counter
	handlerError       prometheus.Counter
	createCommandError prometheus.Counter
	cmdStartError      prometheus.Counter
	writeError         prometheus.Counter
	resizeError        prometheus.Counter
	outputReaderError  prometheus.Counter
	inputWriterError   prometheus.Counter
}

func newAgentMetrics(registerer prometheus.Registerer) *agentMetrics {
	handler := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "reconnecting_pty", Name: "handler",
	})
	registerer.MustRegister(handler)

	handlerError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "reconnecting_pty", Name: "error",
	})
	registerer.MustRegister(handlerError)

	createCommandError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "reconnecting_pty", Name: "create_command_error",
	})
	registerer.MustRegister(createCommandError)

	cmdStartError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "reconnecting_pty", Name: "cmd_start_error",
	})
	registerer.MustRegister(cmdStartError)

	writeError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "reconnecting_pty", Name: "write_error",
	})
	registerer.MustRegister(writeError)

	resizeError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "reconnecting_pty", Name: "resize_error",
	})
	registerer.MustRegister(resizeError)

	outputReaderError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "reconnecting_pty", Name: "output_reader_error",
	})
	registerer.MustRegister(outputReaderError)

	inputWriterError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "reconnecting_pty", Name: "input_writer_error",
	})
	registerer.MustRegister(inputWriterError)

	return &agentMetrics{
		handler:            handler,
		handlerError:       handlerError,
		createCommandError: createCommandError,
		cmdStartError:      cmdStartError,
		writeError:         writeError,
		resizeError:        resizeError,
		outputReaderError:  outputReaderError,
		inputWriterError:   inputWriterError,
	}
}

func (a *agent) collectMetrics(ctx context.Context) []agentsdk.AgentMetric {
	var collected []agentsdk.AgentMetric

	// Tailscale internal metrics
	metrics := clientmetric.Metrics()
	for _, m := range metrics {
		if isIgnoredMetric(m.Name()) {
			continue
		}

		collected = append(collected, agentsdk.AgentMetric{
			Name:  m.Name(),
			Type:  asMetricType(m.Type()),
			Value: float64(m.Value()),
		})
	}

	// Agent metrics
	metricFamilies, err := a.prometheusRegistry.Gather()
	if err != nil {
		a.logger.Error(ctx, "can't gather agent metrics", slog.Error(err))
		return collected
	}

	for _, metricFamily := range metricFamilies {
		for _, metric := range metricFamily.GetMetric() {
			if metric.Counter != nil {
				collected = append(collected, agentsdk.AgentMetric{
					Name:  metricFamily.GetName(),
					Type:  agentsdk.AgentMetricTypeCounter,
					Value: metric.Counter.GetValue(),
				})
			} else if metric.Gauge != nil {
				collected = append(collected, agentsdk.AgentMetric{
					Name:  metricFamily.GetName(),
					Type:  agentsdk.AgentMetricTypeGauge,
					Value: metric.Gauge.GetValue(),
				})
			} else {
				a.logger.Error(ctx, "unsupported metric type", slog.F("type", metricFamily.Type.String()))
			}
		}
	}
	return collected
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

func asMetricType(typ clientmetric.Type) agentsdk.AgentMetricType {
	switch typ {
	case clientmetric.TypeGauge:
		return agentsdk.AgentMetricTypeGauge
	case clientmetric.TypeCounter:
		return agentsdk.AgentMetricTypeCounter
	default:
		panic(fmt.Sprintf("unknown metric type: %d", typ))
	}
}
