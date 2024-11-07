package tfparse

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/coder/coder/v2/archive"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// NOTE: This is duplicated from coderd but we can't import it here without
// introducing a circular dependency
const maxFileSizeBytes = 10 * (10 << 20) // 10 MB

// WorkspaceTags extracts tags from coder_workspace_tags data sources defined in module.
// Note that this only returns the lexical values of the data source, and does not
// evaluate variables and such. To do this, see evalProvisionerTags below.
func WorkspaceTags(ctx context.Context, module *tfconfig.Module) (map[string]string, error) {
	workspaceTags := map[string]string{}

	for _, dataResource := range module.DataResources {
		if dataResource.Type != "coder_workspace_tags" {
			continue
		}

		var file *hcl.File
		var diags hcl.Diagnostics
		parser := hclparse.NewParser()

		if !strings.HasSuffix(dataResource.Pos.Filename, ".tf") {
			continue
		}
		// We know in which HCL file is the data resource defined.
		file, diags = parser.ParseHCLFile(dataResource.Pos.Filename)
		if diags.HasErrors() {
			return nil, xerrors.Errorf("can't parse the resource file: %s", diags.Error())
		}

		// Parse root to find "coder_workspace_tags".
		content, _, diags := file.Body.PartialContent(rootTemplateSchema)
		if diags.HasErrors() {
			return nil, xerrors.Errorf("can't parse the resource file: %s", diags.Error())
		}

		// Iterate over blocks to locate the exact "coder_workspace_tags" data resource.
		for _, block := range content.Blocks {
			if !slices.Equal(block.Labels, []string{"coder_workspace_tags", dataResource.Name}) {
				continue
			}

			// Parse "coder_workspace_tags" to find all key-value tags.
			resContent, _, diags := block.Body.PartialContent(coderWorkspaceTagsSchema)
			if diags.HasErrors() {
				return nil, xerrors.Errorf(`can't parse the resource coder_workspace_tags: %s`, diags.Error())
			}

			if resContent == nil {
				continue // workspace tags are not present
			}

			if _, ok := resContent.Attributes["tags"]; !ok {
				return nil, xerrors.Errorf(`"tags" attribute is required by coder_workspace_tags`)
			}

			expr := resContent.Attributes["tags"].Expr
			tagsExpr, ok := expr.(*hclsyntax.ObjectConsExpr)
			if !ok {
				return nil, xerrors.Errorf(`"tags" attribute is expected to be a key-value map`)
			}

			// Parse key-value entries in "coder_workspace_tags"
			for _, tagItem := range tagsExpr.Items {
				key, err := previewFileContent(tagItem.KeyExpr.Range())
				if err != nil {
					return nil, xerrors.Errorf("can't preview the resource file: %v", err)
				}
				key = strings.Trim(key, `"`)

				value, err := previewFileContent(tagItem.ValueExpr.Range())
				if err != nil {
					return nil, xerrors.Errorf("can't preview the resource file: %v", err)
				}

				if _, ok := workspaceTags[key]; ok {
					return nil, xerrors.Errorf(`workspace tag %q is defined multiple times`, key)
				}
				workspaceTags[key] = value
			}
		}
	}
	return workspaceTags, nil
}

//	WorkspaceTagDefaultsFromFile extracts the default values for a `coder_workspace_tags` resource from the given
//
// file. It also ensures that any uses of the `coder_workspace_tags` data source only
// reference the following data types:
// 1. Static variables
// 2. Template variables
// 3. Coder parameters
// Any other data types are not allowed, as their values cannot be known at
// the time of template import.
func WorkspaceTagDefaultsFromFile(ctx context.Context, logger slog.Logger, file []byte, mimetype string) (tags map[string]string, err error) {
	module, cleanup, err := loadModuleFromFile(file, mimetype)
	if err != nil {
		return nil, xerrors.Errorf("load module from file: %w", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			logger.Error(ctx, "failed to clean up", slog.Error(err))
		}
	}()

	// This only gets us the expressions. We need to evaluate them.
	// Example: var.region -> "us"
	tags, err = WorkspaceTags(ctx, module)
	if err != nil {
		return nil, xerrors.Errorf("extract workspace tags: %w", err)
	}

	// To evaluate the expressions, we need to load the default values for
	// variables and parameters.
	varsDefaults, paramsDefaults, err := loadDefaults(module)
	if err != nil {
		return nil, xerrors.Errorf("load defaults: %w", err)
	}

	// Evaluate the tags expressions given the inputs.
	// This will resolve any variables or parameters to their default
	// values.
	evalTags, err := EvalProvisionerTags(varsDefaults, paramsDefaults, tags)
	if err != nil {
		return nil, xerrors.Errorf("eval provisioner tags: %w", err)
	}

	// Ensure that none of the tag values are empty after evaluation.
	for k, v := range evalTags {
		if len(strings.TrimSpace(v)) > 0 {
			continue
		}
		return nil, xerrors.Errorf("provisioner tag %q evaluated to an empty value, please set a default value", k)
	}
	logger.Info(ctx, "found workspace tags", slog.F("tags", evalTags))

	return evalTags, nil
}

