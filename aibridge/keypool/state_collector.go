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

		var valid, temporary, permanent int
		for _, state := range pool.PoolState() {
			switch state {
			case KeyStateValid:
				valid++
			case KeyStateTemporary:
				temporary++
			case KeyStatePermanent:
				permanent++
			}
		}

		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(valid), pool.providerName, "valid")
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(temporary), pool.providerName, "temporary")
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(permanent), pool.providerName, "permanent")
	}
}
