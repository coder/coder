package aibridged

// Expose the production timeouts only to external tests.
const (
	BridgeDrainTimeoutForTest  = bridgeDrainTimeout
	PoolShutdownTimeoutForTest = poolShutdownTimeout
)
