package unit_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/testutil"
)

func TestManager_ConditionalDependencyValidation(t *testing.T) {
	t.Parallel()

	t.Run("InvalidArguments", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		require.NoError(t, manager.Register(unitA))

		err := manager.AddConditionalDependency("", unitB, unit.RequirementSuccess)
		require.ErrorIs(t, err, unit.ErrUnitIDRequired)
		err = manager.AddConditionalDependency(unitA, "", unit.RequirementSuccess)
		require.ErrorIs(t, err, unit.ErrUnitIDRequired)
		err = manager.AddConditionalDependency(unitB, unitA, unit.RequirementSuccess)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
		err = manager.AddConditionalDependency(unitA, unitB, unit.RequirementSuccess)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)

		require.NoError(t, manager.Register(unitB))
		err = manager.AddConditionalDependency(unitA, unitB, "unknown")
		require.ErrorIs(t, err, unit.ErrInvalidRequirement)
		err = manager.AddConditionalDependency(unitA, unitB, "")
		require.ErrorIs(t, err, unit.ErrInvalidRequirement)
	})

	t.Run("Cycle", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		require.NoError(t, manager.Register(unitA))
		require.NoError(t, manager.Register(unitB))
		require.NoError(t, manager.AddConditionalDependency(unitA, unitB, unit.RequirementSuccess))

		err := manager.AddConditionalDependency(unitB, unitA, unit.RequirementCompletion)
		require.ErrorIs(t, err, unit.ErrCycleDetected)
		require.ErrorIs(t, err, unit.ErrFailedToAddDependency)
	})

	t.Run("SameRequirementTwiceIsOneEdge", func(t *testing.T) {
		t.Parallel()

		manager := conditionalManager(t, unit.RequirementSuccess)
		require.NoError(t, manager.AddConditionalDependency(unitA, unitB, unit.RequirementSuccess))

		evaluation, err := manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionWaiting, evaluation.Decision)
		require.Len(t, evaluation.UnmetDependencies, 1)
	})

	t.Run("ConflictingRequirementIsRejected", func(t *testing.T) {
		t.Parallel()

		manager := conditionalManager(t, unit.RequirementSuccess)
		err := manager.AddConditionalDependency(unitA, unitB, unit.RequirementCompletion)
		require.ErrorIs(t, err, unit.ErrConflictingRequirement)

		// The first edge stands: a failed prerequisite still skips the dependent.
		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeFailed))
		evaluation, err := manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionSkip, evaluation.Decision)
		require.Equal(t, unit.RequirementSuccess, evaluation.UnmetDependencies[0].Requirement)
	})
}

func TestManager_UpdateOutcome(t *testing.T) {
	t.Parallel()

	t.Run("Transitions", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		require.NoError(t, manager.Register(unitA))

		snapshot, err := manager.Unit(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.OutcomePending, snapshot.Outcome())

		require.NoError(t, manager.UpdateOutcome(unitA, unit.OutcomeRunning))
		snapshot, err = manager.Unit(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.OutcomeRunning, snapshot.Outcome())

		require.NoError(t, manager.UpdateOutcome(unitA, unit.OutcomeSucceeded))
		snapshot, err = manager.Unit(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.OutcomeSucceeded, snapshot.Outcome())
	})

	t.Run("InvalidArguments", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		require.NoError(t, manager.Register(unitA))
		require.NoError(t, manager.UpdateOutcome(unitA, unit.OutcomeRunning))

		err := manager.UpdateOutcome(unitA, unit.OutcomeRunning)
		require.ErrorIs(t, err, unit.ErrSameOutcomeAlreadySet)
		err = manager.UpdateOutcome(unitA, "unknown")
		require.ErrorIs(t, err, unit.ErrInvalidOutcome)
		err = manager.UpdateOutcome(unitA, unit.OutcomeNotRegistered)
		require.ErrorIs(t, err, unit.ErrInvalidOutcome)
		err = manager.UpdateOutcome(unitB, unit.OutcomeRunning)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
		err = manager.UpdateOutcome("", unit.OutcomeRunning)
		require.ErrorIs(t, err, unit.ErrUnitIDRequired)
	})

	t.Run("TerminalOutcomeIsFinal", func(t *testing.T) {
		t.Parallel()

		for _, terminal := range []unit.Outcome{
			unit.OutcomeSucceeded,
			unit.OutcomeFailed,
			unit.OutcomeTimedOut,
			unit.OutcomeSkipped,
			unit.OutcomeCanceled,
		} {
			manager := unit.NewManager()
			require.NoError(t, manager.Register(unitA))
			require.NoError(t, manager.UpdateOutcome(unitA, terminal))

			for _, next := range []unit.Outcome{
				unit.OutcomeRunning,
				unit.OutcomeSucceeded,
				unit.OutcomeFailed,
				unit.OutcomeCanceled,
			} {
				if next == terminal {
					continue
				}
				err := manager.UpdateOutcome(unitA, next)
				require.ErrorIs(t, err, unit.ErrOutcomeAlreadyTerminal, "%s -> %s", terminal, next)
			}

			snapshot, err := manager.Unit(unitA)
			require.NoError(t, err)
			require.Equal(t, terminal, snapshot.Outcome())
		}
	})

	t.Run("CanceledWhilePendingOrRunning", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		require.NoError(t, manager.Register(unitA))
		require.NoError(t, manager.Register(unitB))

		require.NoError(t, manager.UpdateOutcome(unitA, unit.OutcomeCanceled))
		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeRunning))
		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeCanceled))

		for _, id := range []unit.ID{unitA, unitB} {
			snapshot, err := manager.Unit(id)
			require.NoError(t, err)
			require.Equal(t, unit.OutcomeCanceled, snapshot.Outcome())
		}
	})
}

