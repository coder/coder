// Package unit tracks named units of work and the dependencies between them.
//
// A Manager holds two independent kinds of dependency. Status dependencies
// (AddDependency, IsReady) wait for a prerequisite to reach one exact Status
// and back coder exp sync. Conditional dependencies (AddConditionalDependency,
// Evaluate, WaitForDecision) wait for a prerequisite's Outcome to satisfy a
// Requirement and back declarative script ordering. "Conditional dependency"
// is the same thing the script ordering RFC calls an "outcome-aware
// dependency".
//
// A Manager instance is the isolation boundary between callers. Unit IDs are
// not namespaced, so callers that must not see each other's units, such as
// the agent socket API and a script execution, use separate instances.
//
// Cycle detection is per graph. A status edge in one direction and a
// conditional edge in the other form a loop neither check sees.
package unit
