package keypool_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/keypool"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// newPool builds a pool named name with the given number of valid, temporary,
// and permanent keys.
func newPool(t *testing.T, clk quartz.Clock, name string, valid, temporary, permanent int) *keypool.Pool {
	t.Helper()
	keys := make([]string, valid+temporary+permanent)
	for i := range keys {
		keys[i] = fmt.Sprintf("%s-key-%d", name, i)
	}
	pool, err := keypool.New(name, keys, clk, nil)
	require.NoError(t, err)

	walker := pool.Walker()
	for range temporary {
		key, kpErr := walker.Next()
		require.Nil(t, kpErr)
		key.MarkTemporary(time.Minute)
	}
	for range permanent {
		key, kpErr := walker.Next()
		require.Nil(t, kpErr)
		key.MarkPermanent()
	}
	return pool
}

func TestStateCollector(t *testing.T) {
	t.Parallel()

	type stateCount struct {
		provider string
		state    string
		count    int
	}
	tests := []struct {
		name                string
		pools               func(t *testing.T, clk quartz.Clock) []*keypool.Pool
		expectedStateCounts []stateCount
	}{
		{
			name:                "no_pools",
			pools:               func(*testing.T, quartz.Clock) []*keypool.Pool { return nil },
			expectedStateCounts: nil,
		},
		{
			name: "single_provider_mixed_states",
			pools: func(t *testing.T, clk quartz.Clock) []*keypool.Pool {
				return []*keypool.Pool{newPool(t, clk, "anthropic", 2, 1, 1)}
			},
			expectedStateCounts: []stateCount{
				{"anthropic", "valid", 2},
				{"anthropic", "temporary", 1},
				{"anthropic", "permanent", 1},
			},
		},
		{
			name: "multiple_providers_nil_skipped",
			pools: func(t *testing.T, clk quartz.Clock) []*keypool.Pool {
				return []*keypool.Pool{
					newPool(t, clk, "anthropic", 2, 1, 0),
					nil,
					newPool(t, clk, "openai", 1, 0, 1),
				}
			},
			expectedStateCounts: []stateCount{
				{"anthropic", "valid", 2},
				{"anthropic", "temporary", 1},
				{"anthropic", "permanent", 0},
				{"openai", "valid", 1},
				{"openai", "temporary", 0},
				{"openai", "permanent", 1},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := quartz.NewMock(t)
			pools := tc.pools(t, clk)

			collector := keypool.NewStateCollector(func() []*keypool.Pool { return pools })
			reg := prometheus.NewRegistry()
			require.NoError(t, reg.Register(collector))

			if len(tc.expectedStateCounts) == 0 {
				require.Equal(t, 0, promtest.CollectAndCount(collector), "no key_pool_state series expected for empty pool list")
			}

			gathered, err := reg.Gather()
			require.NoError(t, err)
			for _, s := range tc.expectedStateCounts {
				assert.True(t, codertestutil.PromGaugeHasValue(t, gathered, float64(s.count),
					"key_pool_state", s.provider, s.state))
			}
		})
	}
}
