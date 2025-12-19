package prometheusmetrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentmetrics"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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

	require.Eventually(t, func() bool {
		ma.cleanupDescCache()
		return len(ma.descCache) == 0
	}, testutil.WaitShort, testutil.IntervalFast)
}

// TestDescCacheTimestampUpdate ensures that the timestamp update in getOrCreateDesc
// updates the map entry because d is a copy, not a pointer.
func TestDescCacheTimestampUpdate(t *testing.T) {
	t.Parallel()

	mClock := quartz.NewMock(t)
	registry := prometheus.NewRegistry()
	ma, err := NewMetricsAggregator(slogtest.Make(t, nil), registry, time.Hour, nil, WithClock(mClock))
	require.NoError(t, err)

	baseLabelNames := []string{"label1", "label2"}
	extraLabels := []*agentproto.Stats_Metric_Label{
		{Name: "extra1", Value: "value1"},
	}

	desc1 := ma.getOrCreateDesc("test_metric", "help text", baseLabelNames, extraLabels)
	require.NotNil(t, desc1)

	key := cacheKeyForDesc("test_metric", baseLabelNames, extraLabels)
	initialEntry := ma.descCache[key]
	initialTime := initialEntry.lastUsed

	// Advance the mock clock to ensure a different timestamp
	mClock.Advance(time.Second)

	desc2 := ma.getOrCreateDesc("test_metric", "help text", baseLabelNames, extraLabels)
	require.NotNil(t, desc2)

	updatedEntry := ma.descCache[key]
	updatedTime := updatedEntry.lastUsed

	require.NotEqual(t, initialTime, updatedTime,
		"Timestamp was NOT updated in map when accessing a metric description that should be cached")
}
