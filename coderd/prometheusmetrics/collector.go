package prometheusmetrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// CachedGaugeVec is a wrapper for the prometheus.GaugeVec which allows
// for staging changes in the metrics vector. Calling "WithLabelValues(...)"
// will update the internal gauge value, but it will not be returned by
// "Collect(...)" until the "Commit()" method is called. The "Commit()" method
// resets the internal gauge and applies all staged changes to it.
//
// The Use of CachedGaugeVec is recommended for use cases when there is a risk
// that the Prometheus collector receives incomplete metrics, collected
// in the middle of metrics recalculation, between "Reset()" and the last
// "WithLabelValues()" call.
type CachedGaugeVec struct {
	m sync.Mutex

	gaugeVec *prometheus.GaugeVec
	records  []vectorRecord
}

var _ prometheus.Collector = new(CachedGaugeVec)

type VectorOperation int

const (
	VectorOperationAdd VectorOperation = iota
	VectorOperationSet
)

type vectorRecord struct {
	operation   VectorOperation
	value       float64
	labelValues []string
}

func NewCachedGaugeVec(gaugeVec *prometheus.GaugeVec) *CachedGaugeVec {
	return &CachedGaugeVec{
		gaugeVec: gaugeVec,
	}
}

func (v *CachedGaugeVec) Describe(desc chan<- *prometheus.Desc) {
	v.gaugeVec.Describe(desc)
}

func (v *CachedGaugeVec) Collect(ch chan<- prometheus.Metric) {
	v.m.Lock()
	defer v.m.Unlock()

	v.gaugeVec.Collect(ch)
}

func (v *CachedGaugeVec) WithLabelValues(operation VectorOperation, value float64, labelValues ...string) {
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
func (v *CachedGaugeVec) Commit() {
	v.m.Lock()
	defer v.m.Unlock()

	v.gaugeVec.Reset()
	for _, record := range v.records {
		g := v.gaugeVec.WithLabelValues(record.labelValues...)
		switch record.operation {
		case VectorOperationAdd:
			g.Add(record.value)
		case VectorOperationSet:
			g.Set(record.value)
		}
	}

	v.records = nil
}
