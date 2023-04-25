package prometheusmetrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/codersdk/agentsdk"
)

const (
	// MetricHelpForAgent is a help string that replaces all agent metric help
	// messages. This is because a registry cannot have conflicting
	// help messages for the same metric in a "gather". If our coder agents are
	// on different versions, this is a possible scenario.
	metricHelpForAgent = "Metric is forwarded from workspace agent connected to this instance of coderd."
)

const (
	sizeCollectCh = 10
	sizeUpdateCh  = 1024
)

type MetricsAggregator struct {
	queue []annotatedMetric

	log slog.Logger

	collectCh chan (chan<- prometheus.Metric)
	updateCh  chan updateRequest
}

type updateRequest struct {
	username      string
	workspaceName string
	agentName     string

	metrics []agentsdk.AgentMetric
}

type annotatedMetric struct {
	agentsdk.AgentMetric

	username      string
	workspaceName string
	agentName     string
}

var _ prometheus.Collector = new(MetricsAggregator)

func NewMetricsAggregator(logger slog.Logger) *MetricsAggregator {
	return &MetricsAggregator{
		log: logger,

		collectCh: make(chan (chan<- prometheus.Metric), sizeCollectCh),
		updateCh:  make(chan updateRequest, sizeUpdateCh),
	}
}

func (ma *MetricsAggregator) Run(ctx context.Context) func() {
	ctx, cancelFunc := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)

		for {
			select {
			case req := <-ma.updateCh:
			UpdateLoop:
				for _, m := range req.metrics {
					for i, q := range ma.queue {
						if q.username == req.username && q.workspaceName == req.workspaceName && q.agentName == req.agentName && q.Name == m.Name {
							ma.queue[i].AgentMetric.Value = m.Value
							continue UpdateLoop
						}
					}

					ma.queue = append(ma.queue, annotatedMetric{
						username:      req.username,
						workspaceName: req.workspaceName,
						agentName:     req.agentName,

						AgentMetric: m,
					})
				}
			case inputCh := <-ma.collectCh:
				for _, m := range ma.queue {
					desc := prometheus.NewDesc(m.Name, metricHelpForAgent, agentMetricsLabels, nil)
					valueType, err := asPrometheusValueType(m.Type)
					if err != nil {
						ma.log.Error(ctx, "can't convert Prometheus value type", slog.F("value_type", m.Type), slog.Error(err))
						continue
					}
					constMetric := prometheus.MustNewConstMetric(desc, valueType, m.Value, m.username, m.workspaceName, m.agentName)
					inputCh <- constMetric
				}
				close(inputCh)
			case <-ctx.Done():
				ma.log.Debug(ctx, "metrics aggregator: is stopped")
				return
			}
		}
	}()
	return func() {
		cancelFunc()
		<-done
	}
}

// Describe function does not have any knowledge about the metrics schema,
// so it does not emit anything.
func (*MetricsAggregator) Describe(_ chan<- *prometheus.Desc) {
}

var agentMetricsLabels = []string{usernameLabel, workspaceNameLabel, agentNameLabel}

func (ma *MetricsAggregator) Collect(ch chan<- prometheus.Metric) {
	collect := make(chan prometheus.Metric, 128)

	select {
	case ma.collectCh <- collect:
	default:
		ma.log.Error(context.Background(), "metrics aggregator: collect queue is full")
		return
	}

	for m := range collect {
		ch <- m
	}
}

func (ma *MetricsAggregator) Update(ctx context.Context, username, workspaceName, agentName string, metrics []agentsdk.AgentMetric) {
	select {
	case ma.updateCh <- updateRequest{
		username:      username,
		workspaceName: workspaceName,
		agentName:     agentName,
		metrics:       metrics,
	}:
	case <-ctx.Done():
		ma.log.Debug(ctx, "metrics aggregator: update is canceled")
	default:
		ma.log.Error(ctx, "metrics aggregator: update queue is full")
	}
}

func asPrometheusValueType(metricType agentsdk.AgentMetricType) (prometheus.ValueType, error) {
	switch metricType {
	case agentsdk.AgentMetricTypeGauge:
		return prometheus.GaugeValue, nil
	case agentsdk.AgentMetricTypeCounter:
		return prometheus.CounterValue, nil
	default:
		return -1, xerrors.Errorf("unsupported value type: %s", metricType)
	}
}
