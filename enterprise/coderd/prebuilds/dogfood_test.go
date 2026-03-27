package prebuilds_test

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	coreprebuilds "github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/coderd/prebuilds/targetexpr"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestDogfoodExpressionAutoscaling(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	clock := quartz.NewMock(t)
	clock.Set(time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC))
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	evaluator := targetexpr.NewEvaluator()

	db, ps := dbtestutil.NewDB(t)
	userID := uuid.New()
	dbgen.User(t, db, database.User{ID: userID})
	org, template := setupTestDBTemplate(t, db, userID, false)
	templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, ps, org.ID, userID, template.ID)
	persistedPreset := setupTestDBPresetWithScheduling(t, db, templateVersionID, 2, "dogfood-expression-autoscaling", "UTC")

	preset := loadDogfoodPresetRow(systemCtx, t, db, template.ID, persistedPreset.ID)
	require.True(t, preset.DesiredInstances.Valid)
	require.EqualValues(t, 2, preset.DesiredInstances.Int32)
	require.Equal(t, "UTC", preset.SchedulingTimezone)

	const baselineExpression = "max(scheduled_target, min(claims_5m * 3, 10))"
	preset.DesiredInstancesExpression = sql.NullString{String: baselineExpression, Valid: true}

	running := make([]database.GetRunningPrebuiltWorkspacesRow, 0)
	nextOrdinal := 1

	phase1Counts := loadDogfoodEventCounts(systemCtx, t, db, clock.Now(), preset.ID)
	phase1Snapshot := newDogfoodPresetSnapshot(preset, running, phase1Counts, evaluator, clock, logger)
	phase1State := phase1Snapshot.CalculateState()
	phase1Actions, err := phase1Snapshot.CalculateActions(time.Minute)
	require.NoError(t, err)
	require.EqualValues(t, 2, phase1State.ScheduledTarget)
	require.EqualValues(t, 2, phase1State.Desired)
	require.Equal(t, "expression", phase1State.TargetSource)
	require.True(t, phase1State.ExpressionPresent)
	require.True(t, phase1State.ExpressionActive)
	require.Zero(t, phase1Counts.ClaimSucceeded.Count5m)
	require.Zero(t, phase1Counts.ClaimMissed.Count5m)
	require.Len(t, phase1Actions, 1)
	require.Equal(t, coreprebuilds.ActionTypeCreate, phase1Actions[0].ActionType)
	require.EqualValues(t, 2, phase1Actions[0].Create)
	logDogfoodPhase(
		t,
		1,
		"Baseline (no demand)",
		baselineExpression,
		"max(2, min(0*3, 10)) = 2",
		running,
		phase1Counts,
		phase1State,
		phase1Actions,
		"Expression correctly returns the scheduled baseline when there are no recent events.",
	)
	running, nextOrdinal = applyDogfoodActions(t, running, preset, phase1Actions, clock, nextOrdinal)
	require.Len(t, running, 2)

	for i := range 4 {
		insertPrebuildEvent(
			systemCtx,
			t,
			db,
			preset.ID,
			coreprebuilds.PrebuildEventClaimSucceeded,
			clock.Now().Add(-time.Duration(i+1)*time.Minute),
		)
	}

	phase2Counts := loadDogfoodEventCounts(systemCtx, t, db, clock.Now(), preset.ID)
	phase2Snapshot := newDogfoodPresetSnapshot(preset, running, phase2Counts, evaluator, clock, logger)
	phase2State := phase2Snapshot.CalculateState()
	phase2Actions, err := phase2Snapshot.CalculateActions(time.Minute)
	require.NoError(t, err)
	require.EqualValues(t, 4, phase2Counts.ClaimSucceeded.Count5m)
	require.Zero(t, phase2Counts.ClaimMissed.Count5m)
	require.EqualValues(t, 10, phase2State.Desired)
	require.Equal(t, "expression", phase2State.TargetSource)
	require.Len(t, phase2Actions, 1)
	require.Equal(t, coreprebuilds.ActionTypeCreate, phase2Actions[0].ActionType)
	require.EqualValues(t, 8, phase2Actions[0].Create)
	logDogfoodPhase(
		t,
		2,
		"Demand spike",
		baselineExpression,
		"max(2, min(4*3, 10)) = 10",
		running,
		phase2Counts,
		phase2State,
		phase2Actions,
		"Expression scales up to the cap after four successful claims arrive inside the 5-minute window.",
	)
	running, nextOrdinal = applyDogfoodActions(t, running, preset, phase2Actions, clock, nextOrdinal)
	require.Len(t, running, 10)

	clock.Advance(6 * time.Minute).MustWait(ctx)

	phase3Counts := loadDogfoodEventCounts(systemCtx, t, db, clock.Now(), preset.ID)
	phase3Snapshot := newDogfoodPresetSnapshot(preset, running, phase3Counts, evaluator, clock, logger)
	phase3State := phase3Snapshot.CalculateState()
	phase3Actions, err := phase3Snapshot.CalculateActions(time.Minute)
	require.NoError(t, err)
	require.Zero(t, phase3Counts.ClaimSucceeded.Count5m)
	require.Zero(t, phase3Counts.ClaimMissed.Count5m)
	require.EqualValues(t, 2, phase3State.Desired)
	require.Equal(t, "expression", phase3State.TargetSource)
	require.Len(t, phase3Actions, 1)
	require.Equal(t, coreprebuilds.ActionTypeDelete, phase3Actions[0].ActionType)
	require.Len(t, phase3Actions[0].DeleteIDs, 8)
	logDogfoodPhase(
		t,
		3,
		"Cooldown (events age out)",
		baselineExpression,
		"max(2, min(0*3, 10)) = 2",
		running,
		phase3Counts,
		phase3State,
		phase3Actions,
		"Once the earlier claims age out of the 5-minute window, reconciliation scales the pool back down to baseline.",
	)
	running, nextOrdinal = applyDogfoodActions(t, running, preset, phase3Actions, clock, nextOrdinal)
	require.Len(t, running, 2)

	preset.DesiredInstancesExpression = sql.NullString{String: "invalid!!!", Valid: true}

	phase4Counts := loadDogfoodEventCounts(systemCtx, t, db, clock.Now(), preset.ID)
	phase4Snapshot := newDogfoodPresetSnapshot(preset, running, phase4Counts, evaluator, clock, logger)
	phase4State := phase4Snapshot.CalculateState()
	phase4Actions, err := phase4Snapshot.CalculateActions(time.Minute)
	require.NoError(t, err)
	require.EqualValues(t, 2, phase4State.Desired)
	require.Equal(t, "expression_fallback", phase4State.TargetSource)
	require.True(t, phase4State.ExpressionPresent)
	require.False(t, phase4State.ExpressionActive)
	require.NotEmpty(t, phase4State.ExpressionError)
	require.Empty(t, phase4Actions)
	logDogfoodPhase(
		t,
		4,
		"Invalid expression fallback",
		preset.DesiredInstancesExpression.String,
		"fallback to scheduled_target = 2",
		running,
		phase4Counts,
		phase4State,
		phase4Actions,
		"Invalid syntax does not block reconciliation; the preset safely falls back to the scheduled baseline.",
	)

	preset.DesiredInstancesExpression = sql.NullString{String: "max(scheduled_target, (claims_5m + misses_5m) * 2)", Valid: true}
	for i := range 3 {
		insertPrebuildEvent(
			systemCtx,
			t,
			db,
			preset.ID,
			coreprebuilds.PrebuildEventClaimMissed,
			clock.Now().Add(-time.Duration(i+1)*time.Minute),
		)
	}

	phase5Counts := loadDogfoodEventCounts(systemCtx, t, db, clock.Now(), preset.ID)
	phase5Snapshot := newDogfoodPresetSnapshot(preset, running, phase5Counts, evaluator, clock, logger)
	phase5State := phase5Snapshot.CalculateState()
	phase5Actions, err := phase5Snapshot.CalculateActions(time.Minute)
	require.NoError(t, err)
	require.Zero(t, phase5Counts.ClaimSucceeded.Count5m)
	require.EqualValues(t, 3, phase5Counts.ClaimMissed.Count5m)
	require.EqualValues(t, 6, phase5State.Desired)
	require.Equal(t, "expression", phase5State.TargetSource)
	require.Len(t, phase5Actions, 1)
	require.Equal(t, coreprebuilds.ActionTypeCreate, phase5Actions[0].ActionType)
	require.EqualValues(t, 4, phase5Actions[0].Create)
	logDogfoodPhase(
		t,
		5,
		"Miss-aware scaling",
		preset.DesiredInstancesExpression.String,
		"max(2, (0 + 3) * 2) = 6",
		running,
		phase5Counts,
		phase5State,
		phase5Actions,
		"Recent claim misses can also drive scale-up when the expression chooses to treat misses as demand pressure.",
	)
	running, _ = applyDogfoodActions(t, running, preset, phase5Actions, clock, nextOrdinal)
	require.Len(t, running, 6)
}

