package keypool

import (
	"github.com/prometheus/client_golang/prometheus"
)

// stateCollector reports the number of keys currently in each state per
// provider. State is read at scrape time rather than tracked via events
// because key recovery (cooldown expiry) happens lazily and is not observable
// as an event.
type stateCollector struct {
	// pools returns the pools to report on. It is called on every scrape so
	// reloaded pools are reflected.
	pools func() []*Pool
	desc  *prometheus.Desc
}

// NewStateCollector returns a collector reporting the number of keys in
// each state, per provider.
func NewStateCollector(pools func() []*Pool) prometheus.Collector {
	return &stateCollector{
		pools: pools,
		desc: prometheus.NewDesc(
			"key_pool_state",
			"The number of keys currently in each state (state: valid, temporary, permanent).",
			[]string{"provider", "state"},
			nil,
		),
	}
}

func (c *stateCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *stateCollector) Collect(ch chan<- prometheus.Metric) {
	for _, pool := range c.pools() {
		if pool == nil {
			continue
		}

		counts := map[KeyState]int{
			KeyStateValid:     0,
			KeyStateTemporary: 0,
			KeyStatePermanent: 0,
		}
		for _, state := range pool.PoolState() {
			counts[state]++
		}

		for _, state := range []KeyState{KeyStateValid, KeyStateTemporary, KeyStatePermanent} {
			ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(counts[state]), pool.providerName, string(state))
		}
	}
}
