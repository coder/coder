// Package nats is an experimental embedded NATS-backed implementation of
// coderd/database/pubsub.Pubsub.
//
// This package lives under coderd/x/ because it is not yet wired into coderd
// and its API is not stable. It is intended as a drop-in replacement for the
// Postgres LISTEN/NOTIFY-backed pubsub used today, with an embedded NATS
// server clustered across coderd replicas. See
// docs/internal/nats-pubsub-research-and-plan.md for the design.
//
// Nothing in this package is currently imported by production code. Do not
// rely on its exported surface remaining backwards compatible.
package nats
