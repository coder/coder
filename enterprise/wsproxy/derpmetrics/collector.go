package derpmetrics

import (
	"expvar"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"tailscale.com/derp"
)

// NewCollector returns a prometheus.Collector that bridges the
// derp.Server's expvar-based stats into Prometheus metrics.
func NewCollector(server *derp.Server) prometheus.Collector {
	return &collector{server: server}
}

const (
	namespace = "coder"
	subsystem = "wsproxy_derp"
)

// Simple counter metrics keyed by their expvar name.
var counterMetrics = map[string]*prometheus.Desc{
	"accepts":                          desc("accepts_total", "Total number of accepted connections."),
	"bytes_received":                   desc("bytes_received_total", "Total bytes received."),
	"bytes_sent":                       desc("bytes_sent_total", "Total bytes sent."),
	"packets_sent":                     desc("packets_sent_total", "Total packets sent."),
	"packets_received":                 desc("packets_received_total", "Total packets received."),
	"packets_dropped":                  desc("packets_dropped_total_unlabeled", "Total packets dropped (unlabeled aggregate)."),
	"packets_forwarded_out":            desc("packets_forwarded_out_total", "Total packets forwarded out."),
	"packets_forwarded_in":             desc("packets_forwarded_in_total", "Total packets forwarded in."),
	"home_moves_in":                    desc("home_moves_in_total", "Total home moves in."),
	"home_moves_out":                   desc("home_moves_out_total", "Total home moves out."),
	"got_ping":                         desc("got_ping_total", "Total pings received."),
	"sent_pong":                        desc("sent_pong_total", "Total pongs sent."),
	"unknown_frames":                   desc("unknown_frames_total", "Total unknown frames received."),
	"peer_gone_disconnected_frames":    desc("peer_gone_disconnected_frames_total", "Total peer-gone-disconnected frames sent."),
	"peer_gone_not_here_frames":        desc("peer_gone_not_here_frames_total", "Total peer-gone-not-here frames sent."),
	"multiforwarder_created":           desc("multiforwarder_created_total", "Total multiforwarders created."),
	"multiforwarder_deleted":           desc("multiforwarder_deleted_total", "Total multiforwarders deleted."),
	"packet_forwarder_delete_other_value": desc("packet_forwarder_delete_other_value_total", "Total packet forwarder delete-other-value events."),
	"counter_total_dup_client_conns":   desc("duplicate_client_conns_total", "Total duplicate client connections."),
}

// Simple gauge metrics keyed by their expvar name.
var gaugeMetrics = map[string]*prometheus.Desc{
	"gauge_current_connections":      desc("current_connections", "Current number of connections."),
	"gauge_current_home_connections": desc("current_home_connections", "Current number of home connections."),
	"gauge_watchers":                 desc("watchers", "Current number of watchers."),
	"gauge_current_file_descriptors": desc("current_file_descriptors", "Current number of file descriptors."),
	"gauge_clients_total":            desc("clients_total", "Current total number of clients."),
	"gauge_clients_local":            desc("clients_local", "Current number of local clients."),
	"gauge_clients_remote":           desc("clients_remote", "Current number of remote clients."),
	"gauge_current_dup_client_keys":  desc("current_duplicate_client_keys", "Current number of duplicate client keys."),
	"gauge_current_dup_client_conns": desc("current_duplicate_client_conns", "Current number of duplicate client connections."),
}

// Labeled counter metrics (nested metrics.Set) with their label name.
var labeledCounterMetrics = map[string]struct {
	desc      *prometheus.Desc
	labelName string
}{
	"counter_packets_dropped_reason": {
		desc:      prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, "packets_dropped_total"), "Total packets dropped by reason.", []string{"reason"}, nil),
		labelName: "reason",
	},
	"counter_packets_dropped_type": {
		desc:      prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, "packets_dropped_by_type_total"), "Total packets dropped by type.", []string{"type"}, nil),
		labelName: "type",
	},
	"counter_packets_received_kind": {
		desc:      prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, "packets_received_by_kind_total"), "Total packets received by kind.", []string{"kind"}, nil),
		labelName: "kind",
	},
	"counter_tcp_rtt": {
		desc:      prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, "tcp_rtt"), "TCP RTT measurements.", []string{"bucket"}, nil),
		labelName: "bucket",
	},
}

var avgQueueDurationDesc = desc("average_queue_duration_seconds", "Average queue duration in seconds.")

func desc(name, help string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, name),
		help, nil, nil,
	)
}

type collector struct {
	server *derp.Server
}

var _ prometheus.Collector = (*collector)(nil)

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range counterMetrics {
		ch <- d
	}
	for _, d := range gaugeMetrics {
		ch <- d
	}
	for _, m := range labeledCounterMetrics {
		ch <- m.desc
	}
	ch <- avgQueueDurationDesc
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	statsVar := c.server.ExpVar()

	// The returned expvar.Var is a *metrics.Set which supports Do().
	type doer interface {
		Do(func(expvar.KeyValue))
	}
	d, ok := statsVar.(doer)
	if !ok {
		return
	}

	d.Do(func(kv expvar.KeyValue) {
		// Counter metrics.
		if desc, ok := counterMetrics[kv.Key]; ok {
			if v, err := strconv.ParseFloat(kv.Value.String(), 64); err == nil {
				ch <- prometheus.MustNewConstMetric(desc, prometheus.CounterValue, v)
			}
			return
		}

		// Gauge metrics.
		if desc, ok := gaugeMetrics[kv.Key]; ok {
			if v, err := strconv.ParseFloat(kv.Value.String(), 64); err == nil {
				ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
			}
			return
		}

		// Labeled counter metrics (nested metrics.Set).
		if lm, ok := labeledCounterMetrics[kv.Key]; ok {
			if nested, ok := kv.Value.(doer); ok {
				nested.Do(func(sub expvar.KeyValue) {
					if v, err := strconv.ParseFloat(sub.Value.String(), 64); err == nil {
						ch <- prometheus.MustNewConstMetric(lm.desc, prometheus.CounterValue, v, sub.Key)
					}
				})
			}
			return
		}

		// Average queue duration: convert ms → seconds.
		if kv.Key == "average_queue_duration_ms" {
			s := kv.Value.String()
			// expvar.Func may return a quoted string or a number.
			s = strings.Trim(s, "\"")
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				ch <- prometheus.MustNewConstMetric(avgQueueDurationDesc, prometheus.GaugeValue, v/1000.0)
			}
			return
		}
	})
}
