package dynamicparameters

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"

	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
)

// DiagnosticCodeStaleOption identifies the warning raised when a value is
// replaced by the default because it is no longer one of the parameter's options.
const DiagnosticCodeStaleOption = "stale_option"

// RenderReconciled renders the template version's parameters, dropping any
// mutable parameter value that is no longer one of that parameter's options.
//
// A template update can remove an option value that a workspace already selected.
// Dropping the value renders the default instead, and the substitution is warn logged.
// Immutable parameters are left alone.
func RenderReconciled(ctx context.Context, renderer Renderer, ownerID uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics) {
	output, diags := renderer.Render(ctx, ownerID, values)
	if output == nil || diags.HasErrors() {
		return output, diags
	}

	stale := staleOptionValues(output.Parameters, values)
	if len(stale) == 0 {
		return output, diags
	}

	reconciled := maps.Clone(values)
	for name := range stale {
		delete(reconciled, name)
	}

	// rerender with stale values removed from reconciled
	output, diags = renderer.Render(ctx, ownerID, reconciled)
	if output == nil || diags.HasErrors() {
		return output, diags
	}

	for i, parameter := range output.Parameters {
		if dropped, ok := stale[parameter.Name]; ok {
			output.Parameters[i].Diagnostics = append(output.Parameters[i].Diagnostics, staleOptionDiagnostic(dropped))
		}
	}

	return output, diags
}

// staleOptionValues returns the supplied value of every mutable parameter that is no longer an option.
func staleOptionValues(parameters []previewtypes.Parameter, values map[string]string) map[string]string {
	stale := make(map[string]string)
	for _, parameter := range parameters {
		if !parameter.Mutable {
			continue
		}

		value, ok := values[parameter.Name]
		if !ok || isValidParameterOption(parameter, value) {
			continue
		}

		stale[parameter.Name] = value
	}
	return stale
}

// isValidParameterOption reports whether value is one of the parameter's valid options.
// Parameters without options accept any value.
//
// A parameter whose options could not all be resolved is treated as valid,
// because there is no complete option set to judge the value against.
func isValidParameterOption(parameter previewtypes.Parameter, value string) bool {
	if len(parameter.Options) == 0 {
		return true
	}

	options := make(map[string]struct{}, len(parameter.Options))
	for _, option := range parameter.Options {
		if !option.Value.IsKnown() || !option.Value.Valid() {
			return true
		}
		options[option.Value.AsString()] = struct{}{}
	}

	if parameter.Type == previewtypes.ParameterTypeListString {
		var selected []string
		if err := json.Unmarshal([]byte(value), &selected); err != nil {
			return false
		}

		for _, entry := range selected {
			if _, ok := options[entry]; !ok {
				return false
			}
		}
		return true
	}

	_, ok := options[value]
	return ok
}

func staleOptionDiagnostic(value string) *hcl.Diagnostic {
	return previewtypes.DiagnosticCode(&hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Previously selected option is no longer available",
		Detail:   fmt.Sprintf("The value %q is not one of the available options.", value),
	}, DiagnosticCodeStaleOption)
}
