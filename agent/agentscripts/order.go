package agentscripts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// scriptOrderWaitLogInterval is how often a blocked ordered script logs
// a "waiting for dependencies" line to its own log source.
const scriptOrderWaitLogInterval = 30 * time.Second

// scriptOutcome is the terminal result of an ordered start script run.
// Dependent scripts consult it to decide whether they should run or be
// skipped.
type scriptOutcome string

const (
	outcomeSuccess scriptOutcome = "success"
	outcomeFailure scriptOutcome = "failure"
	outcomeSkipped scriptOutcome = "skipped"
)

// describe returns the outcome as a verb phrase for log messages.
func (o scriptOutcome) describe() string {
	switch o {
	case outcomeFailure:
		return "failed"
	case outcomeSkipped:
		return "was skipped"
	default:
		return string(o)
	}
}

// scriptUnitID namespaces template-declared script units in the unit
// manager so they cannot collide with units created via
// "coder exp sync".
func scriptUnitID(id uuid.UUID) unit.ID {
	return unit.ID("tf:" + id.String())
}

// initScriptOrder registers start scripts that participate in
// coder_script_order rules with the unit manager and adds their
// dependency edges. Scripts without rules are not registered and run
// ungated. Called at most once per runner via orderOnce.
func (r *Runner) initScriptOrder() error {
	startScripts := make(map[uuid.UUID]struct{})
	for _, script := range r.scripts {
		if script.RunOnStart && script.ID != uuid.Nil {
			startScripts[script.ID] = struct{}{}
		}
	}

	type orderEdge struct {
		dependent uuid.UUID
		dependsOn uuid.UUID
	}
	var edges []orderEdge
	participants := make(map[uuid.UUID]struct{})
	for _, script := range r.scripts {
		if !script.RunOnStart || script.ID == uuid.Nil {
			continue
		}
		for _, dep := range script.OrderDependencies {
			_, isStartScript := startScripts[dep.ScriptID]
			if !isStartScript || dep.ScriptID == script.ID {
				// The dependency target does not run as a start script in
				// this runner (for example, a devcontainer script extracted
				// from the list). The edge would never be satisfied, so it
				// is dropped instead of deadlocking the dependent script.
				r.Logger.Warn(context.Background(), "ignoring script order rule with unsatisfiable dependency",
					slog.F("script_id", script.ID),
					slog.F("depends_on_script_id", dep.ScriptID),
				)
				continue
			}
			edges = append(edges, orderEdge{dependent: script.ID, dependsOn: dep.ScriptID})
			participants[script.ID] = struct{}{}
			participants[dep.ScriptID] = struct{}{}
		}
	}
	if len(edges) == 0 {
		return nil
	}

	for id := range participants {
		err := r.UnitManager.Register(scriptUnitID(id))
		if err != nil && !errors.Is(err, unit.ErrUnitAlreadyRegistered) {
			return xerrors.Errorf("register script unit %q: %w", id, err)
		}
	}
	// Both requires=success and requires=completion gate on the
	// dependency reaching a terminal state (StatusComplete). The
	// recorded outcome decides whether a requires=success dependent
	// runs or is skipped.
	for _, edge := range edges {
		err := r.UnitManager.AddDependency(scriptUnitID(edge.dependent), scriptUnitID(edge.dependsOn), unit.StatusComplete)
		if err != nil {
			return xerrors.Errorf("add script order dependency %q -> %q: %w", edge.dependent, edge.dependsOn, err)
		}
	}
	r.orderParticipants = participants
	return nil
}

