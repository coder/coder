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
//   - Production defaults: the benchmark measures the pubsub as Coder
//     configures it. It does not autotune queue or pending limits to
//     avoid drops; sane defaults are the thing under test.
//   - Drops as a metric: each subscriber's expected delivery count is
//     computed up front, so the exact, complete loss is Expected minus
//     Delivered. Dropped messages are reported in the Drops column, not
//     treated as a failure; a run that drops still reports the
//     throughput it achieved. Only a real error (publish failure,
//     cancellation, hard timeout) invalidates a run.
//   - Delivery completion: a zero-drop run finishes precisely when every
//     subscriber reaches its expected count. A run that dropped messages
//     can never reach that count, so the deliver phase instead completes
//     by quiescence: once the delivery counter stays flat for a fixed
//     settle window, the phase ends, and the rate is measured to the
//     last observed delivery so the idle wait stays out of it.
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
