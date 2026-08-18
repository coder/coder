package unit

import (
	"context"
	"errors"
	"slices"
	"strings"

	"golang.org/x/xerrors"
)

var (
	ErrInvalidRequirement    = xerrors.New("invalid dependency requirement")
	ErrInvalidOutcome        = xerrors.New("invalid unit outcome")
	ErrSameOutcomeAlreadySet = xerrors.New("same outcome already set")
)

// Requirement describes the outcome a conditional dependency requires.
type Requirement string

const (
	RequirementSuccess    Requirement = "success"
	RequirementCompletion Requirement = "completion"
)

func (r Requirement) valid() bool {
	return r == RequirementSuccess || r == RequirementCompletion
}

// Outcome represents a unit's progress and terminal result.
type Outcome string

const (
	OutcomeNotRegistered Outcome = ""
	OutcomePending       Outcome = "pending"
	OutcomeRunning       Outcome = "running"
	OutcomeSucceeded     Outcome = "succeeded"
	OutcomeFailed        Outcome = "failed"
	OutcomeTimedOut      Outcome = "timed_out"
	OutcomeSkipped       Outcome = "skipped"
)

func (o Outcome) valid() bool {
	switch o {
	case OutcomePending, OutcomeRunning, OutcomeSucceeded, OutcomeFailed, OutcomeTimedOut, OutcomeSkipped:
		return true
	default:
		return false
	}
}

// Terminal reports whether the outcome is final for dependency evaluation.
func (o Outcome) Terminal() bool {
	switch o {
	case OutcomeSucceeded, OutcomeFailed, OutcomeTimedOut, OutcomeSkipped:
		return true
	default:
		return false
	}
}

// Decision describes whether a unit should keep waiting, run, or be skipped.
type Decision string

const (
	DecisionWaiting  Decision = "waiting"
	DecisionRunnable Decision = "runnable"
	DecisionSkipped  Decision = "skipped"
)

// ConditionalDependency is a point-in-time view of an unsatisfied
// outcome-based dependency.
type ConditionalDependency struct {
	Unit           ID
	DependsOn      ID
	Requirement    Requirement
	CurrentOutcome Outcome
}

// Evaluation is the current decision for a unit and its unsatisfied
// outcome-based dependencies. UnmetDependencies is sorted by DependsOn.
type Evaluation struct {
	Decision          Decision
	UnmetDependencies []ConditionalDependency
}

// AddConditionalDependency makes unit depend on the outcome of dependsOn.
// These dependencies are independent from the exact-status dependencies added
// by AddDependency.
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

	if err := m.conditionalGraph.AddEdge(unit, dependsOn, requirement); err != nil {
		return xerrors.Errorf("adding conditional edge for unit %q: %w", unit, errors.Join(ErrFailedToAddDependency, err))
	}

	m.broadcastOutcomeChangeUnsafe()
	return nil
}

// UpdateOutcome updates a unit's outcome and wakes conditional dependency
// waiters.
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
	if u.outcome == outcome {
		return xerrors.Errorf("checking outcome for unit %q: %w", unit, ErrSameOutcomeAlreadySet)
	}

	u.outcome = outcome
	m.units[unit] = u
	m.broadcastOutcomeChangeUnsafe()
	return nil
}

// Evaluate returns the current conditional dependency decision for a unit.
func (m *Manager) Evaluate(id ID) (Evaluation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.evaluateUnsafe(id)
}

// WaitForDecision waits until a unit is runnable, must be skipped, or ctx is
// done. It never treats elapsed waiting time as dependency satisfaction.
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

// evaluateUnsafe evaluates conditional dependencies. The caller must hold at
// least a read lock.
func (m *Manager) evaluateUnsafe(id ID) (Evaluation, error) {
	if id == "" {
		return Evaluation{}, xerrors.Errorf("unit ID cannot be empty: %w", ErrUnitIDRequired)
	}
	if !m.registered(id) {
		return Evaluation{}, xerrors.Errorf("evaluating unit %q: %w", id, ErrUnitNotFound)
	}

	evaluation := Evaluation{Decision: DecisionRunnable}
	for _, edge := range m.conditionalGraph.GetForwardAdjacentVertices(id) {
		outcome := m.units[edge.To].outcome
		satisfied := edge.Edge == RequirementSuccess && outcome == OutcomeSucceeded ||
			edge.Edge == RequirementCompletion && outcome.Terminal()
		if satisfied {
			continue
		}

		evaluation.UnmetDependencies = append(evaluation.UnmetDependencies, ConditionalDependency{
			Unit:           id,
			DependsOn:      edge.To,
			Requirement:    edge.Edge,
			CurrentOutcome: outcome,
		})
		if edge.Edge == RequirementSuccess && outcome.Terminal() {
			evaluation.Decision = DecisionSkipped
		} else if evaluation.Decision != DecisionSkipped {
			evaluation.Decision = DecisionWaiting
		}
	}

	slices.SortFunc(evaluation.UnmetDependencies, func(a, b ConditionalDependency) int {
		return strings.Compare(string(a.DependsOn), string(b.DependsOn))
	})
	return evaluation, nil
}

// broadcastOutcomeChangeUnsafe wakes all conditional dependency waiters. The
// caller must hold the write lock.
func (m *Manager) broadcastOutcomeChangeUnsafe() {
	close(m.outcomeChanged)
	m.outcomeChanged = make(chan struct{})
}
