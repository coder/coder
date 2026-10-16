package aibridged

// Expose the production timeouts only to external tests.
const (
	BridgeDrainTimeout  = bridgeDrainTimeout
	PoolShutdownTimeout = poolShutdownTimeout
)
