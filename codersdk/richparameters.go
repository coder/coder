package codersdk

import (
	"golang.org/x/xerrors"

	"github.com/coder/terraform-provider-coder/provider"
)

func ValidateWorkspaceBuildParameters(richParameters []TemplateVersionParameter, buildParameters []WorkspaceBuildParameter) error {
	for _, buildParameter := range buildParameters {
		richParameter, found := findTemplateVersionParameter(richParameters, buildParameter.Name)
		if !found {
			return xerrors.Errorf(`workspace build parameter is not defined in the template ("coder_parameter")`)
		}

		err := ValidateWorkspaceBuildParameter(*richParameter, buildParameter)
		if err != nil {
			return xerrors.Errorf("can't validate build parameter %q: %w", buildParameter.Name, err)
		}
	}
	return nil
}

func ValidateWorkspaceBuildParameter(richParameter TemplateVersionParameter, buildParameter WorkspaceBuildParameter) error {
	if buildParameter.Value == "" {
		return xerrors.Errorf("parameter value can't be empty")
	}

	if len(richParameter.Options) > 0 {
		var matched bool
		for _, opt := range richParameter.Options {
			if opt.Value == buildParameter.Value {
				matched = true
				break
			}
		}

		if !matched {
			return xerrors.Errorf("parameter value must match one of options: %s", parameterValuesAsArray(richParameter.Options))
		}
		return nil
	}

	validation := &provider.Validation{
		Min:   int(richParameter.ValidationMin),
		Max:   int(richParameter.ValidationMax),
		Regex: richParameter.ValidationRegex,
	}
	return validation.Valid(richParameter.Type, buildParameter.Value)
}

func findTemplateVersionParameter(params []TemplateVersionParameter, parameterName string) (*TemplateVersionParameter, bool) {
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
