package unit_test

import (
	"context"
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
		err = manager.AddConditionalDependency(unitA, unitB, "unknown")
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
	})
}

func TestManager_UpdateOutcome(t *testing.T) {
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

	err = manager.UpdateOutcome(unitA, unit.OutcomeRunning)
	require.ErrorIs(t, err, unit.ErrSameOutcomeAlreadySet)
	err = manager.UpdateOutcome(unitA, "unknown")
	require.ErrorIs(t, err, unit.ErrInvalidOutcome)
	err = manager.UpdateOutcome(unitB, unit.OutcomeRunning)
	require.ErrorIs(t, err, unit.ErrUnitNotFound)
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
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.terminal, test.outcome.Terminal())
		})
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

	t.Run("SuccessRequiresSucceeded", func(t *testing.T) {
		t.Parallel()

		manager := conditionalManager(t, unit.RequirementSuccess)

		evaluation, err := manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionWaiting, evaluation.Decision)
		require.Equal(t, []unit.ConditionalDependency{{
			Unit:           unitA,
			DependsOn:      unitB,
			Requirement:    unit.RequirementSuccess,
			CurrentOutcome: unit.OutcomePending,
		}}, evaluation.UnmetDependencies)

		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeSucceeded))
		evaluation, err = manager.Evaluate(unitA)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionRunnable, evaluation.Decision)
		require.Empty(t, evaluation.UnmetDependencies)
	})

	for _, outcome := range []unit.Outcome{
		unit.OutcomeFailed,
		unit.OutcomeTimedOut,
		unit.OutcomeSkipped,
	} {
		t.Run("SuccessSkipsAfter"+string(outcome), func(t *testing.T) {
			t.Parallel()

			manager := conditionalManager(t, unit.RequirementSuccess)
			require.NoError(t, manager.UpdateOutcome(unitB, outcome))

			evaluation, err := manager.Evaluate(unitA)
			require.NoError(t, err)
			require.Equal(t, unit.DecisionSkipped, evaluation.Decision)
			require.Equal(t, outcome, evaluation.UnmetDependencies[0].CurrentOutcome)
		})
	}

	for _, outcome := range []unit.Outcome{
		unit.OutcomeSucceeded,
		unit.OutcomeFailed,
		unit.OutcomeTimedOut,
		unit.OutcomeSkipped,
	} {
		t.Run("CompletionRunsAfter"+string(outcome), func(t *testing.T) {
			t.Parallel()

			manager := conditionalManager(t, unit.RequirementCompletion)
			require.NoError(t, manager.UpdateOutcome(unitB, outcome))

			evaluation, err := manager.Evaluate(unitA)
			require.NoError(t, err)
			require.Equal(t, unit.DecisionRunnable, evaluation.Decision)
			require.Empty(t, evaluation.UnmetDependencies)
		})
	}

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
		require.Equal(t, unit.DecisionSkipped, evaluation.Decision)
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
		require.Equal(t, unit.DecisionSkipped, evaluation.Decision)
		require.NoError(t, manager.UpdateOutcome(unitB, unit.OutcomeSkipped))

		evaluation, err = manager.Evaluate(unitC)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionRunnable, evaluation.Decision)
		evaluation, err = manager.Evaluate(unitD)
		require.NoError(t, err)
		require.Equal(t, unit.DecisionSkipped, evaluation.Decision)
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

	t.Run("WakesWhenRunnable", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		manager := conditionalManager(t, unit.RequirementSuccess)
		result := make(chan unit.Evaluation, 1)
		errCh := make(chan error, 1)
		started := make(chan struct{})
		go func() {
			close(started)
			evaluation, err := manager.WaitForDecision(ctx, unitA)
			result <- evaluation
			errCh <- err
		}()
		<-started

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
		require.Equal(t, unit.DecisionSkipped, testutil.RequireReceive(ctx, t, result).Decision)
		require.NoError(t, testutil.RequireReceive(ctx, t, errCh))
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		manager := conditionalManager(t, unit.RequirementSuccess)

		_, err := manager.WaitForDecision(ctx, unitA)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("UnregisteredUnit", func(t *testing.T) {
		t.Parallel()

		manager := unit.NewManager()
		_, err := manager.WaitForDecision(context.Background(), unitA)
		require.ErrorIs(t, err, unit.ErrUnitNotFound)
	})
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
