package terraform

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/mitchellh/go-wordwrap"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisioner/terraform/tfparse"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

// Parse extracts Terraform variables from source-code.
func (s *server) Parse(sess *provisionersdk.Session, _ *proto.ParseRequest, _ <-chan struct{}) *proto.ParseComplete {
	ctx := sess.Context()
	_, span := s.startTrace(ctx, tracing.FuncName())
	defer span.End()

	// Load the module and print any parse errors.
	parser, diags := tfparse.New(sess.WorkDirectory, tfparse.WithLogger(s.logger.Named("tfparse")))
	if diags.HasErrors() {
		return provisionersdk.ParseErrorf("load module: %s", formatDiagnostics(sess.WorkDirectory, diags))
	}

	workspaceTags, _, err := parser.WorkspaceTags(ctx)
	if err != nil {
		return provisionersdk.ParseErrorf("can't load workspace tags: %v", err)
	}

	templateVariables, err := parser.TemplateVariables()
	if err != nil {
		return provisionersdk.ParseErrorf("can't load template variables: %v", err)
	}

	return &proto.ParseComplete{
		TemplateVariables: templateVariables,
		WorkspaceTags:     workspaceTags,
	}
}

// FormatDiagnostics returns a nicely formatted string containing all of the
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