func loadModuleFromFile(file []byte, mimetype string) (module *tfconfig.Module, cleanup func() error, err error) {
	// Create a temporary directory
	cleanup = func() error { return nil } // no-op cleanup
	tmpDir, err := os.MkdirTemp("", "tfparse-*")
	if err != nil {
		return nil, cleanup, xerrors.Errorf("create temp dir: %w", err)
	}
	cleanup = func() error { // real cleanup
		return os.RemoveAll(tmpDir)
	}

	// Untar the file into the temporary directory
	var rdr io.Reader
	switch mimetype {
	case "application/x-tar":
		rdr = bytes.NewReader(file)
	case "application/zip":
		zr, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
		if err != nil {
			return nil, cleanup, xerrors.Errorf("read zip file: %w", err)
		}
		tarBytes, err := archive.CreateTarFromZip(zr, maxFileSizeBytes)
		if err != nil {
			return nil, cleanup, xerrors.Errorf("convert zip to tar: %w", err)
		}
		rdr = bytes.NewReader(tarBytes)
	default:
		return nil, cleanup, xerrors.Errorf("unsupported mimetype: %s", mimetype)
	}

	if err := provisionersdk.Untar(tmpDir, rdr); err != nil {
		return nil, cleanup, xerrors.Errorf("untar: %w", err)
	}

	module, diags := tfconfig.LoadModule(tmpDir)
	if diags.HasErrors() {
		return nil, cleanup, xerrors.Errorf("load module: %s", diags.Error())
	}

	return module, cleanup, nil
}

// loadDefaults inspects the given module and returns the default values for
// all variables and coder_parameter data sources referenced there.
func loadDefaults(module *tfconfig.Module) (varsDefaults map[string]string, paramsDefaults map[string]string, err error) {
	// iterate through module.Variables to get the default values for all
	// variables.
	varsDefaults = make(map[string]string)
	for _, v := range module.Variables {
		sv, err := interfaceToString(v.Default)
		if err != nil {
			return nil, nil, xerrors.Errorf("can't convert variable default value to string: %v", err)
		}
		varsDefaults[v.Name] = strings.Trim(sv, `"`)
	}

	// iterate through module.DataResources to get the default values for all
	// coder_parameter data resources.
	paramsDefaults = make(map[string]string)
	for _, dataResource := range module.DataResources {
		if dataResource.Type != "coder_parameter" {
			continue
		}

		var file *hcl.File
		var diags hcl.Diagnostics
		parser := hclparse.NewParser()

		if !strings.HasSuffix(dataResource.Pos.Filename, ".tf") {
			continue
		}

		// We know in which HCL file is the data resource defined.
		file, diags = parser.ParseHCLFile(dataResource.Pos.Filename)
		if diags.HasErrors() {
			return nil, nil, xerrors.Errorf("can't parse the resource file %q: %s", dataResource.Pos.Filename, diags.Error())
		}

		// Parse root to find "coder_parameter".
		content, _, diags := file.Body.PartialContent(rootTemplateSchema)
		if diags.HasErrors() {
			return nil, nil, xerrors.Errorf("can't parse the resource file: %s", diags.Error())
		}

		// Iterate over blocks to locate the exact "coder_parameter" data resource.
		for _, block := range content.Blocks {
			if !slices.Equal(block.Labels, []string{"coder_parameter", dataResource.Name}) {
				continue
			}

			// Parse "coder_parameter" to find the default value.
			resContent, _, diags := block.Body.PartialContent(coderParameterSchema)
			if diags.HasErrors() {
				return nil, nil, xerrors.Errorf(`can't parse the coder_parameter: %s`, diags.Error())
			}

			if _, ok := resContent.Attributes["default"]; !ok {
				paramsDefaults[dataResource.Name] = ""
			} else {
				expr := resContent.Attributes["default"].Expr
				value, err := previewFileContent(expr.Range())
				if err != nil {
					return nil, nil, xerrors.Errorf("can't preview the resource file: %v", err)
				}
				paramsDefaults[dataResource.Name] = strings.Trim(value, `"`)
			}
		}
	}
	return varsDefaults, paramsDefaults, nil
}

