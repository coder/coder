package agentscripts

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/quartz"
)

// Often enough to show the wait is alive, rare enough not to flood the log.
const dependencyLogInterval = 30 * time.Second

const (
	canceledBeforeStartLine = "Canceled before starting."
	canceledRunningLine     = "Canceled while running."
)

// lifecycleExecutor runs one start or stop execution whose scripts carry
// dependencies.
type lifecycleExecutor struct {
	clock           quartz.Clock
	logger          slog.Logger
	getScriptLogger func(uuid.UUID) ScriptLogger
	// run is Runner.runTracked; tests replace it with a fake.
	run func(context.Context, codersdk.WorkspaceAgentScript, ExecuteOption) error
	// reportUnstarted is Runner.reportUnstarted; tests replace it with a
	// fake or leave it nil.
	reportUnstarted func(context.Context, codersdk.WorkspaceAgentScript, ExecuteOption, proto.Timing_Status)
}

func (e lifecycleExecutor) execute(ctx context.Context, scripts []codersdk.WorkspaceAgentScript, option ExecuteOption) error {
	manager, scriptsByAddress, err := lifecycleManager(scripts)
	if err != nil {
		// The agent log is not visible in the dashboard.
		for _, script := range scripts {
			e.sendScriptLog(ctx, script, "Lifecycle scripts did not run: "+err.Error())
		}
		return err
	}
	return e.executeWith(ctx, manager, scriptsByAddress, scripts, option)
}

// executeWith is separate from execute so a test can keep the manager and
// read every unit's outcome after the execution ends.
func (e lifecycleExecutor) executeWith(
	ctx context.Context,
	manager *unit.Manager,
	scriptsByAddress map[string]codersdk.WorkspaceAgentScript,
	scripts []codersdk.WorkspaceAgentScript,
	option ExecuteOption,
) error {
	var group errgroup.Group
	for _, script := range scripts {
		group.Go(func() error {
			// A script without an address is not a unit and nothing can
			// wait for it, so it starts at once.
			if script.ResourceAddress == "" {
				return e.run(ctx, script, option)
			}
			return e.executeScript(ctx, manager, scriptsByAddress, script, option)
		})
	}
	return group.Wait()
}

// lifecycleManager builds the manager for one execution and checks the
// whole graph before any script starts, so an inconsistent dependency list
// fails with nothing half done.
func lifecycleManager(scripts []codersdk.WorkspaceAgentScript) (*unit.Manager, map[string]codersdk.WorkspaceAgentScript, error) {
	manager := unit.NewManager()
	scriptsByAddress := make(map[string]codersdk.WorkspaceAgentScript, len(scripts))

	// Both ends of an edge must be registered before the edge is added, and
	// a prerequisite can come after its dependent in the list.
	for _, script := range scripts {
		if script.ResourceAddress == "" {
			if len(script.Dependencies) > 0 {
				return nil, nil, xerrors.Errorf("script %q has dependencies but no Terraform resource address", script.DisplayName)
			}
			continue
		}
		if _, exists := scriptsByAddress[script.ResourceAddress]; exists {
			return nil, nil, xerrors.Errorf("duplicate script resource address %q", script.ResourceAddress)
		}
		scriptsByAddress[script.ResourceAddress] = script
		if err := manager.Register(unit.ID(script.ResourceAddress)); err != nil {
			return nil, nil, xerrors.Errorf("register script %q: %w", script.ResourceAddress, err)
		}
	}

	for _, script := range scripts {
		for _, dependency := range script.Dependencies {
			if _, exists := scriptsByAddress[dependency.PrerequisiteResourceAddress]; !exists {
				return nil, nil, xerrors.Errorf(
					"script %q depends on %q, which is not part of this lifecycle execution",
					script.ResourceAddress, dependency.PrerequisiteResourceAddress,
				)
			}
			requirement, err := lifecycleRequirement(dependency.Requirement)
			if err != nil {
				return nil, nil, xerrors.Errorf(
					"script %q dependency on %q: %w",
					script.ResourceAddress, dependency.PrerequisiteResourceAddress, err,
				)
			}
			if err := manager.AddConditionalDependency(
				unit.ID(script.ResourceAddress),
				unit.ID(dependency.PrerequisiteResourceAddress),
				requirement,
			); err != nil {
				return nil, nil, xerrors.Errorf(
					"add script dependency %q after %q: %w",
					script.ResourceAddress, dependency.PrerequisiteResourceAddress, err,
				)
			}
		}
	}
	return manager, scriptsByAddress, nil
}

