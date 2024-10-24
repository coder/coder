package promhelp

import (
	"context"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	ptestutil "github.com/prometheus/client_golang/prometheus/testutil"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

// RegistryDump returns the http page for a given registry's metrics.
// Very useful for visual debugging.
func RegistryDump(reg *prometheus.Registry) string {
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	rec := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	resp := rec.Result()
	data, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return string(data)
}

// Compare can be used to compare a registry to some prometheus formatted
// text. If any values differ, an error is returned.
// If metric names are passed in, only those metrics will be compared.
// Usage: `Compare(reg, RegistryDump(reg))`
func Compare(reg prometheus.Gatherer, compare string, metricNames ...string) error {
	return ptestutil.GatherAndCompare(reg, strings.NewReader(compare), metricNames...)
}

// HistogramValue returns the value of a histogram metric with the given name and labels.
func HistogramValue(t testing.TB, reg prometheus.Gatherer, metricName string, labels prometheus.Labels) *io_prometheus_client.Histogram {
	t.Helper()

	labeled := MetricValue(t, reg, metricName, labels)
	require.NotNilf(t, labeled, "metric %q with labels %v not found", metricName, labels)
	return labeled.GetHistogram()
}

// GaugeValue returns the value of a gauge metric with the given name and labels.
func GaugeValue(t testing.TB, reg prometheus.Gatherer, metricName string, labels prometheus.Labels) int {
	t.Helper()

	labeled := MetricValue(t, reg, metricName, labels)
	require.NotNilf(t, labeled, "metric %q with labels %v not found", metricName, labels)
	return int(labeled.GetGauge().GetValue())
}

// CounterValue returns the value of a counter metric with the given name and labels.
func CounterValue(t testing.TB, reg prometheus.Gatherer, metricName string, labels prometheus.Labels) int {
	t.Helper()

	labeled := MetricValue(t, reg, metricName, labels)
	require.NotNilf(t, labeled, "metric %q with labels %v not found", metricName, labels)
	return int(labeled.GetCounter().GetValue())
}

func MetricValue(t testing.TB, reg prometheus.Gatherer, metricName string, labels prometheus.Labels) *io_prometheus_client.Metric {
	t.Helper()

	metrics, err := reg.Gather()
	require.NoError(t, err)

	for _, m := range metrics {
		if m.GetName() == metricName {
			for _, labeled := range m.GetMetric() {
				mLabels := make(prometheus.Labels)
				for _, v := range labeled.GetLabel() {
					mLabels[v.GetName()] = v.GetValue()
				}
				if maps.Equal(mLabels, labels) {
					return labeled
				}
			}
		}
	}
	return nil
}
