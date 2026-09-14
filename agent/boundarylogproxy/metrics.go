package boundarylogproxy

import "github.com/prometheus/client_golang/prometheus"

// Metrics tracks observability for the boundary -> agent -> coderd audit log
// pipeline.
//
// Audit logs from boundary workspaces pass through several async buffers
// before reaching coderd, and any stage can silently drop data. These
// metrics make that loss visible so operators/devs can:
//
//   - Bubble up data loss: a non-zero drop rate means audit logs are being
//     lost, which may have auditing implications.
//   - Identify the bottleneck: the reason label pinpoints where drops
//     occur: boundary's internal buffers, the agent's channel, or the
//     RPC to coderd.
//   - Tune buffer sizes: sustained "buffer_full" drops indicate the
//     agent's channel (or boundary's batch buffer) is too small for the
//     workload. Combined with batches_forwarded_total you can compute a
//     drop rate: drops / (drops + forwards).
//   - Detect batch forwarding issues: "forward_failed" drops increase when
//     the agent cannot reach coderd.
//
// Drops are captured at two stages:
//   - Agent-side: the agent's channel buffer overflows (reason
//     "buffer_full") or the RPC forward to coderd fails (reason
//     "forward_failed").
//   - Boundary-reported: boundary self-reports drops via BoundaryStatus
//     messages (reasons "boundary_channel_full", "boundary_batch_full").
//     These arrive on the next successful flush from boundary.
//
// There are circumstances where metrics could be lost e.g., agent restarts,
// boundary crashes, or the agent shuts down when the DRPC connection is down.
type Metrics struct {
	batchesDropped   *prometheus.CounterVec
	logsDropped      *prometheus.CounterVec
	batchesForwarded prometheus.Counter
}

func newMetrics(registerer prometheus.Registerer) *Metrics {
	batchesDropped := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "agent",
		Subsystem: "boundary_log_proxy",
		Name:      "batches_dropped_total",
		Help: "Total number of boundary log batches dropped before reaching coderd. " +
			"Reason: buffer_full = the agent's internal buffer is full, meaning boundary is producing logs faster than the agent can forward them to coderd; " +
			"forward_failed = the agent failed to send the batch to coderd, potentially because coderd is unreachable or the connection was interrupted.",
	}, []string{"reason"})
	registerer.MustRegister(batchesDropped)

	logsDropped := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "agent",
		Subsystem: "boundary_log_proxy",
		Name:      "logs_dropped_total",
		Help: "Total number of individual boundary log entries dropped before reaching coderd. " +
			"Reason: buffer_full = the agent's internal buffer is full; " +
			"forward_failed = the agent failed to send the batch to coderd; " +
			"boundary_channel_full = boundary's internal send channel overflowed, meaning boundary is generating logs faster than it can batch and send them; " +
			"boundary_batch_full = boundary's outgoing batch buffer overflowed after a failed flush, meaning boundary could not write to the agent's socket.",
	}, []string{"reason"})
	registerer.MustRegister(logsDropped)

	batchesForwarded := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent",
		Subsystem: "boundary_log_proxy",
		Name:      "batches_forwarded_total",
		Help: "Total number of boundary log batches successfully forwarded to coderd. " +
			"Compare with batches_dropped_total to compute a drop rate.",
	})
	registerer.MustRegister(batchesForwarded)

	return &Metrics{
		batchesDropped:   batchesDropped,
		logsDropped:      logsDropped,
		batchesForwarded: batchesForwarded,
	}
}