// lifecycleRequirement uses a switch rather than a cast so the codersdk and
// unit requirement strings can change independently.
func lifecycleRequirement(requirement codersdk.WorkspaceAgentScriptDependencyRequirement) (unit.Requirement, error) {
	switch requirement {
	case codersdk.WorkspaceAgentScriptDependencyRequirementSuccess:
		return unit.RequirementSuccess, nil
	case codersdk.WorkspaceAgentScriptDependencyRequirementCompletion:
		return unit.RequirementCompletion, nil
	default:
		return "", xerrors.Errorf("unsupported requirement %q", requirement)
	}
}

// executeScript waits for the manager's decision, then skips or runs the
// script. Every exit records one terminal outcome, because dependents only
// wake on one and the execution would otherwise stay open until the agent
// shuts down. Only this goroutine writes its unit's outcome.
func (e lifecycleExecutor) executeScript(
	ctx context.Context,
	manager *unit.Manager,
	scriptsByAddress map[string]codersdk.WorkspaceAgentScript,
	script codersdk.WorkspaceAgentScript,
	option ExecuteOption,
) (err error) {
	id := unit.ID(script.ResourceAddress)

	terminal := false
	record := func(outcome unit.Outcome) error {
		if err := recordOutcome(manager, id, outcome); err != nil {
			return err
		}
		if outcome.Terminal() {
			terminal = true
		}
		return nil
	}
	defer func() {
		if !terminal {
			err = errors.Join(err, recordOutcome(manager, id, unit.OutcomeFailed))
		}
	}()

	evaluation, err := e.waitForDecision(ctx, manager, script)
	if err == nil {
		// Nothing starts after cancellation, even if the decision arrived
		// in the same instant.
		err = ctx.Err()
	}
	if err != nil {
		if ctx.Err() != nil {
			e.logCanceled(ctx, script, canceledBeforeStartLine)
			e.report(ctx, script, option, proto.Timing_CANCELED)
			return errors.Join(err, record(unit.OutcomeCanceled))
		}
		return err
	}

	switch evaluation.Decision {
	case unit.DecisionSkip:
		e.logSkipped(ctx, script, scriptsByAddress, evaluation)
		e.report(ctx, script, option, proto.Timing_SKIPPED)
		if err := record(unit.OutcomeSkipped); err != nil {
			return err
		}
		// The prerequisite's own error already sets the lifecycle state.
		return nil
	case unit.DecisionRunnable:
		if err := record(unit.OutcomeRunning); err != nil {
			return err
		}
	default:
		return xerrors.Errorf("script %q returned unexpected dependency decision %q", script.ResourceAddress, evaluation.Decision)
	}

	runErr := e.run(ctx, script, option)
	outcome := scriptOutcome(runErr, ctx.Err())
	if outcome == unit.OutcomeCanceled {
		// run already sent the timing with the real interval.
		e.logCanceled(ctx, script, canceledRunningLine)
	}
	if err := record(outcome); err != nil {
		return errors.Join(runErr, err)
	}
	return runErr
}

func (e lifecycleExecutor) report(ctx context.Context, script codersdk.WorkspaceAgentScript, option ExecuteOption, status proto.Timing_Status) {
	if e.reportUnstarted != nil {
		e.reportUnstarted(ctx, script, option, status)
	}
}

// logCanceled uses a context that outlives the canceled execution, because
// sending through the execution's context would fail at exactly this moment.
func (e lifecycleExecutor) logCanceled(ctx context.Context, script codersdk.WorkspaceAgentScript, line string) {
	logCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reportTimeout)
	defer cancel()
	e.sendScriptLog(logCtx, script, line)
}

func recordOutcome(manager *unit.Manager, id unit.ID, outcome unit.Outcome) error {
	if err := manager.UpdateOutcome(id, outcome); err != nil {
		return xerrors.Errorf("record script %q outcome %q: %w", id, outcome, err)
	}
	return nil
}

