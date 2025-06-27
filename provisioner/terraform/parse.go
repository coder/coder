package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/mitchellh/go-wordwrap"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/preview"
)

// Parse extracts Terraform variables from source-code.
func (s *server) Parse(sess *provisionersdk.Session, _ *proto.ParseRequest, _ <-chan struct{}) *proto.ParseComplete {
	ctx := sess.Context()
	_, span := s.startTrace(ctx, tracing.FuncName())
	defer span.End()

	// Load the module and print any parse errors.
	output, diags := preview.Preview(ctx, preview.Input{}, os.DirFS(sess.WorkDirectory))
	if diags.HasErrors() {
		return provisionersdk.ParseErrorf("load module: %s", formatDiagnostics(sess.WorkDirectory, diags))
	}

	tags := output.WorkspaceTags
	failedTags := tags.UnusableTags()
	if len(failedTags) > 0 {
		return provisionersdk.ParseErrorf("can't load workspace tags: %v", failedTags.SafeNames())
	}

	// TODO: THIS
	//templateVariables, err := parser.TemplateVariables()
	//if err != nil {
	//	return provisionersdk.ParseErrorf("can't load template variables: %v", err)
	//}

	return &proto.ParseComplete{
		TemplateVariables: nil, // TODO: Handle template variables.
		WorkspaceTags:     tags.Tags(),
	}
}

// FormatDiagnostics returns a nicely formatted string containing all of the
// error details within the tfconfig.Diagnostics. We need to use this because
// the default format doesn't provide much useful information.
func formatDiagnostics(baseDir string, diags hcl.Diagnostics) string {
	var msgs strings.Builder
	for _, d := range diags {
		// Convert severity.
		severity := "UNKNOWN SEVERITY"
		switch {
		case d.Severity == hcl.DiagError:
			severity = "ERROR"
		case d.Severity == hcl.DiagWarning:
			severity = "WARN"
		}

		// Determine filepath and line
		location := "unknown location"
		if d.Subject != nil {
			filename, err := filepath.Rel(baseDir, d.Subject.Filename)
			if err != nil {
				filename = d.Subject.Filename
			}
			location = fmt.Sprintf("%s:%d", filename, d.Subject.Start.Line)
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