func loadDogfoodPresetRow(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	templateID uuid.UUID,
	presetID uuid.UUID,
) database.GetTemplatePresetsWithPrebuildsRow {
	t.Helper()

	rows, err := db.GetTemplatePresetsWithPrebuilds(ctx, uuid.NullUUID{UUID: templateID, Valid: true})
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	for _, row := range rows {
		if row.ID == presetID {
			return row
		}
	}

	require.Failf(t, "preset not found", "preset %s was not returned by GetTemplatePresetsWithPrebuilds", presetID)
	return database.GetTemplatePresetsWithPrebuildsRow{}
}

func loadDogfoodEventCounts(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	now time.Time,
	presetID uuid.UUID,
) coreprebuilds.PrebuildEventCounts {
	t.Helper()

	rows, err := db.GetPrebuildEventCounts(ctx, database.GetPrebuildEventCountsParams{
		Since5m:   now.Add(-5 * time.Minute),
		Since10m:  now.Add(-10 * time.Minute),
		Since30m:  now.Add(-30 * time.Minute),
		Since60m:  now.Add(-60 * time.Minute),
		Since120m: now.Add(-120 * time.Minute),
	})
	require.NoError(t, err)

	counts := coreprebuilds.PrebuildEventCounts{}
	for _, row := range rows {
		if row.PresetID != presetID {
			continue
		}

		windowed := coreprebuilds.WindowedCounts{
			Count5m:   row.Count5m,
			Count10m:  row.Count10m,
			Count30m:  row.Count30m,
			Count60m:  row.Count60m,
			Count120m: row.Count120m,
		}

		switch coreprebuilds.PrebuildEventType(row.EventType) {
		case coreprebuilds.PrebuildEventClaimSucceeded:
			counts.ClaimSucceeded = windowed
		case coreprebuilds.PrebuildEventClaimMissed:
			counts.ClaimMissed = windowed
		}
	}

	return counts
}

