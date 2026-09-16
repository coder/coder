package lab

// Done returns the completion channel of s's current run. It is closed when
// every goroutine the run launched has exited, and is nil when no run is
// installed.
func Done(s Scenario) <-chan struct{} {
	return s.(interface{ doneChan() <-chan struct{} }).doneChan()
}

// ClampKnob exposes clampKnob for tests.
var ClampKnob = clampKnob

// ParseKnobs exposes parseKnobs for tests.
var ParseKnobs = parseKnobs
