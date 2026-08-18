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

const dependencyLogInterval = 30 * time.Second

type lifecycleExecutor struct {
	clock           quartz.Clock
	logger          slog.Logger
	getScriptLogger func(uuid.UUID) ScriptLogger
	run             func(context.Context, codersdk.WorkspaceAgentScript, ExecuteOption) error
	reportSkipped   func(context.Context, codersdk.WorkspaceAgentScript, ExecuteOption)
}

func (e lifecycleExecutor) execute(ctx context.Context, scripts []codersdk.WorkspaceAgentScript, option ExecuteOption) error {
	manager, scriptsByAddress, err := lifecycleManager(scripts)
	if err != nil {
		return err
	}

	var group errgroup.Group
	for _, script := range scripts {
		group.Go(func() error {
			if script.ResourceAddress == "" {
				return e.run(ctx, script, option)
			}
			return e.executeScript(ctx, manager, scriptsByAddress, script, option)
		})
	}
	return group.Wait()
}

func lifecycleManager(scripts []codersdk.WorkspaceAgentScript) (*unit.Manager, map[string]codersdk.WorkspaceAgentScript, error) {
	manager := unit.NewManager()
	scriptsByAddress := make(map[string]codersdk.WorkspaceAgentScript, len(scripts))
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

func (e lifecycleExecutor) executeScript(
	ctx context.Context,
	manager *unit.Manager,
	scriptsByAddress map[string]codersdk.WorkspaceAgentScript,
	script codersdk.WorkspaceAgentScript,
	option ExecuteOption,
) error {
	evaluation, err := e.waitForDecision(ctx, manager, script)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	switch evaluation.Decision {
	case unit.DecisionSkipped:
		e.logSkipped(ctx, script, scriptsByAddress, evaluation)
		if err := manager.UpdateOutcome(unit.ID(script.ResourceAddress), unit.OutcomeSkipped); err != nil {
			return xerrors.Errorf("mark script %q skipped: %w", script.ResourceAddress, err)
		}
		if e.reportSkipped != nil {
			e.reportSkipped(ctx, script, option)
		}
		return nil
	case unit.DecisionRunnable:
		if err := manager.UpdateOutcome(unit.ID(script.ResourceAddress), unit.OutcomeRunning); err != nil {
			return xerrors.Errorf("mark script %q running: %w", script.ResourceAddress, err)
		}
	default:
		return xerrors.Errorf("script %q returned unexpected dependency decision %q", script.ResourceAddress, evaluation.Decision)
	}

	runErr := e.run(ctx, script, option)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return errors.Join(runErr, ctxErr)
	}
	if err := manager.UpdateOutcome(unit.ID(script.ResourceAddress), scriptOutcome(runErr)); err != nil {
		return errors.Join(runErr, xerrors.Errorf("record script %q outcome: %w", script.ResourceAddress, err))
	}
	return runErr
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
	decision := make(chan result, 1)
	go func() {
		evaluation, err := manager.WaitForDecision(ctx, id)
		decision <- result{evaluation: evaluation, err: err}
	}()

	for {
		select {
		case <-ctx.Done():
			return unit.Evaluation{}, ctx.Err()
		case result := <-decision:
			return result.evaluation, result.err
		case <-ticker.C:
			evaluation, err := manager.Evaluate(id)
			if err != nil {
				return unit.Evaluation{}, err
			}
			if evaluation.Decision == unit.DecisionWaiting {
				e.logWaiting(ctx, script, evaluation.UnmetDependencies)
			}
		}
	}
}

func (e lifecycleExecutor) logWaiting(
	ctx context.Context,
	script codersdk.WorkspaceAgentScript,
	dependencies []unit.ConditionalDependency,
) {
	quotedAddresses := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		quotedAddresses = append(quotedAddresses, strconv.Quote(string(dependency.DependsOn)))
	}
	e.sendScriptLog(ctx, script, fmt.Sprintf(
		"Waiting for dependencies: %s... (30s)",
		strings.Join(quotedAddresses, ", "),
	))
}

func (e lifecycleExecutor) logSkipped(
	ctx context.Context,
	script codersdk.WorkspaceAgentScript,
	scriptsByAddress map[string]codersdk.WorkspaceAgentScript,
	evaluation unit.Evaluation,
) {
	for _, dependency := range evaluation.UnmetDependencies {
		if dependency.Requirement != unit.RequirementSuccess || !dependency.CurrentOutcome.Terminal() {
			continue
		}
		prerequisite := scriptsByAddress[string(dependency.DependsOn)]
		name := prerequisite.DisplayName
		if name == "" {
			name = prerequisite.ResourceAddress
		}
		if dependency.CurrentOutcome == unit.OutcomeSkipped {
			e.sendScriptLog(ctx, script, fmt.Sprintf("Skipping script: dependency %q was skipped.", name))
		} else {
			e.sendScriptLog(ctx, script, fmt.Sprintf("Skipping script: dependency %q failed.", name))
		}
		return
	}
}

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

func scriptOutcome(err error) unit.Outcome {
	switch {
	case err == nil:
		return unit.OutcomeSucceeded
	case errors.Is(err, ErrTimeout):
		return unit.OutcomeTimedOut
	default:
		return unit.OutcomeFailed
	}
}