// EvalProvisionerTags evaluates the given workspaceTags based on the given
// default values for variables and coder_parameter data sources.
func EvalProvisionerTags(varsDefaults, paramsDefaults, workspaceTags map[string]string) (map[string]string, error) {
	// Filter only allowed data sources for preflight check.
	// This is not strictly required but provides a friendlier error.
	if err := validWorkspaceTagValues(workspaceTags); err != nil {
		return nil, err
	}
	// We only add variables and coder_parameter data sources. Anything else will be
	// undefined and will raise a Terraform error.
	evalCtx := buildEvalContext(varsDefaults, paramsDefaults)
	tags := make(map[string]string)
	for workspaceTagKey, workspaceTagValue := range workspaceTags {
		expr, diags := hclsyntax.ParseExpression([]byte(workspaceTagValue), "expression.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			return nil, xerrors.Errorf("failed to parse workspace tag key %q value %q: %s", workspaceTagKey, workspaceTagValue, diags.Error())
		}

		val, diags := expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, xerrors.Errorf("failed to evaluate workspace tag key %q value %q: %s", workspaceTagKey, workspaceTagValue, diags.Error())
		}

		// Do not use "val.AsString()" as it can panic
		str, err := ctyValueString(val)
		if err != nil {
			return nil, xerrors.Errorf("failed to marshal workspace tag key %q value %q as string: %s", workspaceTagKey, workspaceTagValue, err)
		}
		tags[workspaceTagKey] = str
	}
	return tags, nil
}

// validWorkspaceTagValues returns an error if any value of the given tags map
// evaluates to a datasource other than "coder_parameter".
// This only serves to provide a friendly error if a user attempts to reference
// a data source other than "coder_parameter" in "coder_workspace_tags".
func validWorkspaceTagValues(tags map[string]string) error {
	for _, v := range tags {
		parts := strings.SplitN(v, ".", 3)
		if len(parts) != 3 {
			continue
		}
		if parts[0] == "data" && parts[1] != "coder_parameter" {
			return xerrors.Errorf("invalid workspace tag value %q: only the \"coder_parameter\" data source is supported here", v)
		}
	}
	return nil
}

func buildEvalContext(varDefaults map[string]string, paramDefaults map[string]string) *hcl.EvalContext {
	varDefaultsM := map[string]cty.Value{}
	for varName, varDefault := range varDefaults {
		varDefaultsM[varName] = cty.MapVal(map[string]cty.Value{
			"value": cty.StringVal(varDefault),
		})
	}

	paramDefaultsM := map[string]cty.Value{}
	for paramName, paramDefault := range paramDefaults {
		paramDefaultsM[paramName] = cty.MapVal(map[string]cty.Value{
			"value": cty.StringVal(paramDefault),
		})
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{},
	}
	if len(varDefaultsM) != 0 {
		evalCtx.Variables["var"] = cty.MapVal(varDefaultsM)
	}
	if len(paramDefaultsM) != 0 {
		evalCtx.Variables["data"] = cty.MapVal(map[string]cty.Value{
			"coder_parameter": cty.MapVal(paramDefaultsM),
		})
	}

	return evalCtx
}

var rootTemplateSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "data",
			LabelNames: []string{"type", "name"},
		},
	},
}

var coderWorkspaceTagsSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name: "tags",
		},
	},
}

var coderParameterSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name: "default",
		},
	},
}

func previewFileContent(fileRange hcl.Range) (string, error) {
	body, err := os.ReadFile(fileRange.Filename)
	if err != nil {
		return "", err
	}
	return string(fileRange.SliceBytes(body)), nil
}

// LoadTerraformVariables extracts all Terraform variables from module and converts them
// to template variables. The variables are sorted by source position.
func LoadTerraformVariables(module *tfconfig.Module) ([]*proto.TemplateVariable, error) {
	// Sort variables by (filename, line) to make the ordering consistent
	variables := make([]*tfconfig.Variable, 0, len(module.Variables))
	for _, v := range module.Variables {
		variables = append(variables, v)
	}
	sort.Slice(variables, func(i, j int) bool {
		return compareSourcePos(variables[i].Pos, variables[j].Pos)
	})

	var templateVariables []*proto.TemplateVariable
	for _, v := range variables {
		mv, err := convertTerraformVariable(v)
		if err != nil {
			return nil, err
		}
		templateVariables = append(templateVariables, mv)
	}
	return templateVariables, nil
}

// convertTerraformVariable converts a Terraform variable to a template-wide variable, processed by Coder.
func convertTerraformVariable(variable *tfconfig.Variable) (*proto.TemplateVariable, error) {
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

func compareSourcePos(x, y tfconfig.SourcePos) bool {
	if x.Filename != y.Filename {
		return x.Filename < y.Filename
	}
	return x.Line < y.Line
}

func ctyValueString(val cty.Value) (string, error) {
	switch val.Type() {
	case cty.Bool:
		if val.True() {
			return "true", nil
		} else {
			return "false", nil
		}
	case cty.Number:
		return val.AsBigFloat().String(), nil
	case cty.String:
		return val.AsString(), nil
	// We may also have a map[string]interface{} with key "value".
	case cty.Map(cty.String):
		valval, ok := val.AsValueMap()["value"]
		if !ok {
			return "", xerrors.Errorf("map does not have key 'value'")
		}
		return ctyValueString(valval)
	default:
		return "", xerrors.Errorf("only primitive types are supported - bool, number, and string")
	}
}

func interfaceToString(i interface{}) (string, error) {
	switch v := i.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case int:
		return strconv.FormatInt(int64(v), 10), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return "", xerrors.Errorf("unsupported type %T", v)
	}
}
