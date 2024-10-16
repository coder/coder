package workspacetags

import (
	"bytes"
	"context"
	"io"
	"os"
	"slices"
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

	evalTags, err := evalProvisionerTags(tags, nil, nil)
	if err != nil {
		return nil, xerrors.Errorf("eval provisioner tags: %w", err)
	}
	return evalTags, err
}

// --- BEGIN COPYPASTA FROM provisioner/terraform/parse.go --- //
func loadWorkspaceTags(ctx context.Context, logger slog.Logger, module *tfconfig.Module) (map[string]string, error) {
	workspaceTags := map[string]string{}

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

// -- BEGIN COPYPASTA FROM coderd/wsbuilder/wsbuilder.go -- //
func evalProvisionerTags(workspaceTags map[string]string, parameterNames, parameterValues []string) (map[string]string, error) {
	tags := make(map[string]string)
	evalCtx := buildParametersEvalContext(parameterNames, parameterValues)
	for workspaceTagKey, workspaceTagValue := range workspaceTags {
		expr, diags := hclsyntax.ParseExpression([]byte(workspaceTagValue), "expression.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			return nil, xerrors.Errorf("failed to parse workspace tag key %q value %q: %w", workspaceTagKey, workspaceTagValue, diags.Error())
		}

		val, diags := expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, xerrors.Errorf("failed to evaluate workspace tag key %q value %q: %w", workspaceTagKey, workspaceTagValue, diags.Error())
		}

		// Do not use "val.AsString()" as it can panic
		str, err := ctyValueString(val)
		if err != nil {
			return nil, xerrors.Errorf("failed to marshal workspace tag key %q value %q as string: %w", workspaceTagKey, workspaceTagValue, diags.Error())
		}
		tags[workspaceTagKey] = str
	}
	return tags, nil
}

func buildParametersEvalContext(names, values []string) *hcl.EvalContext {
	m := map[string]cty.Value{}
	for i, name := range names {
		m[name] = cty.MapVal(map[string]cty.Value{
			"value": cty.StringVal(values[i]),
		})
	}

	if len(m) == 0 {
		return nil // otherwise, panic: must not call MapVal with empty map
	}

	return &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"data": cty.MapVal(map[string]cty.Value{
				"coder_parameter": cty.MapVal(m),
			}),
		},
	}
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
	default:
		return "", xerrors.Errorf("only primitive types are supported - bool, number, and string")
	}
}
