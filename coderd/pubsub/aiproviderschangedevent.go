package pubsub

// AIProvidersChangedChannel is the pubsub channel that carries AI
// provider lifecycle events: provider create / update / soft-delete
// and key insert / delete. Subscribers (aibridged, aibridgeproxyd,
// chatd) reload their in-memory provider snapshot on receipt.
//
// The payload is an empty invalidation hint; subscribers refetch the
// authoritative state from the database, so dropped messages only
// delay convergence rather than diverge state.
const AIProvidersChangedChannel = "ai_providers_changed"
