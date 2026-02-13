package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"tailscale.com/types/ptr"

	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/terraform-provider-coder/v2/provider"
)

func (c *Client) EvaluateTemplateVersion(ctx context.Context, templateVersionID uuid.UUID, ownerID uuid.UUID, inputs map[string]string) (DynamicParametersResponse, error) {
	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/templateversions/%s/dynamic-parameters/evaluate", templateVersionID),
		DynamicParametersRequest{
			ID:      0,
			Inputs:  inputs,
			OwnerID: ownerID,
		})
	if err != nil {
		return DynamicParametersResponse{}, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return DynamicParametersResponse{}, ReadBodyAsError(res)
	}

	var dynResp DynamicParametersResponse
	return dynResp, json.NewDecoder(res.Body).Decode(&dynResp)
}

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
	var (
		current  string
		previous *string
	)

	if buildParameter != nil {
		current = buildParameter.Value
	}

	if lastBuildParameter != nil {
		previous = ptr.To(lastBuildParameter.Value)
	}

	if richParameter.Required && current == "" {
		return xerrors.Errorf("parameter value is required")
	}

	if current == "" { // parameter is optional, so take the default value
		current = richParameter.DefaultValue
	}

	if len(richParameter.Options) > 0 && !inOptionSet(richParameter, current) {
		return xerrors.Errorf("parameter value must match one of options: %s", parameterValuesAsArray(richParameter.Options))
	}

	if !validationEnabled(richParameter) {
		return nil
	}

	var minVal, maxVal int
	if richParameter.ValidationMin != nil {
		minVal = int(*richParameter.ValidationMin)
	}
	if richParameter.ValidationMax != nil {
		maxVal = int(*richParameter.ValidationMax)
	}

	validation := &provider.Validation{
		Min:         minVal,
		Max:         maxVal,
		MinDisabled: richParameter.ValidationMin == nil,
		MaxDisabled: richParameter.ValidationMax == nil,
		Regex:       richParameter.ValidationRegex,
		Error:       richParameter.ValidationError,
		Monotonic:   string(richParameter.ValidationMonotonic),
	}
	return validation.Valid(richParameter.Type, current, previous)
}

// inOptionSet returns if the value given is in the set of options for a parameter.
func inOptionSet(richParameter TemplateVersionParameter, value string) bool {
	optionValues := make([]string, 0, len(richParameter.Options))
	for _, option := range richParameter.Options {
		optionValues = append(optionValues, option.Value)
	}

	// If the type is `list(string)` and the form_type is `multi-select`, then we check each individual
	// value in the list against the option set.
	isMultiSelect := richParameter.Type == provider.OptionTypeListString && richParameter.FormType == string(provider.ParameterFormTypeMultiSelect)

	if !isMultiSelect {
		// This is the simple case. Just checking if the value is in the option set.
		return slice.Contains(optionValues, value)
	}

	var checks []string
	err := json.Unmarshal([]byte(value), &checks)
	if err != nil {
		return false
	}

	for _, check := range checks {
		if !slice.Contains(optionValues, check) {
			return false
		}
	}

	return true
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
	if !p.Mutable && v != nil && prevV != nil && v.Value != prevV.Value {
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

// Resolve returns the value of the parameter. It does not do any validation,
// and is meant for use with the new dynamic parameters code path.
func (r *ParameterResolver) Resolve(p TemplateVersionParameter, v *WorkspaceBuildParameter) string {
	prevV := r.findLastValue(p)
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
	return resolvedValue.Value
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
