package prometheusmetrics

import "github.com/prometheus/client_golang/prometheus"

// metricAliasRegisterer exposes each collector under multiple prefixes.
type metricAliasRegisterer struct {
	registerers []prometheus.Registerer
}

// NewMetricAliasRegisterer exposes collectors under canonicalPrefix and each
// alias prefix. Every exported name reads from the same collector. Alias
// prefixes are typically deprecated names scheduled for removal; see each
// call site for the specific deprecation ticket.
func NewMetricAliasRegisterer(base prometheus.Registerer, canonicalPrefix string, aliasPrefixes ...string) prometheus.Registerer {
	prefixes := append([]string{canonicalPrefix}, aliasPrefixes...)
	registerers := make([]prometheus.Registerer, 0, len(prefixes))
	for _, prefix := range prefixes {
		registerers = append(registerers, prometheus.WrapRegistererWithPrefix(prefix, base))
	}
	return &metricAliasRegisterer{registerers: registerers}
}

// Register registers c under each prefix and rolls back on failure.
func (m *metricAliasRegisterer) Register(c prometheus.Collector) error {
	for i, registerer := range m.registerers {
		if err := registerer.Register(c); err != nil {
			for _, registered := range m.registerers[:i] {
				registered.Unregister(c)
			}
			return err
		}
	}
	return nil
}

// MustRegister registers collectors and panics on the first failure.
func (m *metricAliasRegisterer) MustRegister(cs ...prometheus.Collector) {
	for _, c := range cs {
		if err := m.Register(c); err != nil {
			panic(err)
		}
	}
}

// Unregister removes c from every prefix.
func (m *metricAliasRegisterer) Unregister(c prometheus.Collector) bool {
	ok := true
	for _, registerer := range m.registerers {
		ok = registerer.Unregister(c) && ok
	}
	return ok
}
