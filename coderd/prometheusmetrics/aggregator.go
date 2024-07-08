package prometheusmetrics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/agentmetrics"

	"cdr.dev/slog"

	agentproto "github.com/coder/coder/v2/agent/proto"
)

const (
	// MetricHelpForAgent is a help string that replaces all agent metric help
	// messages. This is because a registry cannot have conflicting
	// help messages for the same metric in a "gather". If our coder agents are
	// on different versions, this is a possible scenario.
	metricHelpForAgent = "Metrics are forwarded from workspace agents connected to this instance of coderd."
)

const (
	loggerName = "prometheusmetrics"

	sizeCollectCh = 10
	sizeUpdateCh  = 4096

	defaultMetricsCleanupInterval = 2 * time.Minute
)

var MetricLabelValueEncoder = strings.NewReplacer("\\", "\\\\", "|", "\\|", ",", "\\,", "=", "\\=")

type MetricsAggregator struct {
	store map[metricKey]annotatedMetric

	log                    slog.Logger
	metricsCleanupInterval time.Duration

	collectCh chan (chan []prometheus.Metric)
	updateCh  chan updateRequest

	storeSizeGauge    prometheus.Gauge
	updateHistogram   prometheus.Histogram
	cleanupHistogram  prometheus.Histogram
	aggregateByLabels []string
}

type updateRequest struct {
	username      string
	workspaceName string
	agentName     string
	templateName  string

	metrics []*agentproto.Stats_Metric

	timestamp time.Time
}

type annotatedMetric struct {
	*agentproto.Stats_Metric

	username      string
	workspaceName string
	agentName     string
	templateName  string

	expiryDate time.Time

	aggregateByLabels []string
}

type metricKey struct {
	username      string
	workspaceName string
	agentName     string
	templateName  string

	metricName string
	labelsStr  string
}

func hashKey(req *updateRequest, m *agentproto.Stats_Metric) metricKey {
	labelPairs := make(sort.StringSlice, 0, len(m.GetLabels()))
	for _, label := range m.GetLabels() {
		if label.Value == "" {
			continue
		}
		labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", label.Name, MetricLabelValueEncoder.Replace(label.Value)))
	}
	labelPairs.Sort()
	return metricKey{
		username:      req.username,
		workspaceName: req.workspaceName,
		agentName:     req.agentName,
		templateName:  req.templateName,
		metricName:    m.Name,
		labelsStr:     strings.Join(labelPairs, ","),
	}
}

var _ prometheus.Collector = new(MetricsAggregator)

func (am *annotatedMetric) asPrometheus() (prometheus.Metric, error) {
	var (
		baseLabelNames  = am.aggregateByLabels
		baseLabelValues []string
		extraLabels     = am.Labels
	)

	for _, label := range baseLabelNames {
		val, err := am.getFieldByLabel(label)
		if err != nil {
			return nil, err
		}

		baseLabelValues = append(baseLabelValues, val)
	}

	labels := make([]string, 0, len(baseLabelNames)+len(extraLabels))
	labelValues := make([]string, 0, len(baseLabelNames)+len(extraLabels))

	labels = append(labels, baseLabelNames...)
	labelValues = append(labelValues, baseLabelValues...)

	for _, l := range extraLabels {
		labels = append(labels, l.Name)
		labelValues = append(labelValues, l.Value)
	}

	desc := prometheus.NewDesc(am.Name, metricHelpForAgent, labels, nil)
	valueType, err := asPrometheusValueType(am.Type)
	if err != nil {
		return nil, err
	}

	return prometheus.MustNewConstMetric(desc, valueType, am.Value, labelValues...), nil
}

// getFieldByLabel returns the related field value for a given label
func (am *annotatedMetric) getFieldByLabel(label string) (string, error) {
	var labelVal string
	switch label {
	case agentmetrics.LabelWorkspaceName:
		labelVal = am.workspaceName
	case agentmetrics.LabelTemplateName:
		labelVal = am.templateName
	case agentmetrics.LabelAgentName:
		labelVal = am.agentName
	case agentmetrics.LabelUsername:
		labelVal = am.username
	default:
		return "", xerrors.Errorf("unexpected label: %q", label)
	}

	return labelVal, nil
}

func (am *annotatedMetric) shallowCopy() annotatedMetric {
	stats := &agentproto.Stats_Metric{
		Name:   am.Name,
		Type:   am.Type,
		Value:  am.Value,
		Labels: am.Labels,
	}

	return annotatedMetric{
		Stats_Metric:  stats,
		username:      am.username,
		workspaceName: am.workspaceName,
		agentName:     am.agentName,
		templateName:  am.templateName,
		expiryDate:    am.expiryDate,
	}
}

