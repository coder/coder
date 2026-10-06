package unit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/xerrors"
)

var (
	ErrInvalidRequirement     = xerrors.New("invalid dependency requirement")
	ErrInvalidOutcome         = xerrors.New("invalid unit outcome")
	ErrSameOutcomeAlreadySet  = xerrors.New("same outcome already set")
	ErrOutcomeAlreadyTerminal = xerrors.New("unit outcome is already terminal")
	ErrConflictingRequirement = xerrors.New("conflicting dependency requirement")
)

// Requirement is what a conditional dependency needs from its prerequisite.
type Requirement string

const (
	RequirementSuccess    Requirement = "success"
	RequirementCompletion Requirement = "completion"
)

func (r Requirement) valid() bool {
	return r == RequirementSuccess || r == RequirementCompletion
}

// satisfiedBy reports whether a prerequisite with the given outcome lets the
// dependent run. A canceled prerequisite satisfies neither requirement.
func (r Requirement) satisfiedBy(o Outcome) bool {
	switch r {
	case RequirementSuccess:
		return o == OutcomeSucceeded
	case RequirementCompletion:
		return o == OutcomeSucceeded || o == OutcomeFailed || o == OutcomeTimedOut || o == OutcomeSkipped
	default:
		return false
	}
}

// impossible reports whether the requirement can no longer be met, so the
// dependent must be skipped instead of waiting. Only a success requirement can
// become impossible, and a canceled prerequisite does not make it so.
func (r Requirement) impossible(o Outcome) bool {
	return r == RequirementSuccess &&
		(o == OutcomeFailed || o == OutcomeTimedOut || o == OutcomeSkipped)
}

// Outcome is a unit's progress and final result for conditional dependencies.
// It is independent of Status.
type Outcome string

var _ fmt.Stringer = Outcome("")

func (o Outcome) String() string {
	if o == OutcomeNotRegistered {
		return "not registered"
	}
	return string(o)
}

const (
	OutcomeNotRegistered Outcome = ""
	OutcomePending       Outcome = "pending"
	OutcomeRunning       Outcome = "running"
	OutcomeSucceeded     Outcome = "succeeded"
	OutcomeFailed        Outcome = "failed"
	OutcomeTimedOut      Outcome = "timed_out"
	OutcomeSkipped       Outcome = "skipped"
	OutcomeCanceled      Outcome = "canceled"
)

func (o Outcome) valid() bool {
	switch o {
	case OutcomePending, OutcomeRunning, OutcomeSucceeded, OutcomeFailed,
		OutcomeTimedOut, OutcomeSkipped, OutcomeCanceled:
		return true
	default:
		return false
	}
}

// Terminal reports whether the outcome can no longer change. It says nothing
// about whether a dependency is satisfied: canceled is terminal and satisfies
// nothing. Dependency logic uses Requirement.satisfiedBy and
// Requirement.impossible instead.
func (o Outcome) Terminal() bool {
	switch o {
	case OutcomeSucceeded, OutcomeFailed, OutcomeTimedOut, OutcomeSkipped, OutcomeCanceled:
		return true
	default:
		return false
	}
}

// Decision is the manager's answer about one unit. It describes what the
// caller should do next. The manager never records an outcome on its own.
type Decision string

const (
	// DecisionWaiting means some prerequisite is neither satisfied nor
	// impossible yet.
	DecisionWaiting Decision = "waiting"
	// DecisionRunnable means every prerequisite is satisfied.
	DecisionRunnable Decision = "runnable"
	// DecisionSkip means a success prerequisite can no longer succeed. The
	// caller must record OutcomeSkipped for the unit. Until it does, the
	// unit's outcome stays as it was and the unit's own dependents keep
	// waiting.
	DecisionSkip Decision = "skip"
)

// ConditionalDependency is one unmet conditional dependency as seen at
// evaluation time.
type ConditionalDependency struct {
	Unit           ID
	DependsOn      ID
	Requirement    Requirement
	CurrentOutcome Outcome
}

// Evaluation is the decision for a unit plus its unmet conditional
// dependencies, sorted by DependsOn.
type Evaluation struct {
	Decision          Decision
	UnmetDependencies []ConditionalDependency
}

// AddConditionalDependency makes unit wait for dependsOn to reach the given
// requirement. These dependencies are independent of the exact-status
// dependencies added by AddDependency. Adding the same edge again with the
// same requirement does nothing. Adding it with a different requirement is an
// error, and the first edge stands.
func (m *Manager) AddConditionalDependency(unit ID, dependsOn ID, requirement Requirement) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch {
	case unit == "":
		return xerrors.Errorf("dependent name cannot be empty: %w", ErrUnitIDRequired)
	case dependsOn == "":
		return xerrors.Errorf("dependency name cannot be empty: %w", ErrUnitIDRequired)
	case !m.registered(unit):
		return xerrors.Errorf("dependent unit %q must be registered first: %w", unit, ErrUnitNotFound)
	case !requirement.valid():
		return xerrors.Errorf("dependency from %q to %q requires %q: %w", unit, dependsOn, requirement, ErrInvalidRequirement)
	}

	// Graph.AddEdge overwrites the edge type when the same pair is added again,
	// and the status graph relies on that: coder exp sync want repeats
	// AddDependency with the same status. So the conflict check lives here.
	for _, edge := range m.conditionalGraph.GetForwardAdjacentVertices(unit) {
		if edge.To != dependsOn {
			continue
		}
		if edge.Edge == requirement {
			return nil
		}
		return xerrors.Errorf("dependency from %q to %q already requires %q, cannot also require %q: %w",
			unit, dependsOn, edge.Edge, requirement, ErrConflictingRequirement)
	}

	if err := m.conditionalGraph.AddEdge(unit, dependsOn, requirement); err != nil {
		return xerrors.Errorf("adding conditional edge for unit %q: %w", unit, errors.Join(ErrFailedToAddDependency, err))
	}

	m.broadcastOutcomeChangeUnsafe()
	return nil
}

