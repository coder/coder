package derpmetrics

import (
	"expvar"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"tailscale.com/derp"
)

// DERPExpvarCollector exports a DERP server's expvar stats as
// properly typed Prometheus metrics.
type DERPExpvarCollector struct {
	server *derp.Server

	// Counters.
	accepts              *prometheus.Desc
	bytesReceived        *prometheus.Desc
	bytesSent            *prometheus.Desc
	packetsReceived      *prometheus.Desc
	packetsSent          *prometheus.Desc
	packetsDropped       *prometheus.Desc
	packetsForwardedIn   *prometheus.Desc
	packetsForwardedOut  *prometheus.Desc
	homeMovesIn          *prometheus.Desc
	homeMovesOut         *prometheus.Desc
	gotPing              *prometheus.Desc
	sentPong             *prometheus.Desc
	peerGoneDisconnected *prometheus.Desc
	peerGoneNotHere      *prometheus.Desc
	unknownFrames        *prometheus.Desc

	// Labeled counters.
	packetsDroppedByReason *prometheus.Desc
	packetsDroppedByType   *prometheus.Desc
	packetsReceivedByKind  *prometheus.Desc

	// Gauges.
	connections     *prometheus.Desc
	homeConnections *prometheus.Desc
	clientsTotal    *prometheus.Desc
	clientsLocal    *prometheus.Desc
	clientsRemote   *prometheus.Desc
	watchers        *prometheus.Desc
	avgQueueDurMS   *prometheus.Desc
}

// NewDERPExpvarCollector creates a Prometheus collector that reads
// stats from a DERP server's expvar on each scrape.
func NewDERPExpvarCollector(server *derp.Server) *DERPExpvarCollector {
	return &DERPExpvarCollector{
		server: server,

		accepts:              prometheus.NewDesc("coder_derp_server_accepts_total", "Total DERP connections accepted.", nil, nil),
		bytesReceived:        prometheus.NewDesc("coder_derp_server_bytes_received_total", "Total bytes received.", nil, nil),
		bytesSent:            prometheus.NewDesc("coder_derp_server_bytes_sent_total", "Total bytes sent.", nil, nil),
		packetsReceived:      prometheus.NewDesc("coder_derp_server_packets_received_total", "Total packets received.", nil, nil),
		packetsSent:          prometheus.NewDesc("coder_derp_server_packets_sent_total", "Total packets sent.", nil, nil),
		packetsDropped:       prometheus.NewDesc("coder_derp_server_packets_dropped_total", "Total packets dropped.", nil, nil),
		packetsForwardedIn:   prometheus.NewDesc("coder_derp_server_packets_forwarded_in_total", "Total packets forwarded in from mesh peers.", nil, nil),
		packetsForwardedOut:  prometheus.NewDesc("coder_derp_server_packets_forwarded_out_total", "Total packets forwarded out to mesh peers.", nil, nil),
		homeMovesIn:          prometheus.NewDesc("coder_derp_server_home_moves_in_total", "Total home moves in.", nil, nil),
		homeMovesOut:         prometheus.NewDesc("coder_derp_server_home_moves_out_total", "Total home moves out.", nil, nil),
		gotPing:              prometheus.NewDesc("coder_derp_server_got_ping_total", "Total pings received.", nil, nil),
		sentPong:             prometheus.NewDesc("coder_derp_server_sent_pong_total", "Total pongs sent.", nil, nil),
		peerGoneDisconnected: prometheus.NewDesc("coder_derp_server_peer_gone_disconnected_total", "Total peer gone (disconnected) frames sent.", nil, nil),
		peerGoneNotHere:      prometheus.NewDesc("coder_derp_server_peer_gone_not_here_total", "Total peer gone (not here) frames sent.", nil, nil),
		unknownFrames:        prometheus.NewDesc("coder_derp_server_unknown_frames_total", "Total unknown frames received.", nil, nil),

		packetsDroppedByReason: prometheus.NewDesc("coder_derp_server_packets_dropped_reason_total", "Packets dropped by reason.", []string{"reason"}, nil),
		packetsDroppedByType:   prometheus.NewDesc("coder_derp_server_packets_dropped_type_total", "Packets dropped by type.", []string{"type"}, nil),
		packetsReceivedByKind:  prometheus.NewDesc("coder_derp_server_packets_received_kind_total", "Packets received by kind.", []string{"kind"}, nil),

		connections:     prometheus.NewDesc("coder_derp_server_connections", "Current DERP connections.", nil, nil),
		homeConnections: prometheus.NewDesc("coder_derp_server_home_connections", "Current home DERP connections.", nil, nil),
		clientsTotal:    prometheus.NewDesc("coder_derp_server_clients", "Total clients (local + remote).", nil, nil),
		clientsLocal:    prometheus.NewDesc("coder_derp_server_clients_local", "Local clients.", nil, nil),
		clientsRemote:   prometheus.NewDesc("coder_derp_server_clients_remote", "Remote (mesh) clients.", nil, nil),
		watchers:        prometheus.NewDesc("coder_derp_server_watchers", "Current watchers.", nil, nil),
		avgQueueDurMS:   prometheus.NewDesc("coder_derp_server_average_queue_duration_ms", "Average queue duration in milliseconds.", nil, nil),
	}
}