func NewMetricsAggregator(logger slog.Logger, registerer prometheus.Registerer, duration time.Duration, aggregateByLabels []string) (*MetricsAggregator, error) {
	metricsCleanupInterval := defaultMetricsCleanupInterval
	if duration > 0 {
		metricsCleanupInterval = duration
	}

	storeSizeGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "prometheusmetrics",
		Name:      "metrics_aggregator_store_size",
		Help:      "The number of metrics stored in the aggregator",
	})
	err := registerer.Register(storeSizeGauge)
	if err != nil {
		return nil, err
	}

	updateHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "prometheusmetrics",
		Name:      "metrics_aggregator_execution_update_seconds",
		Help:      "Histogram for duration of metrics aggregator update in seconds.",
		Buckets:   []float64{0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.500, 1, 5, 10, 30},
	})
	err = registerer.Register(updateHistogram)
	if err != nil {
		return nil, err
	}

	cleanupHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "prometheusmetrics",
		Name:      "metrics_aggregator_execution_cleanup_seconds",
		Help:      "Histogram for duration of metrics aggregator cleanup in seconds.",
		Buckets:   []float64{0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.500, 1, 5, 10, 30},
	})
	err = registerer.Register(cleanupHistogram)
	if err != nil {
		return nil, err
	}

	return &MetricsAggregator{
		log:                    logger.Named(loggerName),
		metricsCleanupInterval: metricsCleanupInterval,

		store: map[metricKey]annotatedMetric{},

		collectCh: make(chan (chan []prometheus.Metric), sizeCollectCh),
		updateCh:  make(chan updateRequest, sizeUpdateCh),

		storeSizeGauge:   storeSizeGauge,
		updateHistogram:  updateHistogram,
		cleanupHistogram: cleanupHistogram,

		aggregateByLabels: aggregateByLabels,
	}, nil
}

// labelAggregator is used to control cardinality of collected Prometheus metrics by pre-aggregating series based on given labels.
type labelAggregator struct {
	aggregations map[string]float64
	metrics      map[string]annotatedMetric
}

func newLabelAggregator(size int) *labelAggregator {
	return &labelAggregator{
		aggregations: make(map[string]float64, size),
		metrics:      make(map[string]annotatedMetric, size),
	}
}

func (a *labelAggregator) aggregate(am annotatedMetric, labels []string) error {
	// Use a LabelSet because it can give deterministic fingerprints of label combinations regardless of map ordering.
	labelSet := make(model.LabelSet, len(labels))

	for _, label := range labels {
		val, err := am.getFieldByLabel(label)
		if err != nil {
			return err
		}

		labelSet[model.LabelName(label)] = model.LabelValue(val)
	}

	// Memoize based on the metric name & the unique combination of labels.
	key := fmt.Sprintf("%s:%v", am.Stats_Metric.Name, labelSet.FastFingerprint())

	// Aggregate the value based on the key.
	a.aggregations[key] += am.Value

	metric, found := a.metrics[key]
	if !found {
		// Take a copy of the given annotatedMetric because it may be manipulated later and contains pointers.
		metric = am.shallowCopy()
	}

	// Store the metric.
	metric.aggregateByLabels = labels
	metric.Value = a.aggregations[key]

	a.metrics[key] = metric

	return nil
}

func (a *labelAggregator) listMetrics() []annotatedMetric {
	var out []annotatedMetric
	for _, am := range a.metrics {
		out = append(out, am)
	}
	return out
}

