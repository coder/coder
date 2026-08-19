package main

import (
	"bytes"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestParseSection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		value   string
		want    documentSection
		wantErr string
	}{
		{
			name:  "prefix",
			value: "common=prefix:coder_ai_gateway_,!coder_ai_gateway_proxy_",
			want: documentSection{
				name: "common",
				filter: metricFilter{
					prefixes:        []string{"coder_ai_gateway_"},
					excludePrefixes: []string{"coder_ai_gateway_proxy_"},
				},
			},
		},
		{
			name:  "names",
			value: "reload=names:one,two",
			want: documentSection{
				name: "reload",
				filter: metricFilter{
					names: map[string]struct{}{"one": {}, "two": {}},
				},
			},
		},
		{
			name:    "invalid name",
			value:   "bad name=prefix:metric_",
			wantErr: "may contain only",
		},
		{
			name:    "invalid kind",
			value:   "common=contains:metric",
			wantErr: "unknown filter kind",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseSection(testCase.value)
			if testCase.wantErr != "" {
				require.ErrorContains(t, err, testCase.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.want, got)
		})
	}
}

func TestFilterMetricFamilies(t *testing.T) {
	t.Parallel()

	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_cost_control_blocked_users", dto.MetricType_GAUGE, "Blocked users.", "group_id"),
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider"),
		metricFamily("coder_ai_gateway_proxy_mitm_requests_total", dto.MetricType_COUNTER, "Proxy requests.", "provider"),
	}

	byPrefix := filterMetricFamilies(metrics, metricFilter{prefixes: []string{"coder_ai_gateway_proxy_"}})
	require.Equal(t, []string{"coder_ai_gateway_proxy_mitm_requests_total"}, metricNames(byPrefix))

	byExcludedPrefix := filterMetricFamilies(metrics, metricFilter{
		prefixes: []string{"coder_ai_gateway_"},
		excludePrefixes: []string{
			"coder_ai_gateway_cost_control_",
			"coder_ai_gateway_proxy_",
		},
	})
	require.Equal(t, []string{"coder_ai_gateway_interceptions_total"}, metricNames(byExcludedPrefix))

	byNames := filterMetricFamilies(metrics, metricFilter{names: map[string]struct{}{
		"coder_ai_gateway_cost_control_blocked_users": {},
		"coder_ai_gateway_interceptions_total":        {},
	}})
	require.Equal(t, []string{
		"coder_ai_gateway_cost_control_blocked_users",
		"coder_ai_gateway_interceptions_total",
	}, metricNames(byNames))
}

func TestUpdateDocumentSections(t *testing.T) {
	t.Parallel()

	commonPrefix := string(namedGeneratorPrefix("common"))
	commonSuffix := string(namedGeneratorSuffix("common"))
	proxyPrefix := string(namedGeneratorPrefix("proxy"))
	proxySuffix := string(namedGeneratorSuffix("proxy"))
	doc := []byte("before\n" + commonPrefix + "\nstale common\n" + commonSuffix + "\nmiddle\n" + proxyPrefix + "\nstale proxy\n" + proxySuffix + "\nafter\n")
	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider", "model"),
		metricFamily("coder_ai_gateway_proxy_mitm_requests_total", dto.MetricType_COUNTER, "Proxy requests.", "provider"),
	}
	sections := []documentSection{
		{name: "common", filter: metricFilter{names: map[string]struct{}{"coder_ai_gateway_interceptions_total": {}}}},
		{name: "proxy", filter: metricFilter{prefixes: []string{"coder_ai_gateway_proxy_"}}},
	}

	updated, err := updateDocumentSections(doc, metrics, sections)
	require.NoError(t, err)
	require.NotContains(t, string(updated), "stale common")
	require.NotContains(t, string(updated), "stale proxy")
	require.Contains(t, string(updated), "| `coder_ai_gateway_interceptions_total` | counter | Intercepted requests. | `model` `provider` |")
	require.Contains(t, string(updated), "| `coder_ai_gateway_proxy_mitm_requests_total` | counter | Proxy requests. | `provider` |")

	updatedAgain, err := updateDocumentSections(updated, metrics, sections)
	require.NoError(t, err)
	require.Equal(t, updated, updatedAgain)
}

func TestUpdatePrometheusDocPreservesDefaultMarkers(t *testing.T) {
	t.Parallel()

	doc := []byte("before\n" + string(generatorPrefix) + "\nstale\n" + string(generatorSuffix) + "\nafter\n")
	metrics := []*dto.MetricFamily{
		metricFamily("z_metric", dto.MetricType_GAUGE, "Last.", "z", "a"),
		metricFamily("a_metric", dto.MetricType_COUNTER, "First."),
	}

	updated, err := updatePrometheusDoc(doc, metrics)
	require.NoError(t, err)
	require.Contains(t, string(updated), string(generatorPrefix))
	require.Contains(t, string(updated), string(generatorSuffix))
	require.Less(t, bytes.Index(updated, []byte("z_metric")), bytes.Index(updated, generatorSuffix))
	require.Contains(t, string(updated), "| `z_metric` | gauge | Last. | `a` `z` |")
}

func metricFamily(name string, metricType dto.MetricType, help string, labels ...string) *dto.MetricFamily {
	metric := &dto.Metric{}
	for _, label := range labels {
		labelName := label
		labelValue := ""
		metric.Label = append(metric.Label, &dto.LabelPair{Name: &labelName, Value: &labelValue})
	}
	return &dto.MetricFamily{
		Name:   &name,
		Type:   &metricType,
		Help:   &help,
		Metric: []*dto.Metric{metric},
	}
}

func metricNames(metrics []*dto.MetricFamily) []string {
	names := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		names = append(names, metric.GetName())
	}
	return names
}
