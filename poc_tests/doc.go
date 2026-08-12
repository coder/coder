// Package poctests holds acceptance tests for the proof of concept described
// in poc_audit/work_breakdown.md.
//
// These tests are deliberately kept out of the packages they exercise. They
// are whole-package tests in the sense used by that document: each one is
// sized to a work package and passes only when that package is complete.
//
// The directory also holds the pieces a test needs but cannot express in Go:
// cmd/ for executables that run inside a workspace, and testdata/ for the
// startup scripts that launch them.
package poctests
