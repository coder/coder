package prometheusmetrics

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentmetrics"
)

func TestFilterAcceptableAgentLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "template label is ignored",
			input:    []string{agentmetrics.LabelTemplateName},
			expected: []string{},
		},
		{
			name:     "all other labels are returned",
			input:    agentmetrics.LabelAll,
			expected: []string{agentmetrics.LabelAgentName, agentmetrics.LabelUsername, agentmetrics.LabelWorkspaceName},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.expected, filterAcceptableAgentLabels(tc.input))
		})
	}
}

func benchAsPrometheus(b *testing.B, base []string, extraN int) {
	am := annotatedMetric{
		Stats_Metric: &agentproto.Stats_Metric{
			Name:   "blink_test_metric",
			Type:   agentproto.Stats_Metric_GAUGE,
			Value:  1,
			Labels: make([]*agentproto.Stats_Metric_Label, extraN),
		},
		username:          "user",
		workspaceName:     "ws",
		agentName:         "agent",
		templateName:      "tmpl",
		aggregateByLabels: base,
	}
	for i := 0; i < extraN; i++ {
		am.Labels[i] = &agentproto.Stats_Metric_Label{Name: fmt.Sprintf("l%d", i), Value: "v"}
	}

	ma := &MetricsAggregator{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ma.asPrometheus(&am)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_asPrometheus(b *testing.B) {
	cases := []struct {
		name   string
		base   []string
		extraN int
	}{
		{"base4_extra0", defaultAgentMetricsLabels, 0},
		{"base4_extra2", defaultAgentMetricsLabels, 2},
		{"base4_extra5", defaultAgentMetricsLabels, 5},
		{"base4_extra10", defaultAgentMetricsLabels, 10},
		{"base2_extra5", []string{agentmetrics.LabelUsername, agentmetrics.LabelWorkspaceName}, 5},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			benchAsPrometheus(b, tc.base, tc.extraN)
		})
	}
}