func newDogfoodPresetSnapshot(
	preset database.GetTemplatePresetsWithPrebuildsRow,
	running []database.GetRunningPrebuiltWorkspacesRow,
	eventCounts coreprebuilds.PrebuildEventCounts,
	evaluator targetexpr.Evaluator,
	clock quartz.Clock,
	logger slog.Logger,
) coreprebuilds.PresetSnapshot {
	return coreprebuilds.NewPresetSnapshot(
		preset,
		nil,
		slices.Clone(running),
		nil,
		nil,
		0,
		nil,
		eventCounts,
		false,
		evaluator,
		clock,
		logger,
	)
}

func applyDogfoodActions(
	t *testing.T,
	running []database.GetRunningPrebuiltWorkspacesRow,
	preset database.GetTemplatePresetsWithPrebuildsRow,
	actions []*coreprebuilds.ReconciliationActions,
	clock quartz.Clock,
	nextOrdinal int,
) ([]database.GetRunningPrebuiltWorkspacesRow, int) {
	t.Helper()

	next := slices.Clone(running)
	for _, action := range actions {
		switch action.ActionType {
		case coreprebuilds.ActionTypeCreate:
			require.Positive(t, action.Create)
			for range int(action.Create) {
				next = append(next, database.GetRunningPrebuiltWorkspacesRow{
					ID:                uuid.New(),
					Name:              fmt.Sprintf("dogfood-prebuild-%02d", nextOrdinal),
					TemplateID:        preset.TemplateID,
					TemplateVersionID: preset.TemplateVersionID,
					CurrentPresetID:   uuid.NullUUID{UUID: preset.ID, Valid: true},
					Ready:             true,
					CreatedAt:         clock.Now().Add(time.Duration(nextOrdinal) * time.Second),
				})
				nextOrdinal++
			}
		case coreprebuilds.ActionTypeDelete:
			deleteSet := make(map[uuid.UUID]struct{}, len(action.DeleteIDs))
			for _, id := range action.DeleteIDs {
				deleteSet[id] = struct{}{}
			}

			filtered := make([]database.GetRunningPrebuiltWorkspacesRow, 0, len(next))
			for _, workspace := range next {
				if _, ok := deleteSet[workspace.ID]; ok {
					continue
				}
				filtered = append(filtered, workspace)
			}
			require.Len(t, filtered, len(next)-len(deleteSet))
			next = filtered
		case coreprebuilds.ActionTypeBackoff, coreprebuilds.ActionTypeCancelPending:
			require.Failf(t, "unexpected action", "dogfood scenario does not expect %v actions", action.ActionType)
		default:
			require.Failf(t, "unexpected action", "dogfood scenario received unknown action type %v", action.ActionType)
		}
	}

	return next, nextOrdinal
}

