package agentcontext

// ManagerStarted exposes the unexported started() channel for
// use by external _test packages. Production code does not need
// this signal; the agent calls Run synchronously after wiring
// the Manager. Tests use it to coordinate without polling.
func ManagerStarted(m *Manager) <-chan struct{} { return m.started() }

// HashedResources exposes the drift-hash resource filter so tests can
// compute the hash a gated push is expected to carry.
func HashedResources(resources []Resource) []Resource { return driftResources(resources) }
