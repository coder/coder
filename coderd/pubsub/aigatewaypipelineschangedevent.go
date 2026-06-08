package pubsub

// AIGatewayPipelinesChangedChannel is the pubsub channel that carries AI gateway
// policy and pipeline lifecycle events: policy / policy-version / pipeline /
// pipeline-version create, active-version swaps, enable/disable, and soft-delete.
// Subscribers (aibridged) rebuild their in-memory policy pipeline snapshot on
// receipt.
//
// The payload is an empty invalidation hint; subscribers refetch the
// authoritative state from the database, so dropped messages only delay
// convergence rather than diverge state. A periodic reload provides a backstop
// against missed notifications and read-replica lag.
//
// Publishers MUST publish only after the writing transaction commits, so a
// subscriber's consistent read cannot race a half-written change.
const AIGatewayPipelinesChangedChannel = "ai_gateway_pipelines_changed"
