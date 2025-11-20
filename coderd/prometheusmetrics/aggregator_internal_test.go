package prometheusmetrics

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentmetrics"
	"github.com/coder/coder/v2/testutil"
)

func TestDescCache_DescExpire(t *testing.T) {
	const (
		testWorkspaceName = "yogi-workspace"
		testUsername      = "yogi-bear"
		testAgentName     = "main-agent"
		testTemplateName  = "main-template"
	)

	testLabels := AgentMetricLabels{
		Username:      testUsername,
		WorkspaceName: testWorkspaceName,
		AgentName:     testAgentName,
		TemplateName:  testTemplateName,
	}

	t.Parallel()

	// given
	registry := prometheus.NewRegistry()
	ma, err := NewMetricsAggregator(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), registry, time.Millisecond, agentmetrics.LabelAll)
	require.NoError(t, err)

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	closeFunc := ma.Run(ctx)
	t.Cleanup(closeFunc)

	given := []*agentproto.Stats_Metric{
		{Name: "a_counter_one", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
	}

	_, err = ma.asPrometheus(&annotatedMetric{
		given[0],
		testLabels.Username,
		testLabels.WorkspaceName,
		testLabels.AgentName,
		testLabels.TemplateName,
		// the rest doesn't matter for this test
		time.Now(),
		[]string{},
	})
	require.NoError(t, err)

	// when
	// metricsAggregator.Update(ctx, testLabels, given)
	// ma.getOrCreateDec(given[0].Name, "a_counter_one counts some thing", testLabels, nil)

	time.Sleep(time.Millisecond * 10) // Ensure that metric is expired

	// then
	require.Eventually(t, func() bool {
		return len(ma.descCache) == 0
	}, testutil.WaitShort, testutil.IntervalFast)
}
