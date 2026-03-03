package tailnet

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// NewDERPExpvarCollector returns a prometheus.Collector that exposes
// the DERP server's expvar stats as Prometheus metrics. The "derp"
// expvar key must already be published via expvar.Publish before
// Collect is called.
func NewDERPExpvarCollector() prometheus.Collector {
	return collectors.NewExpvarCollector(map[string]*prometheus.Desc{
		"derp": prometheus.NewDesc(
			"coder_derp",
			"DERP server metrics scraped from expvar.",
			[]string{"metric"}, nil,
		),
	})
}
