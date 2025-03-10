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
	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// NOTE: This is duplicated from coderd but we can't import it here without
// introducing a circular dependency
const maxFileSizeBytes = 10 * (10 << 20) // 10 MB

// parseHCLFiler is the actual interface of *hclparse.Parser we use
// to parse HCL. This is extracted to an interface so we can more
// easily swap this out for an alternative implementation later on.
type parseHCLFiler interface {
	ParseHCLFile(filename string) (*hcl.File, hcl.Diagnostics)
}

// Parser parses a Terraform module on disk.
type Parser struct {
	logger     slog.Logger
	underlying parseHCLFiler
	module     *tfconfig.Module
	workdir    string
}

// Option is an option for a new instance of Parser.
type Option func(*Parser)

// WithLogger sets the logger to be used by Parser
func WithLogger(logger slog.Logger) Option {
	return func(p *Parser) {
		p.logger = logger
	}
}

// New returns a new instance of Parser, as well as any diagnostics
// encountered while parsing the module.
func New(workdir string, opts ...Option) (*Parser, tfconfig.Diagnostics) {
	p := Parser{
		logger:     slog.Make(),
		underlying: hclparse.NewParser(),
		workdir:    workdir,
		module:     nil,
	}
	for _, o := range opts {
		o(&p)
	}

	var diags tfconfig.Diagnostics
	if p.module == nil {
		m, ds := tfconfig.LoadModule(workdir)
		diags = ds
		p.module = m
	}

	return &p, diags
}

// WorkspaceTags looks for all coder_workspace_tags datasource in the module
// and returns the raw values for the tags. It also returns the set of
// variables referenced by any expressions in the raw values of tags.
func (p *Parser) WorkspaceTags(ctx context.Context) (map[string]string, map[string]struct{}, error) {
	tags := map[string]string{}
	skipped := []string{}
	requiredVars := map[string]struct{}{}
	for _, dataResource := range p.module.DataResources {
		if dataResource.Type != "coder_workspace_tags" {
			skipped = append(skipped, strings.Join([]string{"data", dataResource.Type, dataResource.Name}, "."))
			continue
		}

		var file *hcl.File
		var diags hcl.Diagnostics

		if !strings.HasSuffix(dataResource.Pos.Filename, ".tf") {
			continue
		}
		// We know in which HCL file is the data resource defined.
		file, diags = p.underlying.ParseHCLFile(dataResource.Pos.Filename)
		if diags.HasErrors() {
			return nil, nil, xerrors.Errorf("can't parse the resource file: %s", diags.Error())
		}

		// Parse root to find "coder_workspace_tags".
		content, _, diags := file.Body.PartialContent(rootTemplateSchema)
		if diags.HasErrors() {
			return nil, nil, xerrors.Errorf("can't parse the resource file: %s", diags.Error())
		}

		// Iterate over blocks to locate the exact "coder_workspace_tags" data resource.
		for _, block := range content.Blocks {
			if !slices.Equal(block.Labels, []string{"coder_workspace_tags", dataResource.Name}) {
				continue
			}

			// Parse "coder_workspace_tags" to find all key-value tags.
			resContent, _, diags := block.Body.PartialContent(coderWorkspaceTagsSchema)
			if diags.HasErrors() {
				return nil, nil, xerrors.Errorf(`can't parse the resource coder_workspace_tags: %s`, diags.Error())
			}

			if resContent == nil {
				continue // workspace tags are not present
			}

			if _, ok := resContent.Attributes["tags"]; !ok {
				return nil, nil, xerrors.Errorf(`"tags" attribute is required by coder_workspace_tags`)
			}

			expr := resContent.Attributes["tags"].Expr
			tagsExpr, ok := expr.(*hclsyntax.ObjectConsExpr)
			if !ok {
				return nil, nil, xerrors.Errorf(`"tags" attribute is expected to be a key-value map`)
			}

			// Parse key-value entries in "coder_workspace_tags"
			for _, tagItem := range tagsExpr.Items {
				key, err := previewFileContent(tagItem.KeyExpr.Range())
				if err != nil {
					return nil, nil, xerrors.Errorf("can't preview the resource file: %v", err)
				}
				key = strings.Trim(key, `"`)

				value, err := previewFileContent(tagItem.ValueExpr.Range())
				if err != nil {
					return nil, nil, xerrors.Errorf("can't preview the resource file: %v", err)
				}

				if _, ok := tags[key]; ok {
					return nil, nil, xerrors.Errorf(`workspace tag %q is defined multiple times`, key)
				}
				tags[key] = value

				// Find values referenced by the expression.
				refVars := referencedVariablesExpr(tagItem.ValueExpr)
				for _, refVar := range refVars {
					requiredVars[refVar] = struct{}{}
				}
			}
		}
	}

	requiredVarNames := maps.Keys(requiredVars)
	slices.Sort(requiredVarNames)
	p.logger.Debug(ctx, "found workspace tags", slog.F("tags", maps.Keys(tags)), slog.F("skipped", skipped), slog.F("required_vars", requiredVarNames))
	return tags, requiredVars, nil
}

