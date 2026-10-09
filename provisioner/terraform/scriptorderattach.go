package terraform

import (
	"cmp"
	"slices"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

// attachScriptOrderDependencies adds one ScriptDependency to the dependent
// script of every edge in order. Prerequisites gain nothing. scriptsByAddress
// must hold every address the graphs name.
func attachScriptOrderDependencies(order scriptorder.ScriptOrder, scriptsByAddress map[string]*proto.Script) error {
	// Nothing is attached until every edge validates, so an error leaves
	// every script unchanged.
	additions := make(map[*proto.Script][]*proto.ScriptDependency)
	for _, graph := range order.Graphs {
		for _, dependency := range graph.Dependencies {
			script, ok := scriptsByAddress[dependency.DependentAddress]
			if !ok {
				return xerrors.Errorf("dependent script %q not found among converted scripts", dependency.DependentAddress)
			}
			if _, ok := scriptsByAddress[dependency.PrerequisiteAddress]; !ok {
				return xerrors.Errorf("prerequisite script %q not found among converted scripts", dependency.PrerequisiteAddress)
			}
			requirement, err := scriptDependencyRequirementProto(dependency.Requirement)
			if err != nil {
				return err
			}
			additions[script] = append(additions[script], &proto.ScriptDependency{
				PrerequisiteResourceAddress: dependency.PrerequisiteAddress,
				Requirement:                 requirement,
			})
		}
	}

	// The wire order is this package's contract. Sorting here keeps it
	// independent of the order the graph builder happens to produce.
	for script, dependencies := range additions {
		script.Dependencies = append(script.Dependencies, dependencies...)
		slices.SortFunc(script.Dependencies, func(a, b *proto.ScriptDependency) int {
			return cmp.Compare(a.PrerequisiteResourceAddress, b.PrerequisiteResourceAddress)
		})
	}
	return nil
}

// scriptDependencyRequirementProto maps a validated requirement to the wire
// enum. Anything else is an error so that UNSPECIFIED is never sent.
func scriptDependencyRequirementProto(requirement scriptorder.Requirement) (proto.ScriptDependencyRequirement, error) {
	switch requirement {
	case scriptorder.ScriptOrderRequirementSuccess:
		return proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS, nil
	case scriptorder.ScriptOrderRequirementCompletion:
		return proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_COMPLETION, nil
	default:
		return proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_UNSPECIFIED,
			xerrors.Errorf("unknown script dependency requirement %q", requirement)
	}
}
