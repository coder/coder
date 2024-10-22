package terraform

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/workspacetags"
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

	// NOTE: we load workspace tags before loading terraform variables.
	// We do not need to know the actual values here, just the expressions.
	workspaceTags, err := workspacetags.LoadWorkspaceTags(ctx, s.logger, module)
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
