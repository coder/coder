package prometheusmetrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/prometheusmetrics"
)

func TestMetricAliasRegisterer(t *testing.T) {
	t.Parallel()

	t.Run("EmitsCanonicalAndAliases", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			suffix   string
			register func(prometheus.Registerer)
		}{
			{
				name:   "counter_vec",
				suffix: "requests_total",
				register: func(reg prometheus.Registerer) {
					counter := prometheus.NewCounterVec(prometheus.CounterOpts{
						Name: "requests_total",
						Help: "Total requests.",
					}, []string{"route"})
					reg.MustRegister(counter)
					counter.WithLabelValues("/api").Add(3)
				},
			},
			{
				name:   "gauge",
				suffix: "inflight_requests",
				register: func(reg prometheus.Registerer) {
					gauge := prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "inflight_requests",
						Help: "Inflight requests.",
					})
					reg.MustRegister(gauge)
					gauge.Set(7)
				},
			},
			{
				name:   "histogram_vec",
				suffix: "request_duration_seconds",
				register: func(reg prometheus.Registerer) {
					histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
						Name:    "request_duration_seconds",
						Help:    "Request duration.",
						Buckets: []float64{1, 5},
					}, []string{"route"})
					reg.MustRegister(histogram)
					histogram.WithLabelValues("/api").Observe(3)
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				base := prometheus.NewRegistry()
				prefixes := []string{"canonical_", "alias_one_", "alias_two_"}
				reg := prometheusmetrics.NewMetricAliasRegisterer(base, prefixes[0], prefixes[1:]...)

				tc.register(reg)

				families, err := base.Gather()
				require.NoError(t, err)

				canonical := prefixes[0] + tc.suffix
				for _, aliasPrefix := range prefixes[1:] {
					assertParity(t, families, canonical, aliasPrefix+tc.suffix)
				}
				require.Len(t, families, len(prefixes))
			})
		}
	})

	t.Run("RegisterRollsBackPartialFailure", func(t *testing.T) {
		t.Parallel()

		base := prometheus.NewRegistry()
		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "requests_total",
			Help: "Total requests.",
		})
		prometheus.WrapRegistererWithPrefix("alias_two_", base).MustRegister(counter)

		reg := prometheusmetrics.NewMetricAliasRegisterer(base, "canonical_", "alias_one_", "alias_two_")
		err := reg.Register(counter)
		require.Error(t, err)

		families, err := base.Gather()
		require.NoError(t, err)
		require.Len(t, families, 1)
		require.Equal(t, "alias_two_requests_total", families[0].GetName())
	})

	t.Run("Unregister", func(t *testing.T) {
		t.Parallel()

		base := prometheus.NewRegistry()
		reg := prometheusmetrics.NewMetricAliasRegisterer(base, "canonical_", "alias_one_", "alias_two_")

		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "requests_total",
			Help: "Total requests.",
		})
		reg.MustRegister(counter)

		require.True(t, reg.Unregister(counter))

		families, err := base.Gather()
		require.NoError(t, err)
		require.Empty(t, families)
	})
}

func assertParity(t *testing.T, families []*io_prometheus_client.MetricFamily, canonical, alias string) {
	t.Helper()
	canonicalFamily := findMetricFamily(t, families, canonical)
	aliasFamily := findMetricFamily(t, families, alias)
	require.Equal(t, canonicalFamily.GetType(), aliasFamily.GetType())
	require.Equal(t, canonicalFamily.GetHelp(), aliasFamily.GetHelp())
	require.Equal(t, canonicalFamily.GetMetric(), aliasFamily.GetMetric())
}

func findMetricFamily(t *testing.T, families []*io_prometheus_client.MetricFamily, name string) *io_prometheus_client.MetricFamily {
	t.Helper()
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	require.Failf(t, "metric family not found", "missing metric family %q", name)
	return nil
}
