package prometheusmetrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type cachableMetric interface {
	prometheus.Collector
	Reset()

	// Process commits the staged changes to the metric. No error can be returned,
	// just do best effort to process the records.
	Process(records []vectorRecord)
}

var _ prometheus.Collector = new(CachedMetric)

// CachedMetric is a wrapper for the prometheus.MetricVec which allows
// for staging changes in the metrics vector. Calling "WithLabelValues(...)"
// will update the internal metric value, but it will not be returned by
// "Collect(...)" until the "Commit()" method is called. The "Commit()" method
// resets the internal gauge and applies all staged changes to it.
//
// The Use of CachedGaugeVec is recommended for use cases when there is a risk
// that the Prometheus collector receives incomplete metrics, collected
// in the middle of metrics recalculation, between "Reset()" and the last
// "WithLabelValues()" call.
type CachedMetric struct {
	m sync.Mutex

	metric  cachableMetric
	records []vectorRecord
}

func newCachedMetric(metric cachableMetric) *CachedMetric {
	return &CachedMetric{
		metric: metric,
	}
}

func (v *CachedMetric) Describe(desc chan<- *prometheus.Desc) {
	v.metric.Describe(desc)
}

func (v *CachedMetric) Collect(ch chan<- prometheus.Metric) {
	v.m.Lock()
	defer v.m.Unlock()

	v.metric.Collect(ch)
}

func (v *CachedMetric) WithLabelValues(operation VectorOperation, value float64, labelValues ...string) {
	switch operation {
	case VectorOperationAdd, VectorOperationSet:
	default:
		panic("unsupported vector operation")
	}

	v.m.Lock()
	defer v.m.Unlock()

	v.records = append(v.records, vectorRecord{
		operation:   operation,
		value:       value,
		labelValues: labelValues,
	})
}

// Commit will set the internal value as the cached value to return from "Collect()".
// The internal metric value is completely reset, so the caller should expect
// the gauge to be empty for the next 'WithLabelValues' values.
func (v *CachedMetric) Commit() {
	v.m.Lock()
	defer v.m.Unlock()

	v.metric.Reset()
	v.metric.Process(v.records)

	v.records = nil
}

type CachedHistogramVec struct {
}

// CachedGaugeVec is a gauge instance of a cached metric.
type cachedGaugeVec struct {
	*prometheus.GaugeVec
}

func NewCachedGaugeVec(gaugeVec *prometheus.GaugeVec) *CachedMetric {
	return newCachedMetric(&cachedGaugeVec{
		GaugeVec: gaugeVec,
	})
}

func (v *cachedGaugeVec) Process(records []vectorRecord) {
	for _, record := range records {
		g := v.GaugeVec.WithLabelValues(record.labelValues...)
		switch record.operation {
		case VectorOperationAdd:
			g.Add(record.value)
		case VectorOperationSet:
			g.Set(record.value)
		default:
			// ignore unsupported vectors.
		}
	}
}

type cachedHistogramVec struct {
	*prometheus.HistogramVec
}

func NewCachedHistogramVec(gaugeVec *prometheus.HistogramVec) *CachedMetric {
	return newCachedMetric(&cachedHistogramVec{
		HistogramVec: gaugeVec,
	})
}

func (v *cachedHistogramVec) Process(records []vectorRecord) {
	for _, record := range records {
		g := v.HistogramVec.WithLabelValues(record.labelValues...)
		switch record.operation {
		case VectorOperationObserve:
			g.Observe(record.value)
		default:
			// ignore unsupported vectors.
		}
	}
}

type VectorOperation int

const (
	VectorOperationAdd VectorOperation = iota
	VectorOperationSet
	VectorOperationObserve
)

type vectorRecord struct {
	operation   VectorOperation
	value       float64
	labelValues []string
}
