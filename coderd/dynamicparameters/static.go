package dynamicparameters

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
	"github.com/coder/terraform-provider-coder/v2/provider"
)

type staticRender struct {
	staticParams []previewtypes.Parameter
}

func (r *loader) staticRender(ctx context.Context, db database.Store) (*staticRender, error) {
	dbTemplateVersionParameters, err := db.GetTemplateVersionParameters(ctx, r.templateVersionID)
	if err != nil {
		return nil, xerrors.Errorf("template version parameters: %w", err)
	}

	params := slice.List(dbTemplateVersionParameters, TemplateVersionParameter)

	for i, param := range params {
		// Update the diagnostics to validate the 'default' value.
		// We do not have a user supplied value yet, so we use the default.
		params[i].Diagnostics = append(params[i].Diagnostics, previewtypes.Diagnostics(param.Valid(param.Value))...)
	}
	return &staticRender{
		staticParams: params,
	}, nil
}

func (r *staticRender) Render(_ context.Context, _ uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics) {
	params := r.staticParams
	for i := range params {
		param := &params[i]
		paramValue, ok := values[param.Name]
		if ok {
			param.Value = previewtypes.StringLiteral(paramValue)
		} else {
			param.Value = param.DefaultValue
		}
		param.Diagnostics = previewtypes.Diagnostics(param.Valid(param.Value))
	}

	return &preview.Output{
			Parameters: params,
		}, hcl.Diagnostics{
			{
				// Only a warning because the form does still work.
				Severity: hcl.DiagWarning,
				Summary:  "This template version is missing required metadata to support dynamic parameters.",
				Detail:   "To restore full functionality, please re-import the terraform as a new template version.",
			},
		}
}

func (*staticRender) Close() {}

func TemplateVersionParameter(it database.TemplateVersionParameter) previewtypes.Parameter {
	param := previewtypes.Parameter{
		ParameterData: previewtypes.ParameterData{
			Name:         it.Name,
			DisplayName:  it.DisplayName,
			Description:  it.Description,
			Type:         previewtypes.ParameterType(it.Type),
			FormType:     provider.ParameterFormType(it.FormType),
			Styling:      previewtypes.ParameterStyling{},
			Mutable:      it.Mutable,
			DefaultValue: previewtypes.StringLiteral(it.DefaultValue),
			Icon:         it.Icon,
			Options:      make([]*previewtypes.ParameterOption, 0),
			Validations:  make([]*previewtypes.ParameterValidation, 0),
			Required:     it.Required,
			Order:        int64(it.DisplayOrder),
			Ephemeral:    it.Ephemeral,
			Source:       nil,
		},
		// Always use the default, since we used to assume the empty string
		Value:       previewtypes.StringLiteral(it.DefaultValue),
		Diagnostics: make(previewtypes.Diagnostics, 0),
	}

	if it.ValidationError != "" || it.ValidationRegex != "" || it.ValidationMonotonic != "" {
		var reg *string
		if it.ValidationRegex != "" {
			reg = ptr.Ref(it.ValidationRegex)
		}

		var vMin *int64
		if it.ValidationMin.Valid {
			vMin = ptr.Ref(int64(it.ValidationMin.Int32))
		}

		var vMax *int64
		if it.ValidationMax.Valid {
			vMax = ptr.Ref(int64(it.ValidationMax.Int32))
		}

		var monotonic *string
		if it.ValidationMonotonic != "" {
			monotonic = ptr.Ref(it.ValidationMonotonic)
		}

		param.Validations = append(param.Validations, &previewtypes.ParameterValidation{
			Error:     it.ValidationError,
			Regex:     reg,
			Min:       vMin,
			Max:       vMax,
			Monotonic: monotonic,
		})
	}

	var protoOptions []*sdkproto.RichParameterOption
	err := json.Unmarshal(it.Options, &protoOptions)
	if err != nil {
		param.Diagnostics = append(param.Diagnostics, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Failed to parse json parameter options",
			Detail:   err.Error(),
		})
	}

	for _, opt := range protoOptions {
		param.Options = append(param.Options, &previewtypes.ParameterOption{
			Name:        opt.Name,
			Description: opt.Description,
			Value:       previewtypes.StringLiteral(opt.Value),
			Icon:        opt.Icon,
		})
	}

	// Take the form type from the ValidateFormType function. This is a bit
	// unfortunate we have to do this, but it will return the default form_type
	// for a given set of conditions.
	_, param.FormType, _ = provider.ValidateFormType(provider.OptionType(param.Type), len(param.Options), param.FormType)
	return param
}