// runOrdered executes a start script that participates in script order
// rules. It waits for all dependencies to reach a terminal state, skips
// the script when a requires=success dependency did not succeed, and
// records the outcome so dependent scripts can make the same decision.
func (r *Runner) runOrdered(ctx context.Context, script codersdk.WorkspaceAgentScript, option ExecuteOption) error {
	unitID := scriptUnitID(script.ID)

	if err := r.waitScriptOrderReady(ctx, script); err != nil {
		return xerrors.Errorf("wait for script order dependencies %q: %w", script.LogSourceID, err)
	}

	if reason := r.scriptOrderSkipReason(script); reason != "" {
		r.recordScriptOutcome(script.ID, outcomeSkipped)
		r.logSkippedScript(ctx, script, reason)
		r.reportSkippedTiming(ctx, script)
		r.completeScriptUnit(ctx, unitID)
		// A skipped script is not a runner failure; the failed dependency
		// already reported its own error.
		return nil
	}

	if err := r.UnitManager.UpdateStatus(unitID, unit.StatusStarted); err != nil {
		r.Logger.Warn(ctx, "mark script unit started", slog.F("unit", unitID), slog.Error(err))
	}
	err := r.trackRun(ctx, script, option)
	if err != nil {
		r.recordScriptOutcome(script.ID, outcomeFailure)
	} else {
		r.recordScriptOutcome(script.ID, outcomeSuccess)
	}
	// The unit completes even on failure so that dependents unblock and
	// decide via the recorded outcome.
	r.completeScriptUnit(ctx, unitID)
	if err != nil {
		return xerrors.Errorf("run agent script %q: %w", script.LogSourceID, err)
	}
	return nil
}

// waitScriptOrderReady blocks until all dependencies of the script's
// unit reach a terminal state, the context is canceled, or the runner
// is closed. While blocked it periodically logs the dependencies it is
// still waiting for to the script's log source.
func (r *Runner) waitScriptOrderReady(ctx context.Context, script codersdk.WorkspaceAgentScript) error {
	unitID := scriptUnitID(script.ID)
	start := r.Clock.Now()
	ticker := r.Clock.NewTicker(scriptOrderWaitLogInterval, "agentscripts", "scriptOrderWait")
	defer ticker.Stop()
	for {
		// Subscribe before checking readiness so a status change between
		// the check and the wait is not missed.
		changed := r.UnitManager.Watch()
		ready, err := r.UnitManager.IsReady(unitID)
		if err != nil {
			return xerrors.Errorf("check readiness of unit %q: %w", unitID, err)
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.closed:
			return xerrors.New("runner closed")
		case <-ticker.C:
			r.logWaitingScript(ctx, script, r.Clock.Now().Sub(start))
		case <-changed:
		}
	}
}

// scriptOrderSkipReason returns a human-readable reason when any
// requires=success dependency did not succeed, or an empty string when
// the script should run. requires=completion dependencies never cause
// skips; they only delay execution until the dependency is terminal.
func (r *Runner) scriptOrderSkipReason(script codersdk.WorkspaceAgentScript) string {
	for _, dep := range script.OrderDependencies {
		if dep.Requires == codersdk.WorkspaceAgentScriptOrderRequiresCompletion {
			continue
		}
		outcome, ok := r.scriptOutcome(dep.ScriptID)
		if !ok || outcome == outcomeSuccess {
			// A missing outcome means the rule was dropped as
			// unsatisfiable during initScriptOrder; the script still
			// runs in that case.
			continue
		}
		return fmt.Sprintf("dependency %q %s", r.scriptDisplayName(dep.ScriptID), outcome.describe())
	}
	return ""
}

func (r *Runner) recordScriptOutcome(id uuid.UUID, outcome scriptOutcome) {
	r.outcomesMu.Lock()
	defer r.outcomesMu.Unlock()
	r.outcomes[id] = outcome
}

func (r *Runner) scriptOutcome(id uuid.UUID) (scriptOutcome, bool) {
	r.outcomesMu.Lock()
	defer r.outcomesMu.Unlock()
	outcome, ok := r.outcomes[id]
	return outcome, ok
}

// completeScriptUnit marks the unit terminal so dependent scripts
// unblock.
func (r *Runner) completeScriptUnit(ctx context.Context, unitID unit.ID) {
	if err := r.UnitManager.UpdateStatus(unitID, unit.StatusComplete); err != nil {
		r.Logger.Warn(ctx, "mark script unit complete", slog.F("unit", unitID), slog.Error(err))
	}
}