func logDogfoodPhase(
	t *testing.T,
	phase int,
	title string,
	expression string,
	resolvedMath string,
	running []database.GetRunningPrebuiltWorkspacesRow,
	counts coreprebuilds.PrebuildEventCounts,
	state *coreprebuilds.ReconciliationState,
	actions []*coreprebuilds.ReconciliationActions,
	note string,
) {
	t.Helper()

	result := fmt.Sprintf("desired=%d, source=%s", state.Desired, state.TargetSource)
	if state.ExpressionError != "" {
		result = fmt.Sprintf("%s, error=%s", result, state.ExpressionError)
	}

	t.Logf("=== PHASE %d: %s ===", phase, title)
	t.Logf("  Expression: %s", expression)
	t.Logf("  Environment: scheduled_target=%d, running=%d, claims_5m=%d, misses_5m=%d", state.ScheduledTarget, len(running), counts.ClaimSucceeded.Count5m, counts.ClaimMissed.Count5m)
	t.Logf("  Resolved math: %s", resolvedMath)
	t.Logf("  Result: %s", result)
	t.Logf("  Actions: %s", formatDogfoodActions(actions))
	t.Logf("  ✓ %s", note)
}

func formatDogfoodActions(actions []*coreprebuilds.ReconciliationActions) string {
	if len(actions) == 0 {
		return "[no changes]"
	}

	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		switch action.ActionType {
		case coreprebuilds.ActionTypeCreate:
			parts = append(parts, fmt.Sprintf("create %d %s", action.Create, pluralizePrebuild(int(action.Create))))
		case coreprebuilds.ActionTypeDelete:
			parts = append(parts, fmt.Sprintf("delete %d %s", len(action.DeleteIDs), pluralizePrebuild(len(action.DeleteIDs))))
		case coreprebuilds.ActionTypeBackoff:
			parts = append(parts, fmt.Sprintf("back off until %s", action.BackoffUntil.Format(time.RFC3339)))
		case coreprebuilds.ActionTypeCancelPending:
			parts = append(parts, "cancel pending prebuilds")
		default:
			parts = append(parts, fmt.Sprintf("unknown action %d", action.ActionType))
		}
	}

	return fmt.Sprintf("[%s]", strings.Join(parts, ", "))
}

func pluralizePrebuild(n int) string {
	if n == 1 {
		return "prebuild"
	}
	return "prebuilds"
}
