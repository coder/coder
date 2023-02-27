package terraform

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk/proto"
)

const featureUseManagedVariables = "feature_use_managed_variables"

var terraformWithFeaturesSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "provider",
			LabelNames: []string{"type"},
		},
	},
}

var providerFeaturesConfigSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name: featureUseManagedVariables,
		},
	},
}

// Parse extracts Terraform variables from source-code.
func (*server) Parse(request *proto.Parse_Request, stream proto.DRPCProvisioner_ParseStream) error {
	// Load the module and print any parse errors.
	module, diags := tfconfig.LoadModule(request.Directory)
	if diags.HasErrors() {
		return xerrors.Errorf("load module: %s", formatDiagnostics(request.Directory, diags))
	}

	flags, flagsDiags := loadEnabledFeatures(request.Directory)
	if flagsDiags.HasErrors() {
		return xerrors.Errorf("load coder provider features: %s", formatDiagnostics(request.Directory, diags))
	}

	// Sort variables by (filename, line) to make the ordering consistent
	variables := make([]*tfconfig.Variable, 0, len(module.Variables))
	for _, v := range module.Variables {
		variables = append(variables, v)
	}
	sort.Slice(variables, func(i, j int) bool {
		return compareSourcePos(variables[i].Pos, variables[j].Pos)
	})

	var parameters []*proto.ParameterSchema
	var templateVariables []*proto.TemplateVariable

	useManagedVariables := flags != nil && flags[featureUseManagedVariables]
	if useManagedVariables {
		for _, v := range variables {
			mv, err := convertTerraformVariableToManagedVariable(v)
			if err != nil {
				return xerrors.Errorf("can't convert the Terraform variable to a managed one: %w", err)
			}
			templateVariables = append(templateVariables, mv)
		}
	} else {
		for _, v := range variables {
			schema, err := convertVariableToParameter(v)
			if err != nil {
				return xerrors.Errorf("convert variable %q: %w", v.Name, err)
			}

			parameters = append(parameters, schema)
		}
	}
	return stream.Send(&proto.Parse_Response{
		Type: &proto.Parse_Response_Complete{
			Complete: &proto.Parse_Complete{
				ParameterSchemas:  parameters,
				TemplateVariables: templateVariables,
			},
		},
	})
}

func loadEnabledFeatures(moduleDir string) (map[string]bool, hcl.Diagnostics) {
	flags := map[string]bool{}
	var diags hcl.Diagnostics

	entries, err := os.ReadDir(moduleDir)
	if err != nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Failed to read module directory",
			Detail:   fmt.Sprintf("Module directory %s does not exist or cannot be read.", moduleDir),
		})
		return flags, diags
	}

	var found bool
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}

		flags, found, diags = parseFeatures(path.Join(moduleDir, entry.Name()))
		if found {
			break
		}
	}
	return flags, diags
}

func parseFeatures(hclFilepath string) (map[string]bool, bool, hcl.Diagnostics) {
	flags := map[string]bool{}
	var diags hcl.Diagnostics

	_, err := os.Stat(hclFilepath)
	if os.IsNotExist(err) {
		return flags, false, diags
	} else if err != nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Failed to open %q file", hclFilepath),
		})
		return flags, false, diags
	}

	parser := hclparse.NewParser()
	parsedHCL, diags := parser.ParseHCLFile(hclFilepath)
	if diags.HasErrors() {
		return flags, false, diags
	}

	var found bool
	content, _ := parsedHCL.Body.Content(terraformWithFeaturesSchema)
	for _, block := range content.Blocks {
		if block.Type == "provider" && block.Labels[0] == "coder" {
			content, _, partialDiags := block.Body.PartialContent(providerFeaturesConfigSchema)
			diags = append(diags, partialDiags...)
			if attr, defined := content.Attributes[featureUseManagedVariables]; defined {
				found = true

				var useManagedVariables bool
				partialDiags := gohcl.DecodeExpression(attr.Expr, nil, &useManagedVariables)
				diags = append(diags, partialDiags...)
				flags[featureUseManagedVariables] = useManagedVariables
			}
		}
	}
	return flags, found, diags
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

// Converts a Terraform variable to a managed variable.
func convertTerraformVariableToManagedVariable(variable *tfconfig.Variable) (*proto.TemplateVariable, error) {
	var defaultData string
	if variable.Default != nil {
		var valid bool
		defaultData, valid = variable.Default.(string)
		if !valid {
			defaultDataRaw, err := json.Marshal(variable.Default)
			if err != nil {
				return nil, xerrors.Errorf("parse variable %q default: %w", variable.Name, err)
			}
			defaultData = string(defaultDataRaw)
		}
	}

	return &proto.TemplateVariable{
		Name:         variable.Name,
		Description:  variable.Description,
		Type:         variable.Type,
		DefaultValue: defaultData,
		// variable.Required is always false. Empty string is a valid default value, so it doesn't enforce required to be "true".
		Required:  variable.Default == nil,
		Sensitive: variable.Sensitive,
	}, nil
}

// formatDiagnostics returns a nicely formatted string containing all of the
// error details within the tfconfig.Diagnostics. We need to use this because
// the default format doesn't provide much useful information.
func formatDiagnostics(baseDir string, diags tfconfig.Diagnostics) string {
	var msgs strings.Builder
	for _, d := range diags {
		// Convert severity.
		severity := "UNKNOWN SEVERITY"
		switch {
		case d.Severity == tfconfig.DiagError:
			severity = "ERROR"
		case d.Severity == tfconfig.DiagWarning:
			severity = "WARN"
		}

		// Determine filepath and line
		location := "unknown location"
		if d.Pos != nil {
			filename, err := filepath.Rel(baseDir, d.Pos.Filename)
			if err != nil {
				filename = d.Pos.Filename
			}
			location = fmt.Sprintf("%s:%d", filename, d.Pos.Line)
		}

		_, _ = msgs.WriteString(fmt.Sprintf("\n%s: %s (%s)\n", severity, d.Summary, location))

		// Wrap the details to 80 characters and indent them.
		if d.Detail != "" {
			wrapped := wordwrap.WrapString(d.Detail, 78)
			for _, line := range strings.Split(wrapped, "\n") {
				_, _ = msgs.WriteString(fmt.Sprintf("> %s\n", line))
			}
		}
	}

	spacer := " "
	if len(diags) > 1 {
		spacer = "\n\n"
	}

	return spacer + strings.TrimSpace(msgs.String())
}

func compareSourcePos(x, y tfconfig.SourcePos) bool {
	if x.Filename != y.Filename {
		return x.Filename < y.Filename
	}
	return x.Line < y.Line
}