func TestManager_ConditionalDependenciesPreserveStatusDependencies(t *testing.T) {
	t.Parallel()

	manager := unit.NewManager()
	require.NoError(t, manager.Register(unitA))
	require.NoError(t, manager.Register(unitB))
	require.NoError(t, manager.AddConditionalDependency(unitA, unitB, unit.RequirementSuccess))

	ready, err := manager.IsReady(unitA)
	require.NoError(t, err)
	require.True(t, ready)
	evaluation, err := manager.Evaluate(unitA)
	require.NoError(t, err)
	require.Equal(t, unit.DecisionWaiting, evaluation.Decision)

	require.NoError(t, manager.AddDependency(unitA, unitB, unit.StatusStarted))
	ready, err = manager.IsReady(unitA)
	require.NoError(t, err)
	require.False(t, ready)
	require.NoError(t, manager.UpdateStatus(unitB, unit.StatusStarted))
	ready, err = manager.IsReady(unitA)
	require.NoError(t, err)
	require.True(t, ready)

	evaluation, err = manager.Evaluate(unitA)
	require.NoError(t, err)
	require.Equal(t, unit.DecisionWaiting, evaluation.Decision)

	require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeSucceeded))
	evaluation, err = manager.Evaluate(unitA)
	require.NoError(t, err)
	require.Equal(t, unit.DecisionRunnable, evaluation.Decision)
	snapshot, err := manager.Unit(unitB)
	require.NoError(t, err)
	require.Equal(t, unit.StatusStarted, snapshot.Status())
}

func TestOutcome_Terminal(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		outcome  unit.Outcome
		terminal bool
	}{
		{name: "not registered", outcome: unit.OutcomeNotRegistered},
		{name: "pending", outcome: unit.OutcomePending},
		{name: "running", outcome: unit.OutcomeRunning},
		{name: "succeeded", outcome: unit.OutcomeSucceeded, terminal: true},
		{name: "failed", outcome: unit.OutcomeFailed, terminal: true},
		{name: "timed out", outcome: unit.OutcomeTimedOut, terminal: true},
		{name: "skipped", outcome: unit.OutcomeSkipped, terminal: true},
		{name: "canceled", outcome: unit.OutcomeCanceled, terminal: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.terminal, test.outcome.Terminal())
		})
	}
}

func TestOutcome_String(t *testing.T) {
	t.Parallel()

	require.Equal(t, "not registered", unit.OutcomeNotRegistered.String())
	require.Equal(t, "timed_out", unit.OutcomeTimedOut.String())
}

