package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/metrics"
)

const (
	testProvider    = "openai"
	testModel       = "gpt-test"
	testInitiatorID = "init-test"
	testClient      = "client-test"
)

// TestAddTokenCount_NegativeRejected guards the bug class fixed in PR #26547:
// a provider violating its own invariant must not panic the counter.
func TestAddTokenCount_NegativeRejected(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	counter := m.TokenUseCount.WithLabelValues(testProvider, testModel, "input", testInitiatorID, testClient)

	require.NoError(t, m.AddTokenCount(testProvider, testModel, "input", testInitiatorID, testClient, 10))
	require.Equal(t, 10.0, testutil.ToFloat64(counter))

	err := m.AddTokenCount(testProvider, testModel, "input", testInitiatorID, testClient, -5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "negative token count")

	assert.Equal(t, 10.0, testutil.ToFloat64(counter))
}

func TestAddTokenCount_PositiveIncrements(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	err := m.AddTokenCount(testProvider, testModel, "output", testInitiatorID, testClient, 42)
	require.NoError(t, err)

	counter := m.TokenUseCount.WithLabelValues(testProvider, testModel, "output", testInitiatorID, testClient)
	assert.Equal(t, 42.0, testutil.ToFloat64(counter))
}

// TestAddTokenCount_ZeroIsAllowed documents that zero is not a violation.
func TestAddTokenCount_ZeroIsAllowed(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	err := m.AddTokenCount(testProvider, testModel, "input", testInitiatorID, testClient, 0)
	require.NoError(t, err)
}
