package keypool

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// newPool builds a pool named name with the given number of valid and temporary keys.
func newPool(t *testing.T, clk quartz.Clock, name string, valid, temporary int) *Pool {
	t.Helper()
	keys := make([]string, valid+temporary)
	for i := range keys {
		keys[i] = fmt.Sprintf("%s-key-%d", name, i)
	}
	pool, err := New(name, keys, clk, nil)
	require.NoError(t, err)

	walker := pool.Walker()
	for range temporary {
		key, kpErr := walker.Next()
		require.Nil(t, kpErr)
		key.markTemporary(time.Minute)
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
		pools               func(t *testing.T, clk quartz.Clock) []*Pool
		expectedStateCounts []stateCount
	}{
		{
			name:                "no_pools",
			pools:               func(*testing.T, quartz.Clock) []*Pool { return nil },
			expectedStateCounts: nil,
		},
		{
			name: "single_provider_mixed_states",
			pools: func(t *testing.T, clk quartz.Clock) []*Pool {
				return []*Pool{newPool(t, clk, "anthropic", 2, 1)}
			},
			expectedStateCounts: []stateCount{
				{"anthropic", "valid", 2},
				{"anthropic", "temporary", 1},
			},
		},
		{
			name: "multiple_providers_nil_skipped",
			pools: func(t *testing.T, clk quartz.Clock) []*Pool {
				return []*Pool{
					newPool(t, clk, "anthropic", 2, 1),
					nil,
					newPool(t, clk, "openai", 1, 0),
				}
			},
			expectedStateCounts: []stateCount{
				{"anthropic", "valid", 2},
				{"anthropic", "temporary", 1},
				{"openai", "valid", 1},
				{"openai", "temporary", 0},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := quartz.NewMock(t)
			pools := tc.pools(t, clk)

			collector := NewStateCollector(func() []*Pool { return pools })
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
