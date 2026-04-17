package toolsdk

// HasAgentConnObserver reports whether d has an agent-conn observer configured.
// Test-only; allows validating option wiring without exporting the field.
func (d Deps) HasAgentConnObserver() bool {
	return d.onAgentConn != nil
}

// InvokeAgentConnObserver invokes the configured observer (if any) and
// returns its release func, for tests that want to verify the observer is
// actually plumbed through.
func (d Deps) InvokeAgentConnObserver() (release func(), ok bool) {
	if d.onAgentConn == nil {
		return nil, false
	}
	return d.onAgentConn(), true
}
