// Command natsbench benchmarks Coder's NATS-backed pubsub
// (github.com/coder/coder/v2/coderd/x/nats) under high fan-out load.
//
// A run publishes a configurable total number of messages across a set
// of publishers, subjects, subscribers, and replica nodes, then reports
// two logical throughput metrics:
//
//   - Pubs/sec: messages published divided by the publish duration.
//   - Deliveries/sec: messages delivered divided by the delivery
//     duration. Every message is delivered to every subscriber on its
//     subject, so fan-out makes deliveries exceed publishes. This is
//     logical throughput, not physical bandwidth.
//
// Correctness rules:
//
//   - Exact accounting: each subscriber's expected delivery count is
//     computed up front, and a run only completes when every subscriber
//     observed exactly that many messages.
//   - Drop policy: any dropped-message signal invalidates the run. Run
//     returns an error and the report renders the row as invalid
//     instead of publishing a throughput number.
//   - Readiness gate: in multi-replica runs, cross-route delivery is
//     silent when subscription interest has not yet propagated. Before
//     the measured phase, in-band probe messages prove that every
//     publisher node can reach every subscriber on its subjects.
//   - Bounded phases: every wait is bounded by Config.Timeout and fails
//     with diagnostics (per-subscriber shortfalls, per-node server
//     stats, goroutine dump) instead of hanging.
//
// The command runs the default scenario matrix, one named scenario, or
// a custom shape, and renders the grouped markdown report to stdout.
// The heavy benchmarks only run through this command, never in CI.
package main