// TestManager_EvaluateRequirementByOutcome pins the decision for every
// requirement and prerequisite outcome pair.
func TestManager_EvaluateRequirementByOutcome(t *testing.T) {
	t.Parallel()

	decisions := map[unit.Requirement]map[unit.Outcome]unit.Decision{
		unit.RequirementSuccess: {
			unit.OutcomePending:   unit.DecisionWaiting,
			unit.OutcomeRunning:   unit.DecisionWaiting,
			unit.OutcomeSucceeded: unit.DecisionRunnable,
			unit.OutcomeFailed:    unit.DecisionSkip,
			unit.OutcomeTimedOut:  unit.DecisionSkip,
			unit.OutcomeSkipped:   unit.DecisionSkip,
			unit.OutcomeCanceled:  unit.DecisionWaiting,
		},
		unit.RequirementCompletion: {
			unit.OutcomePending:   unit.DecisionWaiting,
			unit.OutcomeRunning:   unit.DecisionWaiting,
			unit.OutcomeSucceeded: unit.DecisionRunnable,
			unit.OutcomeFailed:    unit.DecisionRunnable,
			unit.OutcomeTimedOut:  unit.DecisionRunnable,
			unit.OutcomeSkipped:   unit.DecisionRunnable,
			unit.OutcomeCanceled:  unit.DecisionWaiting,
		},
	}

	for requirement, byOutcome := range decisions {
		for outcome, want := range byOutcome {
			t.Run(fmt.Sprintf("%s/%s", requirement, outcome), func(t *testing.T) {
				t.Parallel()

				manager := conditionalManager(t, requirement)
				if outcome != unit.OutcomePending {
					require.NoError(t, manager.UpdateOutcome(unitB, outcome))
				}

				evaluation, err := manager.Evaluate(unitA)
				require.NoError(t, err)
				require.Equal(t, want, evaluation.Decision)
				if want == unit.DecisionRunnable {
					require.Empty(t, evaluation.UnmetDependencies)
					return
				}
				require.Equal(t, []unit.ConditionalDependency{{
					Unit:           unitA,
					DependsOn:      unitB,
					Requirement:    requirement,
					CurrentOutcome: outcome,
				}}, evaluation.UnmetDependencies)
			})
		}
	}
}

