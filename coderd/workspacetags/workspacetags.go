package workspacetags

import (
	"bytes"
	"context"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"cdr.dev/slog"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisionersdk"
)

// Validate ensures that any uses of the `coder_workspace_tags` data source only
// reference the following data types:
// 1. Static variables
// 2. Template variables
// 3. Coder parameters
// Any other data types are not allowed, as their values cannot be known at
// the time of template import.
func Validate(ctx context.Context, logger slog.Logger, file []byte, mimetype string) (tags map[string]string, err error) {
	// TODO(cian): logic already exists in provisioner/terraform/parse.go for
	// this. Can we reuse it?

	// TODO(cian): we need to detect if there are missing UserVariableValues.
	// This is normally done by the provisioner, but we need to do it here.
	// EDIT: maybe hclsyntax.ParseExpression does this for us?

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "workspacetags-validate")
	if err != nil {
		return nil, xerrors.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Untar the file into the temporary directory
	var rdr io.Reader
	switch mimetype {
	case "application/x-tar":
		rdr = bytes.NewReader(file)
	case "application/zip":
		// TODO: convert to tar
		return nil, xerrors.Errorf("todo: convert zip to tar")
	default:
		return nil, xerrors.Errorf("unsupported mimetype: %s", mimetype)
	}

	if err := provisionersdk.Untar(tmpDir, rdr); err != nil {
		return nil, xerrors.Errorf("untar: %w", err)
	}

	module, diags := tfconfig.LoadModule(tmpDir)
	if diags.HasErrors() {
		return nil, xerrors.Errorf("load module: %s", diags.Error())
	}

	// TODO: this only gets us the expressions. We need to evaluate them.
	// Example: var.region -> "us"
	tags, err = loadWorkspaceTags(ctx, logger, module)

	varsDefaults, paramsDefaults, err := loadDefaults(ctx, logger, module)
	if err != nil {
		return nil, xerrors.Errorf("load defaults: %w", err)
	}

	evalContext := buildEvalContext(varsDefaults, paramsDefaults)

	// Filter only allowed data sources for preflight check.
	if err := validWorkspaceTagValues(tags); err != nil {
		return nil, err
	}

	evalTags, err := evalProvisionerTags(evalContext, tags)
	if err != nil {
		return nil, xerrors.Errorf("eval provisioner tags: %w", err)
	}
	return evalTags, err
}

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

func loadDefaults(ctx context.Context, logger slog.Logger, module *tfconfig.Module) (varsDefaults map[string]string, paramsDefaults map[string]string, err error) {
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
			logger.Debug(ctx, "skip resource as it is not a coder_parameter", "resource_name", dataResource.Name, "resource_type", dataResource.Type)
		}

		var file *hcl.File
		var diags hcl.Diagnostics
		parser := hclparse.NewParser()

		if !strings.HasSuffix(dataResource.Pos.Filename, ".tf") {
			logger.Debug(ctx, "only .tf files can be parsed", "filename", dataResource.Pos.Filename)
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
				return nil, nil, xerrors.Errorf(`can't parse the resource coder_parameter: %s`, diags.Error())
			}

			if _, ok := resContent.Attributes["default"]; !ok {
				return nil, nil, xerrors.Errorf(`"default" attribute is required by coder_parameter %q`, dataResource.Name)
			}

			expr := resContent.Attributes["default"].Expr
			value, err := previewFileContent(expr.Range())
			if err != nil {
				return nil, nil, xerrors.Errorf("can't preview the resource file: %v", err)
			}

			paramsDefaults[dataResource.Name] = strings.Trim(value, `"`)
		}
	}
	return varsDefaults, paramsDefaults, nil
}

// --- BEGIN COPYPASTA FROM provisioner/terraform/parse.go --- //
func loadWorkspaceTags(ctx context.Context, logger slog.Logger, module *tfconfig.Module) (map[string]string, error) {
	workspaceTags := map[string]string{}

	// Now we have all the default values for variables and coder_parameters.
	// We can use them to evaluate the workspace tags.
	for _, dataResource := range module.DataResources {
		if dataResource.Type != "coder_workspace_tags" {
			logger.Debug(ctx, "skip resource as it is not a coder_workspace_tags", "resource_name", dataResource.Name, "resource_type", dataResource.Type)
			continue
		}

		var file *hcl.File
		var diags hcl.Diagnostics
		parser := hclparse.NewParser()

		if !strings.HasSuffix(dataResource.Pos.Filename, ".tf") {
			logger.Debug(ctx, "only .tf files can be parsed", "filename", dataResource.Pos.Filename)
			continue
		}
		// We know in which HCL file is the data resource defined.
		file, diags = parser.ParseHCLFile(dataResource.Pos.Filename)

		if diags.HasErrors() {
			return nil, xerrors.Errorf("can't parse the resource file %q: %s", dataResource.Pos.Filename, diags.Error())
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

				logger.Info(ctx, "workspace tag found", "key", key, "value", value)

				if _, ok := workspaceTags[key]; ok {
					return nil, xerrors.Errorf(`workspace tag "%s" is defined multiple times`, key)
				}
				workspaceTags[key] = value
			}
		}
	}
	return workspaceTags, nil
}

func previewFileContent(fileRange hcl.Range) (string, error) {
	body, err := os.ReadFile(fileRange.Filename)
	if err != nil {
		return "", err
	}
	return string(fileRange.SliceBytes(body)), nil
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

// -- BEGIN COPYPASTA FROM coderd/wsbuilder/wsbuilder.go -- //
func evalProvisionerTags(evalCtx *hcl.EvalContext, workspaceTags map[string]string) (map[string]string, error) {
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

type BuildError struct {
	// Status is a suitable HTTP status code
	Status  int
	Message string
	Wrapped error
}

func (e BuildError) Error() string {
	return e.Wrapped.Error()
}

func (e BuildError) Unwrap() error {
	return e.Wrapped
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

// -- END COPYPASTA FROM coderd/wsbuilder/wsbuilder.go -- //

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