// referencedVariablesExpr determines the variables referenced in expr
// and returns the names of those variables.
func referencedVariablesExpr(expr hclsyntax.Expression) (names []string) {
	var parts []string
	for _, expVar := range expr.Variables() {
		for _, tr := range expVar {
			switch v := tr.(type) {
			case hcl.TraverseRoot:
				parts = append(parts, v.Name)
			case hcl.TraverseAttr:
				parts = append(parts, v.Name)
			default: // skip
			}
		}

		cleaned := cleanupTraversalName(parts)
		names = append(names, strings.Join(cleaned, "."))
	}
	return names
}

// cleanupTraversalName chops off extraneous pieces of the traversal.
// for example:
// - var.foo -> unchanged
// - data.coder_parameter.bar.value -> data.coder_parameter.bar
// - null_resource.baz.zap -> null_resource.baz
func cleanupTraversalName(parts []string) []string {
	if len(parts) == 0 {
		return parts
	}
	if len(parts) > 3 && parts[0] == "data" {
		return parts[:3]
	}
	if len(parts) > 2 {
		return parts[:2]
	}
	return parts
}

func (p *Parser) WorkspaceTagDefaults(ctx context.Context) (map[string]string, error) {
	// This only gets us the expressions. We need to evaluate them.
	// Example: var.region -> "us"
	tags, requiredVars, err := p.WorkspaceTags(ctx)
	if err != nil {
		return nil, xerrors.Errorf("extract workspace tags: %w", err)
	}

	if len(tags) == 0 {
		return map[string]string{}, nil
	}

	// To evaluate the expressions, we need to load the default values for
	// variables and parameters.
	varsDefaults, err := p.VariableDefaults(ctx)
	if err != nil {
		return nil, xerrors.Errorf("load variable defaults: %w", err)
	}
	paramsDefaults, err := p.CoderParameterDefaults(ctx, varsDefaults, requiredVars)
	if err != nil {
		return nil, xerrors.Errorf("load parameter defaults: %w", err)
	}

	// Evaluate the tags expressions given the inputs.
	// This will resolve any variables or parameters to their default
	// values.
	evalTags, err := evaluateWorkspaceTags(varsDefaults, paramsDefaults, tags)
	if err != nil {
		return nil, xerrors.Errorf("eval provisioner tags: %w", err)
	}

	return evalTags, nil
}