func (c *DERPExpvarCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.accepts
	ch <- c.bytesReceived
	ch <- c.bytesSent
	ch <- c.packetsReceived
	ch <- c.packetsSent
	ch <- c.packetsDropped
	ch <- c.packetsForwardedIn
	ch <- c.packetsForwardedOut
	ch <- c.homeMovesIn
	ch <- c.homeMovesOut
	ch <- c.gotPing
	ch <- c.sentPong
	ch <- c.peerGoneDisconnected
	ch <- c.peerGoneNotHere
	ch <- c.unknownFrames
	ch <- c.packetsDroppedByReason
	ch <- c.packetsDroppedByType
	ch <- c.packetsReceivedByKind
	ch <- c.connections
	ch <- c.homeConnections
	ch <- c.clientsTotal
	ch <- c.clientsLocal
	ch <- c.clientsRemote
	ch <- c.watchers
	ch <- c.avgQueueDurMS
}

// Collect reads the DERP server's expvar stats and emits them as
// Prometheus metrics. Called on each /metrics scrape.
func (c *DERPExpvarCollector) Collect(ch chan<- prometheus.Metric) {
	vars, ok := c.server.ExpVar().(interface {
		Do(func(expvar.KeyValue))
	})
	if !ok {
		return
	}

	vars.Do(func(kv expvar.KeyValue) {
		switch kv.Key {
		case "accepts":
			emitCounter(ch, c.accepts, kv.Value)
		case "bytes_received":
			emitCounter(ch, c.bytesReceived, kv.Value)
		case "bytes_sent":
			emitCounter(ch, c.bytesSent, kv.Value)
		case "packets_received":
			emitCounter(ch, c.packetsReceived, kv.Value)
		case "packets_sent":
			emitCounter(ch, c.packetsSent, kv.Value)
		case "packets_dropped":
			emitCounter(ch, c.packetsDropped, kv.Value)
		case "packets_forwarded_in":
			emitCounter(ch, c.packetsForwardedIn, kv.Value)
		case "packets_forwarded_out":
			emitCounter(ch, c.packetsForwardedOut, kv.Value)
		case "home_moves_in":
			emitCounter(ch, c.homeMovesIn, kv.Value)
		case "home_moves_out":
			emitCounter(ch, c.homeMovesOut, kv.Value)
		case "got_ping":
			emitCounter(ch, c.gotPing, kv.Value)
		case "sent_pong":
			emitCounter(ch, c.sentPong, kv.Value)
		case "peer_gone_disconnected_frames":
			emitCounter(ch, c.peerGoneDisconnected, kv.Value)
		case "peer_gone_not_here_frames":
			emitCounter(ch, c.peerGoneNotHere, kv.Value)
		case "unknown_frames":
			emitCounter(ch, c.unknownFrames, kv.Value)

		case "counter_packets_dropped_reason":
			emitLabeledCounters(ch, c.packetsDroppedByReason, kv.Value)
		case "counter_packets_dropped_type":
			emitLabeledCounters(ch, c.packetsDroppedByType, kv.Value)
		case "counter_packets_received_kind":
			emitLabeledCounters(ch, c.packetsReceivedByKind, kv.Value)

		case "gauge_current_connections":
			emitGauge(ch, c.connections, kv.Value)
		case "gauge_current_home_connections":
			emitGauge(ch, c.homeConnections, kv.Value)
		case "gauge_clients_total":
			emitGauge(ch, c.clientsTotal, kv.Value)
		case "gauge_clients_local":
			emitGauge(ch, c.clientsLocal, kv.Value)
		case "gauge_clients_remote":
			emitGauge(ch, c.clientsRemote, kv.Value)
		case "gauge_watchers":
			emitGauge(ch, c.watchers, kv.Value)
		case "average_queue_duration_ms":
			emitGauge(ch, c.avgQueueDurMS, kv.Value)
		}
	})
}

func emitCounter(ch chan<- prometheus.Metric, desc *prometheus.Desc, v expvar.Var) {
	if f, ok := parseExpvarFloat(v); ok {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.CounterValue, f)
	}
}

func emitGauge(ch chan<- prometheus.Metric, desc *prometheus.Desc, v expvar.Var) {
	if f, ok := parseExpvarFloat(v); ok {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, f)
	}
}

func emitLabeledCounters(ch chan<- prometheus.Metric, desc *prometheus.Desc, v expvar.Var) {
	sub, ok := v.(interface{ Do(func(expvar.KeyValue)) })
	if !ok {
		return
	}
	sub.Do(func(kv expvar.KeyValue) {
		if f, ok := parseExpvarFloat(kv.Value); ok {
			ch <- prometheus.MustNewConstMetric(desc, prometheus.CounterValue, f, kv.Key)
		}
	})
}

func parseExpvarFloat(v expvar.Var) (float64, bool) {
	switch val := v.(type) {
	case *expvar.Int:
		return float64(val.Value()), true
	case *expvar.Float:
		return val.Value(), true
	default:
		f, err := strconv.ParseFloat(v.String(), 64)
		return f, err == nil
	}
}