func (r *Runner) scriptDisplayName(id uuid.UUID) string {
	for _, script := range r.scripts {
		if script.ID == id {
			if script.DisplayName != "" {
				return script.DisplayName
			}
			return script.LogSourceID.String()
		}
	}
	return id.String()
}

// logSkippedScript surfaces the skip in both the agent log and the
// script's own log source so it is visible in the workspace UI.
func (r *Runner) logSkippedScript(ctx context.Context, script codersdk.WorkspaceAgentScript, reason string) {
	r.Logger.Warn(ctx, "skipping agent script",
		slog.F("log_source_id", script.LogSourceID),
		slog.F("display_name", script.DisplayName),
		slog.F("reason", reason),
	)
	scriptLogger := r.GetScriptLogger(script.LogSourceID)
	err := scriptLogger.Send(ctx, agentsdk.Log{
		CreatedAt: dbtime.Now(),
		Output:    fmt.Sprintf("Skipping script: %s.", reason),
		Level:     codersdk.LogLevelWarn,
	})
	if err != nil {
		r.Logger.Warn(ctx, "send script skip log", slog.Error(err))
		return
	}
	if err := scriptLogger.Flush(ctx); err != nil {
		r.Logger.Warn(ctx, "flush script skip log", slog.Error(err))
	}
}

// reportSkippedTiming reports a zero-duration timing with the SKIPPED
// status so the skip shows up in the workspace build timeline.
func (r *Runner) reportSkippedTiming(ctx context.Context, script codersdk.WorkspaceAgentScript) {
	if r.scriptCompleted == nil {
		r.Logger.Debug(ctx, "r.scriptCompleted unexpectedly nil")
		return
	}
	now := dbtime.Now()
	reportCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := r.scriptCompleted(reportCtx, &proto.WorkspaceAgentScriptCompletedRequest{
		Timing: &proto.Timing{
			ScriptId: script.ID[:],
			Start:    timestamppb.New(now),
			End:      timestamppb.New(now),
			ExitCode: -1,
			Stage:    proto.Timing_START,
			Status:   proto.Timing_SKIPPED,
		},
	})
	if err != nil {
		r.Logger.Warn(ctx, "reporting script skipped", slog.Error(err))
	}
}

// logWaitingScript writes a periodic progress line to the script's log
// source naming the dependencies that have not reached a terminal state
// yet.
func (r *Runner) logWaitingScript(ctx context.Context, script codersdk.WorkspaceAgentScript, elapsed time.Duration) {
	var pending []string
	for _, dep := range script.OrderDependencies {
		if _, participates := r.orderParticipants[dep.ScriptID]; !participates {
			// The rule was dropped as unsatisfiable during
			// initScriptOrder and never blocks this script.
			continue
		}
		if _, done := r.scriptOutcome(dep.ScriptID); done {
			continue
		}
		pending = append(pending, fmt.Sprintf("%q", r.scriptDisplayName(dep.ScriptID)))
	}
	if len(pending) == 0 {
		return
	}
	r.Logger.Debug(ctx, "agent script waiting for dependencies",
		slog.F("log_source_id", script.LogSourceID),
		slog.F("display_name", script.DisplayName),
		slog.F("pending", strings.Join(pending, ", ")),
		slog.F("elapsed", elapsed),
	)
	scriptLogger := r.GetScriptLogger(script.LogSourceID)
	err := scriptLogger.Send(ctx, agentsdk.Log{
		CreatedAt: dbtime.Now(),
		Output:    fmt.Sprintf("Waiting for %s... (%s)", strings.Join(pending, ", "), elapsed.Truncate(time.Second)),
		Level:     codersdk.LogLevelInfo,
	})
	if err != nil {
		r.Logger.Warn(ctx, "send script waiting log", slog.Error(err))
		return
	}
	if err := scriptLogger.Flush(ctx); err != nil {
		r.Logger.Warn(ctx, "flush script waiting log", slog.Error(err))
	}
}
