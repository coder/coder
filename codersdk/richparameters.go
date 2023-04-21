package codersdk

import (
	"golang.org/x/xerrors"

	"github.com/coder/terraform-provider-coder/provider"
)

func ValidateNewWorkspaceParameters(richParameters []TemplateVersionParameter, buildParameters []WorkspaceBuildParameter) error {
	return ValidateWorkspaceBuildParameters(richParameters, buildParameters, nil)
}

func ValidateWorkspaceBuildParameters(richParameters []TemplateVersionParameter, buildParameters, lastBuildParameters []WorkspaceBuildParameter) error {
	for _, richParameter := range richParameters {
		buildParameter, foundBuildParameter := findBuildParameter(buildParameters, richParameter.Name)
		lastBuildParameter, foundLastBuildParameter := findBuildParameter(lastBuildParameters, richParameter.Name)

		if richParameter.Required && !foundBuildParameter && !foundLastBuildParameter {
			return xerrors.Errorf("workspace build parameter %q is required", richParameter.Name)
		}

		if !foundBuildParameter && foundLastBuildParameter {
			continue // previous build parameters have been validated before the last build
		}

		err := ValidateWorkspaceBuildParameter(richParameter, buildParameter, lastBuildParameter)
		if err != nil {
			return xerrors.Errorf("can't validate build parameter %q: %w", richParameter.Name, err)
		}
	}
	return nil
}

func ValidateWorkspaceBuildParameter(richParameter TemplateVersionParameter, buildParameter *WorkspaceBuildParameter, lastBuildParameter *WorkspaceBuildParameter) error {
	var value string

	if buildParameter != nil {
		value = buildParameter.Value
	}

	if richParameter.Required && value == "" {
		return xerrors.Errorf("parameter value is required")
	}

	if value == "" { // parameter is optional, so take the default value
		value = richParameter.DefaultValue
	}

	if lastBuildParameter != nil && richParameter.Type == "number" && len(richParameter.ValidationMonotonic) > 0 {
		switch richParameter.ValidationMonotonic {
		case MonotonicOrderIncreasing:
			if lastBuildParameter.Value > buildParameter.Value {
				return xerrors.Errorf("parameter value must be equal or greater than previous value: %s", lastBuildParameter.Value)
			}
		case MonotonicOrderDecreasing:
			if lastBuildParameter.Value < buildParameter.Value {
				return xerrors.Errorf("parameter value must be equal or lower than previous value: %s", lastBuildParameter.Value)
			}
		}
	}

	if len(richParameter.Options) > 0 {
		var matched bool
		for _, opt := range richParameter.Options {
			if opt.Value == value {
				matched = true
				break
			}
		}

		if !matched {
			return xerrors.Errorf("parameter value must match one of options: %s", parameterValuesAsArray(richParameter.Options))
		}
		return nil
	}

	if !validationEnabled(richParameter) {
		return nil
	}

	validation := &provider.Validation{
		Min:       int(richParameter.ValidationMin),
		Max:       int(richParameter.ValidationMax),
		Regex:     richParameter.ValidationRegex,
		Error:     richParameter.ValidationError,
		Monotonic: string(richParameter.ValidationMonotonic),
	}
	return validation.Valid(richParameter.Type, value)
}

func findBuildParameter(params []WorkspaceBuildParameter, parameterName string) (*WorkspaceBuildParameter, bool) {
	if params == nil {
		return nil, false
	}

	for _, p := range params {
		if p.Name == parameterName {
			return &p, true
		}
	}
	return nil, false
}

func parameterValuesAsArray(options []TemplateVersionParameterOption) []string {
	var arr []string
	for _, opt := range options {
		arr = append(arr, opt.Value)
	}
	return arr
}

func validationEnabled(param TemplateVersionParameter) bool {
	return len(param.ValidationRegex) > 0 ||
		(param.ValidationMin != 0 && param.ValidationMax != 0) ||
		len(param.ValidationMonotonic) > 0 ||
		param.Type == "bool" || // boolean type doesn't have any custom validation rules, but the value must be checked (true/false).
		param.Type == "list(string)" // list(string) type doesn't have special validation, but we need to check if this is a correct list.
}
