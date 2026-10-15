// Package promhelp provides helper functions for asserting Prometheus
// metric values in unit tests.
//
// Each value-reading helper gathers the registry on every call, so values
// read by separate calls are separate reads and need not agree with each
// other. A test that waits on one metric and then asserts another can fail
// intermittently. See "Wait for Full Completion before Asserting" in
// .claude/docs/TESTING.md.
package promhelp