func TestManager_Evaluate(t *testing.T) {
	t.Parallel()

	t.Run("NoDependenciesIsRunnable", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		require.NoError(t, manager.Register(unitA))

		evaluation, err := manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionRunnable, evaluation.Decision)
		require.Empty(t, evaluation.UnmetDependencies)
	})

	t.Run("SuccessWaitsThenRuns", func(t *testing.T) {
		t.Parallel()

		manager := conditionalManager(t, unit.RequirementSuccess)

		evaluation, err := manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionWaiting, evaluation.Decision)
		require.Equal(t, unit.OutcomePending, evaluation.UnmetDependencies[0].CurrentOutcome)

		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeRunning))
		evaluation, err = manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionWaiting, evaluation.Decision)
		require.Equal(t, unit.OutcomeRunning, evaluation.UnmetDependencies[0].CurrentOutcome)

		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeSucceeded))
		evaluation, err = manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionRunnable, evaluation.Decision)
		require.Empty(t, evaluation.UnmetDependencies)
	})

	t.Run("UnregisteredPrerequisiteIsRejected", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		require.NoError(t, manager.Register(unitA))
		err := manager.AddConditionalDependency(unitA, unitB, unit.RequirementSuccess)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)

		// No edge was added, so the dependent is not left waiting on it.
		evaluation, err := manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionRunnable, evaluation.Decision)
		require.Empty(t, evaluation.UnmetDependencies)
	})

	t.Run("CanceledPrerequisiteKeepsWaiting", func(t *testing.T) {
		t.Parallel()

		for _, requirement := range []unit.Requirement{unit.RequirementSuccess, unit.RequirementCompletion} {
			manager := conditionalManager(t, requirement)
			require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeRunning))
			require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeCanceled))

			evaluation, err := manager.Evaluate(unitA)
			require.NoError(t, err)
			require.Equal(t, unit.DecisionWaiting, evaluation.Decision, "requirement %s", requirement)
			require.Equal(t, unit.OutcomeCanceled, evaluation.UnmetDependencies[0].CurrentOutcome)
		}
	})

	t.Run("MixedRequirements", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		for _, id := range []unit.ID{unitA, unitB, unitC} {
			require.NoError(t, manager.Register(id))
		}
		require.NoError(t, manager.AddConditionalDependency(unitA, unitB, unit.RequirementSuccess))
		require.NoError(t, manager.AddConditionalDependency(unitA, unitC, unit.RequirementCompletion))

		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeSucceeded))
		require.NoError(t, manager.UpdateOutcome(unitC, unit.OutcomeRunning))
		evaluation, err := manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionWaiting, evaluation.Decision)
		require.Equal(t, []unit.ID{unitC}, conditionalDependencyIDs(evaluation.UnmetDependencies))

		// A failed completion prerequisite releases rather than skips.
		require.NoError(t, manager.UpdateOutcome(unitC, unit.OutcomeFailed))
		evaluation, err = manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionRunnable, evaluation.Decision)
	})

	t.Run("SkipIsDecidedOnFirstFailure", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		for _, id := range []unit.ID{unitA, unitB, unitC} {
			require.NoError(t, manager.Register(id))
		}
		require.NoError(t, manager.AddConditionalDependency(unitA, unitB, unit.RequirementSuccess))
		require.NoError(t, manager.AddConditionalDependency(unitA, unitC, unit.RequirementSuccess))
		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeRunning))
		require.NoError(t, manager.UpdateOutcome(unitC, unit.OutcomeFailed))

		evaluation, err := manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionSkip, evaluation.Decision)
		require.Equal(t, []unit.ConditionalDependency{
			{Unit: unitA, DependsOn: unitB, Requirement: unit.RequirementSuccess, CurrentOutcome: unit.OutcomeRunning},
			{Unit: unitA, DependsOn: unitC, Requirement: unit.RequirementSuccess, CurrentOutcome: unit.OutcomeFailed},
		}, evaluation.UnmetDependencies)

		// The other prerequisite finishing later does not change the decision.
		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeSucceeded))
		evaluation, err = manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionSkip, evaluation.Decision)
		require.Equal(t, []unit.ID{unitC}, conditionalDependencyIDs(evaluation.UnmetDependencies))
	})

	t.Run("UnmetDependenciesAreSorted", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		for _, id := range []unit.ID{unitA, unitB, unitC, unitD} {
			require.NoError(t, manager.Register(id))
		}
		require.NoError(t, manager.AddConditionalDependency(unitA, unitD, unit.RequirementCompletion))
		require.NoError(t, manager.AddConditionalDependency(unitA, unitB, unit.RequirementSuccess))
		require.NoError(t, manager.AddConditionalDependency(unitA, unitC, unit.RequirementCompletion))
		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeFailed))

		evaluation, err := manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionSkip, evaluation.Decision)
		require.Equal(t, []unit.ID{unitB, unitC, unitD}, conditionalDependencyIDs(evaluation.UnmetDependencies))
	})

	t.Run("SkippedPropagatesByRequirement", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		for _, id := range []unit.ID{unitA, unitB, unitC, unitD} {
			require.NoError(t, manager.Register(id))
		}
		require.NoError(t, manager.AddConditionalDependency(unitB, unitA, unit.RequirementSuccess))
		require.NoError(t, manager.AddConditionalDependency(unitC, unitB, unit.RequirementCompletion))
		require.NoError(t, manager.AddConditionalDependency(unitD, unitB, unit.RequirementSuccess))
		require.NoError(t, manager.UpdateOutcome(unitA, unit.OutcomeFailed))

		evaluation, err := manager.Evaluate(unitB)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionSkip, evaluation.Decision)

		// The manager does not record the skip itself. Until the caller does,
		// units downstream of B keep waiting.
		for _, id := range []unit.ID{unitC, unitD} {
			evaluation, err = manager.Evaluate(id)
			require.NoError(t, err)
			require.Equal(t, unit.DecisionWaiting, evaluation.Decision)
		}

		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeSkipped))
		evaluation, err = manager.Evaluate(unitC)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionRunnable, evaluation.Decision)
		evaluation, err = manager.Evaluate(unitD)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionSkip, evaluation.Decision)
	})

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		_, err := manager.Evaluate("")
		require.ErrorIs(t, err, unit.ErrUnitIDRequired)
		_, err = manager.Evaluate(unitA)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
	})
}

