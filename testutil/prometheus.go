package testutil

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

type kind string

const (
	counterKind kind = "counter"
	gaugeKind   kind = "gauge"
)

func PromGaugeHasValue(t testing.TB, metrics []*dto.MetricFamily, value float64, name string, labels ...string) bool {
	t.Helper()
	return value == getValue(t, metrics, gaugeKind, name, labels...)
}

func PromCounterHasValue(t testing.TB, metrics []*dto.MetricFamily, value float64, name string, labels ...string) bool {
	t.Helper()
	return value == getValue(t, metrics, counterKind, name, labels...)
}

func PromGaugeAssertion(t testing.TB, metrics []*dto.MetricFamily, assert func(in float64) bool, name string, labels ...string) bool {
	t.Helper()
	return assert(getValue(t, metrics, gaugeKind, name, labels...))
}

func PromCounterAssertion(t testing.TB, metrics []*dto.MetricFamily, assert func(in float64) bool, name string, labels ...string) bool {
	t.Helper()
	return assert(getValue(t, metrics, counterKind, name, labels...))
}

func PromCounterGathered(t testing.TB, metrics []*dto.MetricFamily, name string, labels ...string) bool {
	t.Helper()
	return getMetric(t, metrics, name, labels...) != nil
}

func PromGaugeGathered(t testing.TB, metrics []*dto.MetricFamily, name string, labels ...string) bool {
	t.Helper()
	return getMetric(t, metrics, name, labels...) != nil
}

func getValue(t testing.TB, metrics []*dto.MetricFamily, kind kind, name string, labels ...string) float64 {
	m := getMetric(t, metrics, name, labels...)
	if m == nil {
		return -1
	}

	switch kind {
	case counterKind:
		return m.GetCounter().GetValue()
	case gaugeKind:
		return m.GetGauge().GetValue()
	default:
		return -1
	}
}

func getMetric(t testing.TB, metrics []*dto.MetricFamily, name string, labels ...string) *dto.Metric {
	for _, family := range metrics {
		if family.GetName() != name {
			continue
		}
		ms := family.GetMetric()
	metricsLoop:
		for _, m := range ms {
			require.Equal(t, len(labels), len(m.GetLabel()))
			for i, lv := range labels {
				if lv != m.GetLabel()[i].GetValue() {
					continue metricsLoop
				}
			}

			return m
		}
	}

	return nil
}
