package terraform

import (
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"
)

type PlanState struct {
	DailyCost   int32
	AITaskCount int32
}

func planModules(plan *tfjson.Plan) []*tfjson.StateModule {
	modules := []*tfjson.StateModule{}
	if plan.PriorState != nil {
		// We need the data resources for rich parameters. For some reason, they
		// only show up in the PriorState.
		//
		// We don't want all prior resources, because Quotas (and
		// future features) would never know which resources are getting
		// deleted by a stop.

		filtered := onlyDataResources(*plan.PriorState.Values.RootModule)
		modules = append(modules, &filtered)
	}
	modules = append(modules, plan.PlannedValues.RootModule)
	return modules
}

// ConvertPlanState consumes a terraform plan json output and produces a thinner
// version of `State` to be used before `terraform apply`. `ConvertState`
// requires `terraform graph`, this does not.
func ConvertPlanState(plan *tfjson.Plan) (*PlanState, error) {
	modules := planModules(plan)

	var dailyCost int32
	var aiTaskCount int32
	for _, mod := range modules {
		err := forEachResource(mod, func(res *tfjson.StateResource) error {
			switch res.Type {
			case "coder_metadata":
				var attrs resourceMetadataAttributes
				err := mapstructure.Decode(res.AttributeValues, &attrs)
				if err != nil {
					return xerrors.Errorf("decode metadata attributes: %w", err)
				}
				dailyCost += attrs.DailyCost
			case "coder_ai_task":
				aiTaskCount++
			}
			return nil
		})
		if err != nil {
			return nil, xerrors.Errorf("parse plan: %w", err)
		}
	}

	return &PlanState{
		DailyCost:   dailyCost,
		AITaskCount: aiTaskCount,
	}, nil
}

func forEachResource(input *tfjson.StateModule, do func(res *tfjson.StateResource) error) error {
	for _, res := range input.Resources {
		err := do(res)
		if err != nil {
			return xerrors.Errorf("in module %s: %w", input.Address, err)
		}
	}

	for _, mod := range input.ChildModules {
		err := forEachResource(mod, do)
		if err != nil {
			return xerrors.Errorf("in module %s: %w", mod.Address, err)
		}
	}
	return nil
}
