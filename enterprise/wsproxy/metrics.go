package wsproxy

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// NewDERPMetricsCollector returns a prometheus.Collector that
// exposes the DERP server's expvar stats. The "derp" expvar key
// must already be published via expvar.Publish before calling
// Collect on the returned collector.
func NewDERPMetricsCollector() prometheus.Collector {
	return collectors.NewExpvarCollector(map[string]*prometheus.Desc{
		"derp": prometheus.NewDesc(
			"coder_wsproxy_derp",
			"Workspace proxy DERP server metrics scraped from expvar.",
			[]string{"metric"}, nil,
		),
	})
}