// UpdateOutcome records a unit's outcome and wakes every WaitForDecision
// caller. A terminal outcome cannot be changed, because dependents may already
// have been released or skipped because of it.
func (m *Manager) UpdateOutcome(unit ID, outcome Outcome) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch {
	case unit == "":
		return xerrors.Errorf("updating outcome for unit %q: %w", unit, ErrUnitIDRequired)
	case !m.registered(unit):
		return xerrors.Errorf("unit %q must be registered first: %w", unit, ErrUnitNotFound)
	case !outcome.valid():
		return xerrors.Errorf("updating outcome for unit %q to %q: %w", unit, outcome, ErrInvalidOutcome)
	}

	u := m.units[unit]
	switch {
	case u.outcome == outcome:
		return xerrors.Errorf("checking outcome for unit %q: %w", unit, ErrSameOutcomeAlreadySet)
	case u.outcome.Terminal():
		return xerrors.Errorf("unit %q is already %q, cannot become %q: %w", unit, u.outcome, outcome, ErrOutcomeAlreadyTerminal)
	}

	u.outcome = outcome
	m.units[unit] = u
	m.broadcastOutcomeChangeUnsafe()
	return nil
}

// Evaluate returns the current decision for a unit. The decision describes the
// unit's prerequisites, not the unit itself: a unit whose own outcome is
// terminal still answers from its edges. Evaluate never changes the unit's
// outcome. On DecisionSkip the caller records OutcomeSkipped itself.
func (m *Manager) Evaluate(id ID) (Evaluation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.evaluateUnsafe(id)
}

// WaitForDecision blocks until the unit is runnable or must be skipped, or ctx
// is done. It has no deadline of its own and never treats elapsed time as
// satisfaction. Only the caller's ctx can end the wait.
func (m *Manager) WaitForDecision(ctx context.Context, id ID) (Evaluation, error) {
	for {
		m.mu.RLock()
		evaluation, err := m.evaluateUnsafe(id)
		changed := m.outcomeChanged
		m.mu.RUnlock()
		if err != nil {
			return Evaluation{}, err
		}
		if evaluation.Decision != DecisionWaiting {
			return evaluation, nil
		}

		select {
		case <-ctx.Done():
			return Evaluation{}, ctx.Err()
		case <-changed:
		}
	}
}

// evaluateUnsafe is Evaluate without locking. The caller must hold at least a
// read lock.
func (m *Manager) evaluateUnsafe(id ID) (Evaluation, error) {
	if id == "" {
		return Evaluation{}, xerrors.Errorf("unit ID cannot be empty: %w", ErrUnitIDRequired)
	}
	// Unlike IsReady, this path never answers for a unit it does not know. A
	// dependent must not run before its prerequisites are even registered.
	if !m.registered(id) {
		return Evaluation{}, xerrors.Errorf("evaluating unit %q: %w", id, ErrUnitNotFound)
	}

	evaluation := Evaluation{Decision: DecisionRunnable}
	for _, edge := range m.conditionalGraph.GetForwardAdjacentVertices(id) {
		// An unregistered prerequisite has an empty outcome, which satisfies
		// nothing and is not impossible, so the dependent keeps waiting.
		outcome := m.units[edge.To].outcome
		if edge.Edge.satisfiedBy(outcome) {
			continue
		}

		evaluation.UnmetDependencies = append(evaluation.UnmetDependencies, ConditionalDependency{
			Unit:           id,
			DependsOn:      edge.To,
			Requirement:    edge.Edge,
			CurrentOutcome: outcome,
		})
		// A skip is decided as soon as one edge is impossible, even if other
		// prerequisites are still running.
		switch {
		case edge.Edge.impossible(outcome):
			evaluation.Decision = DecisionSkip
		case evaluation.Decision != DecisionSkip:
			evaluation.Decision = DecisionWaiting
		}
	}

	slices.SortFunc(evaluation.UnmetDependencies, func(a, b ConditionalDependency) int {
		return strings.Compare(string(a.DependsOn), string(b.DependsOn))
	})
	return evaluation, nil
}

// broadcastOutcomeChangeUnsafe wakes every WaitForDecision caller. Closing the
// channel wakes all current waiters; replacing it gives later waiters a fresh
// one. The caller must hold the write lock. That is what lets WaitForDecision
// capture the channel under a read lock, unlock, then select: any change in
// between closes the captured channel, so no wakeup is missed.
func (m *Manager) broadcastOutcomeChangeUnsafe() {
	close(m.outcomeChanged)
	m.outcomeChanged = make(chan struct{})
}
