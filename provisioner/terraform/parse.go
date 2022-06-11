package terraform

import (
	"encoding/json"
	"os"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk/proto"
)

// Parse extracts Terraform variables from source-code.
func (*server) Parse(request *proto.Parse_Request, stream proto.DRPCProvisioner_ParseStream) error {
	defer func() {
		_ = stream.CloseSend()
	}()

	module, diags := tfconfig.LoadModule(request.Directory)
	if diags.HasErrors() {
		return xerrors.Errorf("load module: %w", diags.Err())
	}
	parameters := make([]*proto.ParameterSchema, 0, len(module.Variables))
	for _, v := range module.Variables {
		schema, err := convertVariableToParameter(v)
		if err != nil {
			return xerrors.Errorf("convert variable %q: %w", v.Name, err)
		}

		parameters = append(parameters, schema)
	}

	return stream.Send(&proto.Parse_Response{
		Type: &proto.Parse_Response_Complete{
			Complete: &proto.Parse_Complete{
				ParameterSchemas: parameters,
			},
		},
	})
}

// Converts a Terraform variable to a provisioner parameter.
func convertVariableToParameter(variable *tfconfig.Variable) (*proto.ParameterSchema, error) {
	schema := &proto.ParameterSchema{
		Name:                variable.Name,
		Description:         variable.Description,
		RedisplayValue:      !variable.Sensitive,
		AllowOverrideSource: !variable.Sensitive,
		ValidationValueType: variable.Type,
		DefaultDestination: &proto.ParameterDestination{
			Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
		},
	}

	if variable.Default != nil {
		defaultData, valid := variable.Default.(string)
		if !valid {
			defaultDataRaw, err := json.Marshal(variable.Default)
			if err != nil {
				return nil, xerrors.Errorf("parse variable %q default: %w", variable.Name, err)
			}
			defaultData = string(defaultDataRaw)
		}

		schema.DefaultSource = &proto.ParameterSource{
			Scheme: proto.ParameterSource_DATA,
			Value:  defaultData,
		}
	}

	if len(variable.Validations) > 0 && variable.Validations[0].Condition != nil {
		// Terraform can contain multiple validation blocks, but it's used sparingly
		// from what it appears.
		validation := variable.Validations[0]
		filedata, err := os.ReadFile(variable.Pos.Filename)
		if err != nil {
			return nil, xerrors.Errorf("read file %q: %w", variable.Pos.Filename, err)
		}
		schema.ValidationCondition = string(filedata[validation.Condition.Range().Start.Byte:validation.Condition.Range().End.Byte])
		schema.ValidationError = validation.ErrorMessage
		schema.ValidationTypeSystem = proto.ParameterSchema_HCL
	}

	return schema, nil
}
