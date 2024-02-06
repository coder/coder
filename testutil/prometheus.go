package testutil

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func PromGaugeHasValue(t testing.TB, metrics []*dto.MetricFamily, value float64, name string, label ...string) bool {
	t.Helper()
	for _, family := range metrics {
		if family.GetName() != name {
			continue
		}
		ms := family.GetMetric()
	metricsLoop:
		for _, m := range ms {
			require.Equal(t, len(label), len(m.GetLabel()))
			for i, lv := range label {
				if lv != m.GetLabel()[i].GetValue() {
					continue metricsLoop
				}
			}
			return value == m.GetGauge().GetValue()
		}
	}
	return false
}

func PromCounterHasValue(t testing.TB, metrics []*dto.MetricFamily, value float64, name string, label ...string) bool {
	t.Helper()
	for _, family := range metrics {
		if family.GetName() != name {
			continue
		}
		ms := family.GetMetric()
	metricsLoop:
		for _, m := range ms {
			require.Equal(t, len(label), len(m.GetLabel()))
			for i, lv := range label {
				if lv != m.GetLabel()[i].GetValue() {
					continue metricsLoop
				}
			}
			return value == m.GetCounter().GetValue()
		}
	}
	return false
}
