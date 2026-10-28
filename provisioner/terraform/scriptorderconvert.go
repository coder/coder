package terraform

import (
	"context"

	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

type scriptOrderGraphConversionResult struct {
	state    *State
	order    *scriptorder.Order
	warnings []string
	// warningsOmitted counts additional warnings omitted to keep
	// diagnostics bounded.
	warningsOmitted int
}

// convertStateWithScriptOrder converts Terraform modules and processes script
// ordering. For plan-sourced modules, it checks for self-dependencies,
// conflicting requirements, and dependency cycles without resolving agents or
// devcontainer subagents. For state-sourced modules, it resolves agents and
// devcontainer subagents and produces the final order.
func convertStateWithScriptOrder(
	ctx context.Context,
	modules []*tfjson.StateModule,
	config *tfjson.Config,
	rawGraph string,
	logger slog.Logger,
	source proto.GraphSource,
) (*scriptOrderGraphConversionResult, error) {
	switch source {
	case proto.GraphSource_SOURCE_PLAN, proto.GraphSource_SOURCE_STATE:
	default:
		return nil, xerrors.Errorf(
			"unknown script order graph source %d", source,
		)
	}

	state, err := ConvertState(ctx, modules, rawGraph, logger)
	if err != nil {
		return nil, err
	}
	result := &scriptOrderGraphConversionResult{state: state}

	program, err := scriptorder.NewProgram(ctx, modules, config)
	if err != nil {
		return nil, xerrors.Errorf("index script order program: %w", err)
	}
	if program == nil {
		return result, nil
	}
	prepared, err := program.Prepare()
	if err != nil {
		return nil, xerrors.Errorf("prepare script order: %w", err)
	}
	result.warnings = prepared.Warnings()
	result.warningsOmitted = prepared.WarningsOmitted()

	switch source {
	case proto.GraphSource_SOURCE_PLAN:
		if err := prepared.ValidateForPlan(); err != nil {
			return nil, xerrors.Errorf("validate script order for plan: %w", err)
		}
	case proto.GraphSource_SOURCE_STATE:
		bindings, err := resolveAppliedAgentBindings(
			ctx, modules, state.agentCatalog,
			prepared.SelectedScriptAddresses(),
		)
		if err != nil {
			return nil, xerrors.Errorf(
				"resolve script order agents from applied state: %w", err,
			)
		}
		order, err := prepared.Finalize(bindings)
		if err != nil {
			return nil, xerrors.Errorf("finalize script order: %w", err)
		}
		if err := attachScriptOrderDependencies(order, state.scriptsByAddress); err != nil {
			return nil, xerrors.Errorf("attach script order dependencies: %w", err)
		}
		if len(order.Graphs) > 0 {
			result.order = &order
		}
	}
	return result, nil
}

func hasScriptOrderDataSource(modules []*tfjson.StateModule) bool {
	for _, module := range modules {
		if module != nil && moduleHasScriptOrderDataSource(module) {
			return true
		}
	}
	return false
}

func moduleHasScriptOrderDataSource(module *tfjson.StateModule) bool {
	for _, resource := range module.Resources {
		if isScriptOrderDataSource(resource) {
			return true
		}
	}
	for _, child := range module.ChildModules {
		if child != nil && moduleHasScriptOrderDataSource(child) {
			return true
		}
	}
	return false
}

func isScriptOrderDataSource(resource *tfjson.StateResource) bool {
	return resource != nil &&
		resource.Mode == tfjson.DataResourceMode &&
		resource.Type == "coder_script_order"
}
