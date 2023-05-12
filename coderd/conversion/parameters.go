package conversion

import (
	"encoding/json"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func WorkspaceBuildParameters(params []database.WorkspaceBuildParameter) []codersdk.WorkspaceBuildParameter {
	out := make([]codersdk.WorkspaceBuildParameter, len(params))
	for i, p := range params {
		out[i] = WorkspaceBuildParameter(p)
	}
	return out
}

func WorkspaceBuildParameter(p database.WorkspaceBuildParameter) codersdk.WorkspaceBuildParameter {
	return codersdk.WorkspaceBuildParameter{
		Name:  p.Name,
		Value: p.Value,
	}
}

func TemplateVersionParameter(param database.TemplateVersionParameter) (codersdk.TemplateVersionParameter, error) {
	var protoOptions []*proto.RichParameterOption
	err := json.Unmarshal(param.Options, &protoOptions)
	if err != nil {
		return codersdk.TemplateVersionParameter{}, err
	}
	options := make([]codersdk.TemplateVersionParameterOption, 0)
	for _, option := range protoOptions {
		options = append(options, codersdk.TemplateVersionParameterOption{
			Name:        option.Name,
			Description: option.Description,
			Value:       option.Value,
			Icon:        option.Icon,
		})
	}

	descriptionPlaintext, err := parameter.Plaintext(param.Description)
	if err != nil {
		return codersdk.TemplateVersionParameter{}, err
	}
	return codersdk.TemplateVersionParameter{
		Name:                 param.Name,
		DisplayName:          param.DisplayName,
		Description:          param.Description,
		DescriptionPlaintext: descriptionPlaintext,
		Type:                 param.Type,
		Mutable:              param.Mutable,
		DefaultValue:         param.DefaultValue,
		Icon:                 param.Icon,
		Options:              options,
		ValidationRegex:      param.ValidationRegex,
		ValidationMin:        param.ValidationMin,
		ValidationMax:        param.ValidationMax,
		ValidationError:      param.ValidationError,
		ValidationMonotonic:  codersdk.ValidationMonotonicOrder(param.ValidationMonotonic),
		Required:             param.Required,
		LegacyVariableName:   param.LegacyVariableName,
	}, nil
}

func Parameters(params []database.ParameterValue) []codersdk.Parameter {
	out := make([]codersdk.Parameter, len(params))
	for i, p := range params {
		out[i] = Parameter(p)
	}
	return out
}

func Parameter(parameterValue database.ParameterValue) codersdk.Parameter {
	return codersdk.Parameter{
		ID:                parameterValue.ID,
		CreatedAt:         parameterValue.CreatedAt,
		UpdatedAt:         parameterValue.UpdatedAt,
		Scope:             codersdk.ParameterScope(parameterValue.Scope),
		ScopeID:           parameterValue.ScopeID,
		Name:              parameterValue.Name,
		SourceScheme:      codersdk.ParameterSourceScheme(parameterValue.SourceScheme),
		DestinationScheme: codersdk.ParameterDestinationScheme(parameterValue.DestinationScheme),
		SourceValue:       parameterValue.SourceValue,
	}
}
