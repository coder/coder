package agentcontext

// ManagerStarted exposes the unexported started() channel for
// use by external _test packages. Production code does not need
// this signal; the agent calls Run synchronously after wiring
// the Manager. Tests use it to coordinate without polling.
func ManagerStarted(m *Manager) <-chan struct{} { return m.started() }

// ProjectChainForTest exposes the unexported walk-up helper so
// external _test packages can assert the ancestor chain directly.
func ProjectChainForTest(workingDir string) []string { return projectChain(workingDir) }
