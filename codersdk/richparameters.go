package codersdk

import (
	"strconv"

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
			return err
		}
	}
	return nil
}

func ValidateWorkspaceBuildParameter(richParameter TemplateVersionParameter, buildParameter *WorkspaceBuildParameter, lastBuildParameter *WorkspaceBuildParameter) error {
	err := validateBuildParameter(richParameter, buildParameter, lastBuildParameter)
	if err != nil {
		name := richParameter.Name
		if richParameter.DisplayName != "" {
			name = richParameter.DisplayName
		}
		return xerrors.Errorf("can't validate build parameter %q: %w", name, err)
	}
	return nil
}

func validateBuildParameter(richParameter TemplateVersionParameter, buildParameter *WorkspaceBuildParameter, lastBuildParameter *WorkspaceBuildParameter) error {
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
		prev, err := strconv.Atoi(lastBuildParameter.Value)
		if err != nil {
			return xerrors.Errorf("previous parameter value is not a number: %s", lastBuildParameter.Value)
		}

		current, err := strconv.Atoi(buildParameter.Value)
		if err != nil {
			return xerrors.Errorf("current parameter value is not a number: %s", buildParameter.Value)
		}

		switch richParameter.ValidationMonotonic {
		case MonotonicOrderIncreasing:
			if prev > current {
				return xerrors.Errorf("parameter value must be equal or greater than previous value: %d", prev)
			}
		case MonotonicOrderDecreasing:
			if prev < current {
				return xerrors.Errorf("parameter value must be equal or lower than previous value: %d", prev)
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

	var min, max int
	if richParameter.ValidationMin != nil {
		min = int(*richParameter.ValidationMin)
	}
	if richParameter.ValidationMax != nil {
		max = int(*richParameter.ValidationMax)
	}

	validation := &provider.Validation{
		Min:         min,
		Max:         max,
		MinDisabled: richParameter.ValidationMin == nil,
		MaxDisabled: richParameter.ValidationMax == nil,
		Regex:       richParameter.ValidationRegex,
		Error:       richParameter.ValidationError,
		Monotonic:   string(richParameter.ValidationMonotonic),
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
		param.ValidationMin != nil ||
		param.ValidationMax != nil ||
		len(param.ValidationMonotonic) > 0 ||
		param.Type == "bool" || // boolean type doesn't have any custom validation rules, but the value must be checked (true/false).
		param.Type == "list(string)" // list(string) type doesn't have special validation, but we need to check if this is a correct list.
}

// ParameterResolver should be populated with legacy workload and rich parameter values from the previous build.  It then
// supports queries against a current TemplateVersionParameter to determine whether a new value is required, or a value
// correctly validates.
// @typescript-ignore ParameterResolver
type ParameterResolver struct {
	Rich []WorkspaceBuildParameter
}

// ValidateResolve checks the provided value, v, against the parameter, p, and the previous build.  If v is nil, it also
// resolves the correct value.  It returns the value of the parameter, if valid, and an error if invalid.
func (r *ParameterResolver) ValidateResolve(p TemplateVersionParameter, v *WorkspaceBuildParameter) (value string, err error) {
	prevV := r.findLastValue(p)
	if !p.Mutable && v != nil && prevV != nil {
		return "", xerrors.Errorf("Parameter %q is not mutable, so it can't be updated after creating a workspace.", p.Name)
	}
	if p.Required && v == nil && prevV == nil {
		return "", xerrors.Errorf("Parameter %q is required but not provided", p.Name)
	}
	// First, the provided value
	resolvedValue := v
	// Second, previous value if not ephemeral
	if resolvedValue == nil && !p.Ephemeral {
		resolvedValue = prevV
	}
	// Last, default value
	if resolvedValue == nil {
		resolvedValue = &WorkspaceBuildParameter{
			Name:  p.Name,
			Value: p.DefaultValue,
		}
	}
	err = ValidateWorkspaceBuildParameter(p, resolvedValue, prevV)
	if err != nil {
		return "", err
	}
	return resolvedValue.Value, nil
}

// findLastValue finds the value from the previous build and returns it, or nil if the parameter had no value in the
// last build.
func (r *ParameterResolver) findLastValue(p TemplateVersionParameter) *WorkspaceBuildParameter {
	for _, rp := range r.Rich {
		if rp.Name == p.Name {
			return &rp
		}
	}
	return nil
}