func (e lifecycleExecutor) waitForDecision(
	ctx context.Context,
	manager *unit.Manager,
	script codersdk.WorkspaceAgentScript,
) (unit.Evaluation, error) {
	id := unit.ID(script.ResourceAddress)
	evaluation, err := manager.Evaluate(id)
	if err != nil || evaluation.Decision != unit.DecisionWaiting {
		return evaluation, err
	}

	ticker := e.clock.NewTicker(dependencyLogInterval, "agent_scripts", "dependency_wait")
	defer ticker.Stop("agent_scripts", "dependency_wait")

	type result struct {
		evaluation unit.Evaluation
		err        error
	}
	// Buffered so the goroutine can deliver after this function has
	// already returned on ctx.Done.
	decision := make(chan result, 1)
	go func() {
		evaluation, err := manager.WaitForDecision(ctx, id)
		decision <- result{evaluation: evaluation, err: err}
	}()

	var waited time.Duration
	for {
		select {
		case <-ctx.Done():
			return unit.Evaluation{}, ctx.Err()
		case result := <-decision:
			return result.evaluation, result.err
		case <-ticker.C:
			waited += dependencyLogInterval
			// Re-evaluate so prerequisites that finished since the wait
			// began drop out of the line.
			evaluation, err := manager.Evaluate(id)
			if err != nil {
				return unit.Evaluation{}, err
			}
			if evaluation.Decision == unit.DecisionWaiting {
				e.logWaiting(ctx, script, evaluation.UnmetDependencies, waited)
			}
		}
	}
}

func (e lifecycleExecutor) logWaiting(
	ctx context.Context,
	script codersdk.WorkspaceAgentScript,
	dependencies []unit.ConditionalDependency,
	waited time.Duration,
) {
	// The manager already sorts the unmet list by prerequisite address.
	quoted := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		quoted = append(quoted, strconv.Quote(string(dependency.DependsOn)))
	}
	e.sendScriptLog(ctx, script, fmt.Sprintf(
		"Waiting for dependencies: %s (%ds)",
		strings.Join(quoted, ", "), int(waited.Seconds()),
	))
}

// logSkipped names every direct success prerequisite that can no longer
// succeed at the moment of the decision.
func (e lifecycleExecutor) logSkipped(
	ctx context.Context,
	script codersdk.WorkspaceAgentScript,
	scriptsByAddress map[string]codersdk.WorkspaceAgentScript,
	evaluation unit.Evaluation,
) {
	var causes []string
	for _, dependency := range evaluation.UnmetDependencies {
		if dependency.Requirement != unit.RequirementSuccess {
			continue
		}
		how, impossible := skipCause(dependency.CurrentOutcome)
		if !impossible {
			continue
		}
		prerequisite := scriptsByAddress[string(dependency.DependsOn)]
		name := prerequisite.DisplayName
		if name == "" {
			name = prerequisite.ResourceAddress
		}
		causes = append(causes, fmt.Sprintf("dependency %q %s", name, how))
	}
	if len(causes) == 0 {
		// A skip decision always has at least one impossible edge, so this
		// means the manager's rules and skipCause disagree.
		e.logger.Warn(ctx, "skip decision with no impossible dependency", slog.F("resource_address", script.ResourceAddress))
		return
	}
	e.sendScriptLog(ctx, script, "Skipping script: "+strings.Join(causes, ", ")+".")
}

// skipCause mirrors unit.Requirement.impossible, which is not exported.
func skipCause(outcome unit.Outcome) (string, bool) {
	switch outcome {
	case unit.OutcomeFailed:
		return "failed", true
	case unit.OutcomeTimedOut:
		return "timed out", true
	case unit.OutcomeSkipped:
		return "was skipped", true
	default:
		return "", false
	}
}

// sendScriptLog flushes after sending so the line is visible while the
// script is still waiting. A failed send never changes a decision.
func (e lifecycleExecutor) sendScriptLog(ctx context.Context, script codersdk.WorkspaceAgentScript, output string) {
	if e.getScriptLogger == nil {
		return
	}
	logger := e.getScriptLogger(script.LogSourceID)
	if err := logger.Send(ctx, agentsdk.Log{
		CreatedAt: e.clock.Now("agent_scripts", "dependency_log").UTC(),
		Output:    output,
		Level:     codersdk.LogLevelInfo,
	}); err != nil {
		e.logger.Warn(ctx, "send script dependency log", slog.F("resource_address", script.ResourceAddress), slog.Error(err))
		return
	}
	if err := logger.Flush(ctx); err != nil {
		e.logger.Warn(ctx, "flush script dependency log", slog.F("resource_address", script.ResourceAddress), slog.Error(err))
	}
}

// scriptOutcome keeps success and the script's own timeout even when the
// context is canceled, because cancellation did not cause them.
func scriptOutcome(runErr error, ctxErr error) unit.Outcome {
	switch {
	case runErr == nil:
		return unit.OutcomeSucceeded
	case errors.Is(runErr, ErrTimeout):
		return unit.OutcomeTimedOut
	case ctxErr != nil:
		return unit.OutcomeCanceled
	default:
		return unit.OutcomeFailed
	}
}
