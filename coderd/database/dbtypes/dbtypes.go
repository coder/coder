package dbtypes

// DERPLatency represents a KV mapping of latency to DERP servers.
// This type is only used for generation. sqlc doesn't support
// complex Go types.
type DERPLatency map[string]float64
