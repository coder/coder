package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

// Parse extracts Terraform variables from source-code.
func (s *server) Parse(sess *provisionersdk.Session, _ *proto.ParseRequest, _ <-chan struct{}) *proto.ParseComplete {
	ctx := sess.Context()
	_, span := s.startTrace(ctx, tracing.FuncName())
	defer span.End()

	// Load the module and print any parse errors.
	module, diags := tfconfig.LoadModule(sess.WorkDirectory)
	if diags.HasErrors() {
		return provisionersdk.ParseErrorf("load module: %s", formatDiagnostics(sess.WorkDirectory, diags))
	}

	workspaceTags, err := s.loadWorkspaceTags(ctx, module)
	if err != nil {
		return provisionersdk.ParseErrorf("can't load workspace tags: %v", err)
	}

	templateVariables, err := loadTerraformVariables(module)
	if err != nil {
		return provisionersdk.ParseErrorf("can't load template variables: %v", err)
	}

	return &proto.ParseComplete{
		TemplateVariables: templateVariables,
		WorkspaceTags:     workspaceTags,
	}
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

func (s *server) loadWorkspaceTags(ctx context.Context, module *tfconfig.Module) (map[string]string, error) {
	workspaceTags := map[string]string{}

	for _, dataResource := range module.DataResources {
		if dataResource.Type != "coder_workspace_tags" {
			s.logger.Debug(ctx, "skip resource as it is not a coder_workspace_tags", "resource_name", dataResource.Name, "resource_type", dataResource.Type)
			continue
		}

		var file *hcl.File
		var diags hcl.Diagnostics
		parser := hclparse.NewParser()

		if !strings.HasSuffix(dataResource.Pos.Filename, ".tf") {
			s.logger.Debug(ctx, "only .tf files can be parsed", "filename", dataResource.Pos.Filename)
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

				s.logger.Info(ctx, "workspace tag found", "key", key, "value", value)

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

func loadTerraformVariables(module *tfconfig.Module) ([]*proto.TemplateVariable, error) {
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

// Converts a Terraform variable to a template-wide variable, processed by Coder.
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
