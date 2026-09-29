package terraform

import (
	"context"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/agentruntime"
	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
)

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
