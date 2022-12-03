package coderd

import "github.com/prometheus/client_golang/prometheus"

type prometheusMetrics struct {
	workspaceTrafficRx *prometheus.CounterVec
	workspaceTrafficTx *prometheus.CounterVec
}

func newPrometheusMetrics(registerer prometheus.Registerer) *prometheusMetrics {
	workspaceTrafficRx := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "wireguard",
			Name:      "workspace_rx_bytes",
			Help:      "Number of received bytes over Wireguard per workspace.",
		}, []string{"workspace_id", "workspace_name"},
	)
	registerer.MustRegister(workspaceTrafficRx)
	workspaceTrafficTx := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "wireguard",
			Name:      "workspace_tx_bytes",
			Help:      "Number of transmitted bytes over Wireguard per workspace.",
		}, []string{"workspace_id", "workspace_name"},
	)
	registerer.MustRegister(workspaceTrafficTx)

	return &prometheusMetrics{
		workspaceTrafficRx: workspaceTrafficRx,
		workspaceTrafficTx: workspaceTrafficTx,
	}
}
