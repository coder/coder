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
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// dependencyLogInterval is how often a waiting script's log gets a summary
// line. The RFC fixes it at 30 seconds.
const dependencyLogInterval = 30 * time.Second

// lifecycleExecutor runs one start or stop execution whose scripts carry
// dependencies. The unit manager decides when each script runs, is skipped
// or keeps waiting; the executor only starts scripts, records outcomes and
// writes the two diagnostic log lines.
type lifecycleExecutor struct {
	clock           quartz.Clock
	logger          slog.Logger
	getScriptLogger func(uuid.UUID) ScriptLogger
	// run is Runner.runTracked in production. Tests substitute a fake so no
	// process starts.
	run func(context.Context, codersdk.WorkspaceAgentScript, ExecuteOption) error
}

func (e lifecycleExecutor) execute(ctx context.Context, scripts []codersdk.WorkspaceAgentScript, option ExecuteOption) error {
	manager, scriptsByAddress, err := lifecycleManager(scripts)
	if err != nil {
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

	// Every addressed script is registered first, because a prerequisite
	// with no dependencies of its own still needs a unit for its
	// dependents' edges to point at.
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

// lifecycleRequirement is a switch rather than a cast so a requirement word
// the manager does not know cannot pass through as a third kind of edge.
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
// script and records exactly one terminal outcome for it. Each goroutine
// records only its own unit, so no unit is updated twice.
func (e lifecycleExecutor) executeScript(
	ctx context.Context,
	manager *unit.Manager,
	scriptsByAddress map[string]codersdk.WorkspaceAgentScript,
	script codersdk.WorkspaceAgentScript,
	option ExecuteOption,
) error {
	id := unit.ID(script.ResourceAddress)

	evaluation, err := e.waitForDecision(ctx, manager, script)
	if err == nil {
		// Nothing starts after cancellation, even if the decision arrived
		// in the same instant.
		err = ctx.Err()
	}
	if err != nil {
		if ctx.Err() != nil {
			return errors.Join(err, recordOutcome(manager, id, unit.OutcomeCanceled))
		}
		return err
	}

	switch evaluation.Decision {
	case unit.DecisionSkip:
		e.logSkipped(ctx, script, scriptsByAddress, evaluation)
		if err := recordOutcome(manager, id, unit.OutcomeSkipped); err != nil {
			return err
		}
		// A skip is not an error. The prerequisite's own failure is what
		// sets the lifecycle state.
		return nil
	case unit.DecisionRunnable:
		if err := recordOutcome(manager, id, unit.OutcomeRunning); err != nil {
			return err
		}
	default:
		return xerrors.Errorf("script %q returned unexpected dependency decision %q", script.ResourceAddress, evaluation.Decision)
	}

	runErr := e.run(ctx, script, option)
	if err := recordOutcome(manager, id, scriptOutcome(runErr, ctx.Err())); err != nil {
		return errors.Join(runErr, err)
	}
	return runErr
}

// recordOutcome wraps rather than ignores the manager's error so a unit
// that is updated twice shows up as a failure instead of a hang.
func recordOutcome(manager *unit.Manager, id unit.ID, outcome unit.Outcome) error {
	if err := manager.UpdateOutcome(id, outcome); err != nil {
		return xerrors.Errorf("record script %q outcome %q: %w", id, outcome, err)
	}
	return nil
}

// waitForDecision blocks until the manager says runnable or skip, or ctx
// ends, writing one summary line per interval to the script's log.
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
// succeed at the moment of the decision. A prerequisite still pending or
// running did not cause the skip and is not named.
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

// scriptOutcome uses a fixed rule so a script that finishes in the same
// instant the context is canceled is recorded the same way whichever
// goroutine ran first. A script's own timeout counts as timed out even
// during cancellation; any other error during cancellation is canceled.
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