// TemplateVariables returns all of the Terraform variables in the module
// as TemplateVariables.
func (p *Parser) TemplateVariables() ([]*proto.TemplateVariable, error) {
	// Sort variables by (filename, line) to make the ordering consistent
	variables := make([]*tfconfig.Variable, 0, len(p.module.Variables))
	for _, v := range p.module.Variables {
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

// WriteArchive is a helper function to write a in-memory archive
// with the given mimetype to disk. Only zip and tar archives
// are currently supported.
func WriteArchive(bs []byte, mimetype string, path string) error {
	// Check if we need to convert the file first!
	var rdr io.Reader
	switch mimetype {
	case "application/x-tar":
		rdr = bytes.NewReader(bs)
	case "application/zip":
		if zr, err := zip.NewReader(bytes.NewReader(bs), int64(len(bs))); err != nil {
			return xerrors.Errorf("read zip file: %w", err)
		} else if tarBytes, err := archive.CreateTarFromZip(zr, maxFileSizeBytes); err != nil {
			return xerrors.Errorf("convert zip to tar: %w", err)
		} else {
			rdr = bytes.NewReader(tarBytes)
		}
	default:
		return xerrors.Errorf("unsupported mimetype: %s", mimetype)
	}

	// Untar the file into the temporary directory
	if err := provisionersdk.Untar(path, rdr); err != nil {
		return xerrors.Errorf("untar: %w", err)
	}

	return nil
}

// VariableDefaults returns the default values for all variables in the module.
func (p *Parser) VariableDefaults(ctx context.Context) (map[string]string, error) {
	// iterate through vars to get the default values for all
	// required variables.
	m := make(map[string]string)
	for _, v := range p.module.Variables {
		if v == nil {
			continue
		}
		sv, err := interfaceToString(v.Default)
		if err != nil {
			return nil, xerrors.Errorf("can't convert variable default value to string: %v", err)
		}
		m[v.Name] = strings.Trim(sv, `"`)
	}
	p.logger.Debug(ctx, "found default values for variables", slog.F("defaults", m))
	return m, nil
}

// CoderParameterDefaults returns the default values of all coder_parameter data sources
// in the parsed module.
func (p *Parser) CoderParameterDefaults(ctx context.Context, varsDefaults map[string]string, names map[string]struct{}) (map[string]string, error) {
	defaultsM := make(map[string]string)
	var (
		skipped []string
		file    *hcl.File
		diags   hcl.Diagnostics
	)

	for _, dataResource := range p.module.DataResources {
		if dataResource == nil {
			continue
		}

		if !strings.HasSuffix(dataResource.Pos.Filename, ".tf") {
			continue
		}

		needle := strings.Join([]string{"data", dataResource.Type, dataResource.Name}, ".")
		if dataResource.Type != "coder_parameter" {
			skipped = append(skipped, needle)
			continue
		}

		if _, found := names[needle]; !found {
			skipped = append(skipped, needle)
			continue
		}

		// We know in which HCL file is the data resource defined.
		// NOTE: hclparse.Parser will cache multiple successive calls to parse the same file.
		file, diags = p.underlying.ParseHCLFile(dataResource.Pos.Filename)
		if diags.HasErrors() {
			return nil, xerrors.Errorf("can't parse the resource file %q: %s", dataResource.Pos.Filename, diags.Error())
		}

		// Parse root to find "coder_parameter".
		content, _, diags := file.Body.PartialContent(rootTemplateSchema)
		if diags.HasErrors() {
			return nil, xerrors.Errorf("can't parse the resource file: %s", diags.Error())
		}

		// Iterate over blocks to locate the exact "coder_parameter" data resource.
		for _, block := range content.Blocks {
			if !slices.Equal(block.Labels, []string{"coder_parameter", dataResource.Name}) {
				continue
			}

			// Parse "coder_parameter" to find the default value.
			resContent, _, diags := block.Body.PartialContent(coderParameterSchema)
			if diags.HasErrors() {
				return nil, xerrors.Errorf(`can't parse the coder_parameter: %s`, diags.Error())
			}

			if _, ok := resContent.Attributes["default"]; !ok {
				p.logger.Warn(ctx, "coder_parameter data source does not have a default value", slog.F("name", dataResource.Name))
				defaultsM[dataResource.Name] = ""
			} else {
				expr := resContent.Attributes["default"].Expr
				value, err := previewFileContent(expr.Range())
				if err != nil {
					return nil, xerrors.Errorf("can't preview the resource file: %v", err)
				}
				// Issue #15795: the "default" value could also be an expression we need
				// to evaluate.
				// TODO: should we support coder_parameter default values that reference other coder_parameter data sources?
				evalCtx := BuildEvalContext(varsDefaults, nil)
				val, diags := expr.Value(evalCtx)
				if diags.HasErrors() {
					return nil, xerrors.Errorf("failed to evaluate coder_parameter %q default value %q: %s", dataResource.Name, value, diags.Error())
				}
				// Do not use "val.AsString()" as it can panic
				strVal, err := CtyValueString(val)
				if err != nil {
					return nil, xerrors.Errorf("failed to marshal coder_parameter %q default value %q as string: %s", dataResource.Name, value, err)
				}
				defaultsM[dataResource.Name] = strings.Trim(strVal, `"`)
			}
		}
	}
	p.logger.Debug(ctx, "found default values for parameters", slog.F("defaults", defaultsM), slog.F("skipped", skipped))
	return defaultsM, nil
}

// evaluateWorkspaceTags evaluates the given workspaceTags based on the given
// default values for variables and coder_parameter data sources.
func evaluateWorkspaceTags(varsDefaults, paramsDefaults, workspaceTags map[string]string) (map[string]string, error) {
	// Filter only allowed data sources for preflight check.
	// This is not strictly required but provides a friendlier error.
	if err := validWorkspaceTagValues(workspaceTags); err != nil {
		return nil, err
	}
	// We only add variables and coder_parameter data sources. Anything else will be
	// undefined and will raise a Terraform error.
	evalCtx := BuildEvalContext(varsDefaults, paramsDefaults)
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
		str, err := CtyValueString(val)
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

// BuildEvalContext builds an evaluation context for the given variable and parameter defaults.
func BuildEvalContext(vars map[string]string, params map[string]string) *hcl.EvalContext {
	varDefaultsM := map[string]cty.Value{}
	for varName, varDefault := range vars {
		varDefaultsM[varName] = cty.MapVal(map[string]cty.Value{
			"value": cty.StringVal(varDefault),
		})
	}

	paramDefaultsM := map[string]cty.Value{}
	for paramName, paramDefault := range params {
		paramDefaultsM[paramName] = cty.MapVal(map[string]cty.Value{
			"value": cty.StringVal(paramDefault),
		})
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{},
		// NOTE: we do not currently support function execution here.
		// The default function map for Terraform is not exposed, so we would essentially
		// have to re-implement or copy the entire map or a subset thereof.
		// ref: https://github.com/hashicorp/terraform/blob/e044e569c5bc81f82e9a4d7891f37c6fbb0a8a10/internal/lang/functions.go#L54
		Functions: Functions(),
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

// CtyValueString converts a cty.Value to a string.
// It supports only primitive types - bool, number, and string.
// As a special case, it also supports map[string]interface{} with key "value".
func CtyValueString(val cty.Value) (string, error) {
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
		return CtyValueString(valval)
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
	default: // just try to JSON-encode it.
		var sb strings.Builder
		if err := json.NewEncoder(&sb).Encode(i); err != nil {
			return "", xerrors.Errorf("convert %T: %w", v, err)
		}
		return strings.TrimSpace(sb.String()), nil
	}
}