func (ma *MetricsAggregator) Run(ctx context.Context) func() {
	ctx, cancelFunc := context.WithCancel(ctx)
	done := make(chan struct{})

	cleanupTicker := time.NewTicker(ma.metricsCleanupInterval)
	go func() {
		defer close(done)
		defer cleanupTicker.Stop()

		for {
			select {
			case req := <-ma.updateCh:
				ma.log.Debug(ctx, "update metrics")

				timer := prometheus.NewTimer(ma.updateHistogram)
				for _, m := range req.metrics {
					key := hashKey(&req, m)

					if val, ok := ma.store[key]; ok {
						val.Stats_Metric.Value = m.Value
						val.expiryDate = req.timestamp.Add(ma.metricsCleanupInterval)
						ma.store[key] = val
					} else {
						ma.store[key] = annotatedMetric{
							Stats_Metric:  m,
							username:      req.username,
							workspaceName: req.workspaceName,
							agentName:     req.agentName,
							templateName:  req.templateName,
							expiryDate:    req.timestamp.Add(ma.metricsCleanupInterval),
						}
					}
				}
				timer.ObserveDuration()

				ma.storeSizeGauge.Set(float64(len(ma.store)))
			case outputCh := <-ma.collectCh:
				ma.log.Debug(ctx, "collect metrics")

				var input []annotatedMetric
				output := make([]prometheus.Metric, 0, len(ma.store))

				if len(ma.aggregateByLabels) == 0 {
					ma.aggregateByLabels = agentmetrics.LabelAll
				}

				// If custom aggregation labels have not been chosen, generate Prometheus metrics without any pre-aggregation.
				// This results in higher cardinality, but may be desirable in larger deployments.
				//
				// Default behavior.
				if len(ma.aggregateByLabels) == len(agentmetrics.LabelAll) {
					for _, m := range ma.store {
						// Aggregate by all available metrics.
						m.aggregateByLabels = defaultAgentMetricsLabels
						input = append(input, m)
					}
				} else {
					// However, if custom aggregations have been chosen, we need to aggregate the values from the annotated
					// metrics because we cannot register multiple metric series with the same labels.
					la := newLabelAggregator(len(ma.store))

					for _, m := range ma.store {
						if err := la.aggregate(m, ma.aggregateByLabels); err != nil {
							ma.log.Error(ctx, "can't aggregate labels", slog.F("labels", strings.Join(ma.aggregateByLabels, ",")), slog.Error(err))
						}
					}

					input = la.listMetrics()
				}

				for _, m := range input {
					promMetric, err := m.asPrometheus()
					if err != nil {
						ma.log.Error(ctx, "can't convert Prometheus value type", slog.F("name", m.Name), slog.F("type", m.Type), slog.F("value", m.Value), slog.Error(err))
						continue
					}
					output = append(output, promMetric)
				}

				outputCh <- output
				close(outputCh)
			case <-cleanupTicker.C:
				ma.log.Debug(ctx, "clean expired metrics")

				timer := prometheus.NewTimer(ma.cleanupHistogram)
				now := time.Now()

				for key, val := range ma.store {
					if now.After(val.expiryDate) {
						delete(ma.store, key)
					}
				}

				timer.ObserveDuration()
				cleanupTicker.Reset(ma.metricsCleanupInterval)
				ma.storeSizeGauge.Set(float64(len(ma.store)))

			case <-ctx.Done():
				ma.log.Debug(ctx, "metrics aggregator is stopped")
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

var defaultAgentMetricsLabels = []string{agentmetrics.LabelUsername, agentmetrics.LabelWorkspaceName, agentmetrics.LabelAgentName, agentmetrics.LabelTemplateName}

// AgentMetricLabels are the labels used to decorate an agent's metrics.
// This list should match the list of labels in agentMetricsLabels.
type AgentMetricLabels struct {
	Username      string
	WorkspaceName string
	AgentName     string
	TemplateName  string
}

func (ma *MetricsAggregator) Collect(ch chan<- prometheus.Metric) {
	output := make(chan []prometheus.Metric, 1)

	select {
	case ma.collectCh <- output:
	default:
		ma.log.Error(context.Background(), "collect queue is full")
		return
	}

	for s := range output {
		for _, m := range s {
			ch <- m
		}
	}
}

func (ma *MetricsAggregator) Update(ctx context.Context, labels AgentMetricLabels, metrics []*agentproto.Stats_Metric) {
	select {
	case ma.updateCh <- updateRequest{
		username:      labels.Username,
		workspaceName: labels.WorkspaceName,
		agentName:     labels.AgentName,
		templateName:  labels.TemplateName,
		metrics:       metrics,

		timestamp: time.Now(),
	}:
	case <-ctx.Done():
		ma.log.Debug(ctx, "update request is canceled")
	default:
		ma.log.Error(ctx, "update queue is full")
	}
}

func asPrometheusValueType(metricType agentproto.Stats_Metric_Type) (prometheus.ValueType, error) {
	switch metricType {
	case agentproto.Stats_Metric_GAUGE:
		return prometheus.GaugeValue, nil
	case agentproto.Stats_Metric_COUNTER:
		return prometheus.CounterValue, nil
	default:
		return -1, xerrors.Errorf("unsupported value type: %s", metricType)
	}
}
