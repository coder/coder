package terraform

import (
	"context"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/provisioner/terraform/agentruntime"
	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

type scriptOrderGraphConversionResult struct {
	state           *State
	order           *scriptorder.ScriptOrder
	warnings        []string
	warningsOmitted int
}

func convertStateWithScriptOrder(
	ctx context.Context,
	modules []*tfjson.StateModule,
	rawGraph string,
	logger slog.Logger,
	input *scriptOrderRuntimeBindingInput,
) (*scriptOrderGraphConversionResult, error) {
	conversion, err := convertState(ctx, modules, rawGraph, logger, input)
	if err != nil {
		return nil, err
	}
	result := &scriptOrderGraphConversionResult{state: conversion.state}
	if conversion.scriptOrder == nil {
		return result, nil
	}

	order, err := conversion.scriptOrder.prepared.Finalize(
		conversion.scriptOrder.scripts,
	)
	if err != nil {
		return nil, err
	}
	if len(order.Graphs) > 0 {
		result.order = &order
	}
	result.warnings = conversion.scriptOrder.prepared.Warnings()
	result.warningsOmitted = conversion.scriptOrder.prepared.WarningsOmitted()
	return result, nil
}

func prepareScriptOrderRuntimeBindingInput(
	ctx context.Context,
	modules []*tfjson.StateModule,
	config *tfjson.Config,
	source scriptOrderConversionSource,
) (*scriptOrderRuntimeBindingInput, error) {
	program, err := scriptorder.NewProgram(ctx, modules, config)
	if err != nil {
		return nil, err
	}
	if program == nil {
		return nil, nil
	}
	var runtimeProgram *agentruntime.Program
	if source == scriptOrderConversionSourcePlan {
		runtimeProgram, err = agentruntime.NewProgram(ctx, config)
		if err != nil {
			return nil, err
		}
	}
	return prepareScriptOrderRuntimeBindingInputWithPrograms(
		ctx, modules, program, runtimeProgram, source,
	)
}

func prepareScriptOrderRuntimeBindingInputWithPrograms(
	ctx context.Context,
	modules []*tfjson.StateModule,
	program *scriptorder.Program,
	runtimeProgram *agentruntime.Program,
	source scriptOrderConversionSource,
) (*scriptOrderRuntimeBindingInput, error) {
	scripts := make(map[string]scriptorder.Script)
	visited := 0
	for _, module := range modules {
		if module == nil {
			continue
		}
		err := forEachResource(module, func(resource *tfjson.StateResource) error {
			visited++
			if visited%256 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if resource == nil ||
				resource.Mode != tfjson.ManagedResourceMode ||
				resource.Type != "coder_script" {
				return nil
			}
			var attributes agentScriptAttributes
			if err := mapstructure.Decode(
				resource.AttributeValues, &attributes,
			); err != nil {
				return xerrors.Errorf(
					"decode script %q attributes: %w", resource.Address, err,
				)
			}
			scripts[resource.Address] = scriptorder.Script{
				RunOnStart: attributes.RunOnStart,
				RunOnStop:  attributes.RunOnStop,
				Cron:       attributes.Cron,
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prepared, preparationErr := program.Prepare(scripts)
	return &scriptOrderRuntimeBindingInput{
		source:         source,
		program:        program,
		runtimeProgram: runtimeProgram,
		prepared:       prepared,
		preparationErr: preparationErr,
	}, nil
}

func filterRemovedScriptOrderDataSources(
	ctx context.Context,
	index *tfgraph.Index,
	program *scriptorder.Program,
) (*scriptorder.Program, error) {
	if index == nil || program == nil {
		return program, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	addresses := program.DataSourceAddresses()
	candidates := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		candidates[address] = struct{}{}
	}

	// Terraform omits data-resource deletes from the JSON plan's resource
	// changes, but retains their destroy nodes in the saved-plan graph.
	removed := make(map[string]struct{}, len(addresses))
	present := make(map[string]struct{}, len(addresses))
	nodeIndex := 0
	for _, node := range index.Nodes() {
		if nodeIndex%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		nodeIndex++
		address := node.Address()
		if _, candidate := candidates[address]; !candidate {
			continue
		}
		switch node.Operation() {
		case "destroy":
			removed[address] = struct{}{}
		case "":
			present[address] = struct{}{}
		}
	}
	for address := range present {
		delete(removed, address)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(removed) == 0 {
		return program, nil
	}
	return program.FilterDataSources(func(address string) bool {
		_, removed := removed[address]
		return !removed
	}), nil
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
		if resource != nil &&
			resource.Mode == tfjson.DataResourceMode &&
			resource.Type == "coder_script_order" {
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