func TestManager_WaitForDecision(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsAtOnceWhenDecided", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		manager := conditionalManager(t, unit.RequirementSuccess)
		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeSucceeded))

		evaluation, err := manager.WaitForDecision(ctx, unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionRunnable, evaluation.Decision)

		require.NoError(t, manager.Register(unitC))
		evaluation, err = manager.WaitForDecision(ctx, unitC)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionRunnable, evaluation.Decision)
	})

	t.Run("WakesWhenRunnable", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		manager := conditionalManager(t, unit.RequirementSuccess)
		result := make(chan unit.Evaluation, 1)
		errCh := make(chan error, 1)
		go func() {
			evaluation, err := manager.WaitForDecision(ctx, unitA)
			result <- evaluation
			errCh <- err
		}()

		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeSucceeded))
		require.Equal(t, unit.DecisionRunnable, testutil.RequireReceive(ctx, t, result).Decision)
		require.NoError(t, testutil.RequireReceive(ctx, t, errCh))
	})

	t.Run("WakesWhenSkipped", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		manager := conditionalManager(t, unit.RequirementSuccess)
		result := make(chan unit.Evaluation, 1)
		errCh := make(chan error, 1)
		go func() {
			evaluation, err := manager.WaitForDecision(ctx, unitA)
			result <- evaluation
			errCh <- err
		}()

		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeFailed))
		require.Equal(t, unit.DecisionSkip, testutil.RequireReceive(ctx, t, result).Decision)
		require.NoError(t, testutil.RequireReceive(ctx, t, errCh))
	})

	t.Run("WakesWhenEdgeAdded", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		manager := conditionalManager(t, unit.RequirementSuccess)
		require.NoError(t, manager.Register(unitC))
		require.NoError(t, manager.UpdateOutcome(unitC, unit.OutcomeFailed))
		result := make(chan unit.Evaluation, 1)
		go func() {
			evaluation, _ := manager.WaitForDecision(ctx, unitA)
			result <- evaluation
		}()

		// A new success edge to an already failed unit makes A skipped.
		require.NoError(t, manager.AddConditionalDependency(unitA, unitC, unit.RequirementSuccess))
		require.Equal(t, unit.DecisionSkip, testutil.RequireReceive(ctx, t, result).Decision)
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		manager := conditionalManager(t, unit.RequirementSuccess)
		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeRunning))

		_, err := manager.WaitForDecision(ctx, unitA)
		require.ErrorIs(t, err, context.Canceled)

		// Nothing changed because the wait was abandoned.
		snapshot, err := manager.Unit(unitB)
		require.NoError(t, err)
		require.Equal(t, unit.OutcomeRunning, snapshot.Outcome())
		evaluation, err := manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionWaiting, evaluation.Decision)
	})

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		_, err := manager.WaitForDecision(context.Background(), unitA)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
		_, err = manager.WaitForDecision(context.Background(), "")
		require.ErrorIs(t, err, unit.ErrUnitIDRequired)
	})
}

func TestManager_WaitForDecision_ManyWaiters(t *testing.T) {
	t.Parallel()

	const count = 50
	ctx := testutil.Context(t, testutil.WaitMedium)
	manager := unit.NewManager()

	dependents := make([]unit.ID, 0, count)
	prerequisites := make([]unit.ID, 0, count)
	for i := range count {
		dependent := unit.ID(fmt.Sprintf("dependent-%02d", i))
		prerequisite := unit.ID(fmt.Sprintf("prerequisite-%02d", i))
		require.NoError(t, manager.Register(dependent))
		require.NoError(t, manager.Register(prerequisite))
		require.NoError(t, manager.AddConditionalDependency(dependent, prerequisite, unit.RequirementSuccess))
		dependents = append(dependents, dependent)
		prerequisites = append(prerequisites, prerequisite)
	}

	type result struct {
		id         unit.ID
		evaluation unit.Evaluation
		err        error
	}
	results := make(chan result, count)
	for _, id := range dependents {
		go func() {
			evaluation, err := manager.WaitForDecision(ctx, id)
			results <- result{id: id, evaluation: evaluation, err: err}
		}()
	}

	// One update releases exactly one waiter.
	require.NoError(t, manager.UpdateOutcome(prerequisites[0], unit.OutcomeSucceeded))
	first := testutil.RequireReceive(ctx, t, results)
	require.NoError(t, first.err)
	require.Equal(t, dependents[0], first.id)
	require.Equal(t, unit.DecisionRunnable, first.evaluation.Decision)
	select {
	case extra := <-results:
		t.Fatalf("unexpected result for %s", extra.id)
	default:
	}

	// The rest wake one by one as their own prerequisite finishes.
	released := map[unit.ID]bool{dependents[0]: true}
	for i := 1; i < count; i++ {
		require.NoError(t, manager.UpdateOutcome(prerequisites[i], unit.OutcomeFailed))
	}
	for i := 1; i < count; i++ {
		got := testutil.RequireReceive(ctx, t, results)
		require.NoError(t, got.err)
		require.Equal(t, unit.DecisionSkip, got.evaluation.Decision, "unit %s", got.id)
		require.False(t, released[got.id], "unit %s released twice", got.id)
		released[got.id] = true
	}
	require.Len(t, released, count)
}

func conditionalManager(t *testing.T, requirement unit.Requirement) *unit.Manager {
	t.Helper()

	manager := unit.NewManager()
	require.NoError(t, manager.Register(unitA))
	require.NoError(t, manager.Register(unitB))
	require.NoError(t, manager.AddConditionalDependency(unitA, unitB, requirement))
	return manager
}

func conditionalDependencyIDs(dependencies []unit.ConditionalDependency) []unit.ID {
	ids := make([]unit.ID, 0, len(dependencies))
	for _, dependency := range dependencies {
		ids = append(ids, dependency.DependsOn)
	}
	return ids
}
