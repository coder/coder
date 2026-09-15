package testutil

import "testing"

// Cleanup registers f with t.Cleanup and drops the reference to f as soon as
// it has run.
//
// testing.T removes a cleanup from its list by reslicing, which leaves the
// closure in the backing array, and every top-level test stays reachable from
// the test binary's root until the binary exits. A cleanup closure that
// captures a heavyweight object, such as a coderd server and its router,
// therefore keeps that object alive for the rest of the package run. Across
// thousands of tests this grows the live heap by gigabytes and makes every GC
// cycle proportionally more expensive. Use this for cleanups registered by
// helpers that build large object graphs.
func Cleanup(t testing.TB, f func()) {
	t.Helper()
	h := &cleanupHolder{f: f}
	t.Cleanup(h.run)
}

type cleanupHolder struct {
	f func()
}

func (h *cleanupHolder) run() {
	f := h.f
	h.f = nil
	f()
}
