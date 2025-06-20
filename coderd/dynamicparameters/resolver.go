package dynamicparameters

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

type ParameterResolver struct {
	renderer       Renderer
	firstBuild     bool
	presetValues   []database.TemplateVersionPresetParameter
	previousValues []database.WorkspaceBuildParameter
	buildValues    []database.WorkspaceBuildParameter
}

type parameterValueSource int

const (
	sourcePrevious parameterValueSource = iota
	sourceBuild
	sourcePreset
)

type parameterValue struct {
	Value  string
	Source parameterValueSource
}

func ResolveParameters(
	ctx context.Context,
	ownerID uuid.UUID,
	renderer Renderer,
	firstBuild bool,
	previousValues []database.WorkspaceBuildParameter,
	buildValues []codersdk.WorkspaceBuildParameter,
	presetValues []database.TemplateVersionPresetParameter,
) (map[string]string, hcl.Diagnostics) {
	previousValuesMap := slice.ToMap(previousValues, func(p database.WorkspaceBuildParameter) (string, string) {
		return p.Value, p.Value
	})

	// Start with previous
	values := parameterValueMap(slice.ToMap(previousValues, func(p database.WorkspaceBuildParameter) (string, parameterValue) {
		return p.Name, parameterValue{Source: sourcePrevious, Value: p.Value}
	}))

	// Add build values
	for _, buildValue := range buildValues {
		if _, ok := values[buildValue.Name]; !ok {
			values[buildValue.Name] = parameterValue{Source: sourceBuild, Value: buildValue.Value}
		}
	}

	// Add preset values
	for _, preset := range presetValues {
		if _, ok := values[preset.Name]; !ok {
			values[preset.Name] = parameterValue{Source: sourcePreset, Value: preset.Value}
		}
	}

	originalValues := make(map[string]parameterValue, len(values))
	for name, value := range values {
		// Store the original values for later use.
		originalValues[name] = value
	}

	// Render the parameters using the values that were supplied to the previous build.
	//
	// This is how the form should look to the user on their workspace settings page.
	// This is the original form truth that our validations should be based on going forward.
	output, diags := renderer.Render(ctx, ownerID, values.ValuesMap())
	if diags.HasErrors() {
		// Top level diagnostics should break the build. Previous values (and new) should
		// always be valid. If there is a case where this is not true, then this has to
		// be changed to allow the build to continue with a different set of values.

		return nil, diags
	}

	// The user's input now needs to be validated against the parameters.
	// Mutability & Ephemeral parameters depend on sequential workspace builds.
	//
	// To enforce these, the user's input values are trimmed based on the
	// mutability and ephemeral parameters defined in the template version.
	// The output parameters
	for _, parameter := range output.Parameters {
		// Ephemeral parameters should not be taken from the previous build.
		// Remove their values from the input if they are sourced from the previous build.
		if parameter.Ephemeral {
			v := values[parameter.Name]
			if v.Source == sourcePrevious {
				delete(values, parameter.Name)
			}
		}

		// Immutable parameters should also not be allowed to be changed from
		// the previous build. Remove any values taken from the preset or
		// new build params. This forces the value to be the same as it was before.
		if !firstBuild && !parameter.Mutable {
			delete(values, parameter.Name)
			prev, ok := previousValuesMap[parameter.Name]
			if ok {
				values[parameter.Name] = parameterValue{
					Value:  prev,
					Source: sourcePrevious,
				}
			}
		}
	}

	// This is the final set of values that will be used. Any errors at this stage
	// are fatal. Additional validation for immutability has to be done manually.
	output, diags = renderer.Render(ctx, ownerID, values.ValuesMap())
	if diags.HasErrors() {
		return nil, diags
	}

	for _, parameter := range output.Parameters {
		if !firstBuild && !parameter.Mutable {
			if parameter.Value.AsString() != originalValues[parameter.Name].Value {
				var src *hcl.Range
				if parameter.Source != nil {
					src = &parameter.Source.HCLBlock().TypeRange
				}

				// An immutable parameter was changed, which is not allowed.
				// Add the failed diagnostic to the output.
				diags = diags.Append(&hcl.Diagnostic{
					Severity: 0,
					Summary:  "Immutable parameter changed",
					Detail:   fmt.Sprintf("Parameter %q is not mutable, so it can't be updated after creating a workspace.", parameter.Name),
					Subject:  src,
				})
			}
		}
	}

	// TODO: Validate all parameter values.

	// Return the values to be saved for the build.
	// TODO: The previous code always returned parameter names and values, even if they were not set
	//  by the user. So this should loop over the parameters and return all of them.
	//  This catches things like if a default value changes, we keep the old value.
	return values.ValuesMap(), diags
}

type parameterValueMap map[string]parameterValue

func (p parameterValueMap) ValuesMap() map[string]string {
	values := make(map[string]string, len(p))
	for name, paramValue := range p {
		values[name] = paramValue.Value
	}
	return values
}
