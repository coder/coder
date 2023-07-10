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

	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/provisionersdk/proto"
)

// Parse extracts Terraform variables from source-code.
func (s *server) Parse(request *proto.Parse_Request, stream proto.DRPCProvisioner_ParseStream) error {
	_, span := s.startTrace(stream.Context(), tracing.FuncName())
	defer span.End()

	// Load the module and print any parse errors.
	module, diags := tfconfig.LoadModule(request.Directory)
	if diags.HasErrors() {
		return xerrors.Errorf("load module: %s", formatDiagnostics(request.Directory, diags))
	}

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
			return xerrors.Errorf("can't convert the Terraform variable to a managed one: %w", err)
		}
		templateVariables = append(templateVariables, mv)
	}
	return stream.Send(&proto.Parse_Response{
		Type: &proto.Parse_Response_Complete{
			Complete: &proto.Parse_Complete{
				TemplateVariables: templateVariables,
			},
		},
	})
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
