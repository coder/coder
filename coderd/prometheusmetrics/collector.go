package prometheusmetrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

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
	v.m.Lock()
	defer v.m.Unlock()

	v.gaugeVec.Describe(desc)
}

func (v *CachedGaugeVec) Collect(ch chan<- prometheus.Metric) {
	v.m.Lock()
	defer v.m.Unlock()

	v.gaugeVec.Collect(ch)
}

func (v *CachedGaugeVec) WithLabelValues(operation VectorOperation, value float64, labelValues ...string) {
	v.m.Lock()
	defer v.m.Unlock()

	v.records = append(v.records, vectorRecord{
		operation:   operation,
		value:       value,
		labelValues: labelValues,
	})
}

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
		default:
			panic("unsupported vector operation")
		}
	}

	v.records = nil
}
