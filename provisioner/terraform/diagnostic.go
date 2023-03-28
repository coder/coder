package terraform

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
)

// This implementation bases on the original Terraform formatter, which unfortunately is internal:
// https://github.com/hashicorp/terraform/blob/6b35927cf0988262739a5f0acea4790ae58a16d3/internal/command/format/diagnostic.go#L125

func FormatDiagnostic(diag *tfjson.Diagnostic) string {
	var buf bytes.Buffer
	appendSourceSnippets(&buf, diag)
	_, _ = buf.WriteString(diag.Detail)
	return buf.String()
}

func appendSourceSnippets(buf *bytes.Buffer, diag *tfjson.Diagnostic) {
	if diag.Range == nil {
		return
	}

	if diag.Snippet == nil {
		// This should generally not happen, as long as sources are always
		// loaded through the main loader. We may load things in other
		// ways in weird cases, so we'll tolerate it at the expense of
		// a not-so-helpful error message.
		_, _ = fmt.Fprintf(buf, "on %s line %d:\n  (source code not available)\n", diag.Range.Filename, diag.Range.Start.Line)
	} else {
		snippet := diag.Snippet
		code := snippet.Code

		var contextStr string
		if snippet.Context != nil {
			contextStr = fmt.Sprintf(", in %s", *snippet.Context)
		}
		_, _ = fmt.Fprintf(buf, "on %s line %d%s:\n", diag.Range.Filename, diag.Range.Start.Line, contextStr)

		// Split the snippet into lines and render one at a time
		lines := strings.Split(code, "\n")
		for i, line := range lines {
			_, _ = fmt.Fprintf(buf, "  %d: %s\n", snippet.StartLine+i, line)
		}

		if len(snippet.Values) > 0 {
			// The diagnostic may also have information about the dynamic
			// values of relevant variables at the point of evaluation.
			// This is particularly useful for expressions that get evaluated
			// multiple times with different values, such as blocks using
			// "count" and "for_each", or within "for" expressions.
			values := make([]tfjson.DiagnosticExpressionValue, len(snippet.Values))
			copy(values, snippet.Values)
			sort.Slice(values, func(i, j int) bool {
				return values[i].Traversal < values[j].Traversal
			})

			_, _ = buf.WriteString("    ├────────────────\n")
			for _, value := range values {
				_, _ = fmt.Fprintf(buf, "    │ %s %s\n", value.Traversal, value.Statement)
			}
		}
	}
	_ = buf.WriteByte('\n')
}
