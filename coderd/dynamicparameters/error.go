package dynamicparameters

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/hashicorp/hcl/v2"

	"github.com/coder/coder/v2/codersdk"
)

func parameterValidationError(diags hcl.Diagnostics) *DiagnosticError {
	return &DiagnosticError{
		Message:          "Unable to validate parameters",
		Diagnostics:      diags,
		KeyedDiagnostics: make(map[string]hcl.Diagnostics),
	}
}

func tagValidationError(diags hcl.Diagnostics) *DiagnosticError {
	return &DiagnosticError{
		Message:          "Unable to parse workspace tags",
		Diagnostics:      diags,
		KeyedDiagnostics: make(map[string]hcl.Diagnostics),
	}
}

func presetValidationError(diags hcl.Diagnostics) *DiagnosticError {
	return &DiagnosticError{
		Message:          "Unable to validate presets",
		Diagnostics:      diags,
		KeyedDiagnostics: make(map[string]hcl.Diagnostics),
	}
}

type DiagnosticError struct {
	// Message is the human-readable message that will be returned to the user.
	Message string
	// Diagnostics are top level diagnostics that will be returned as "Detail" in the response.
	Diagnostics hcl.Diagnostics
	// KeyedDiagnostics translate to Validation errors in the response. A key could
	// be a parameter name, or a tag name. This allows diagnostics to be more closely
	// associated with a specific index/parameter/tag.
	KeyedDiagnostics map[string]hcl.Diagnostics
}

// Error is a pretty bad format for these errors. Try to avoid using this.
func (e *DiagnosticError) Error() string {
	var diags hcl.Diagnostics
	diags = diags.Extend(e.Diagnostics)
	for _, d := range e.KeyedDiagnostics {
		diags = diags.Extend(d)
	}

	return diags.Error()
}

func (e *DiagnosticError) HasError() bool {
	if e.Diagnostics.HasErrors() {
		return true
	}

	for _, diags := range e.KeyedDiagnostics {
		if diags.HasErrors() {
			return true
		}
	}
	return false
}

func (e *DiagnosticError) Append(key string, diag *hcl.Diagnostic) {
	e.Extend(key, hcl.Diagnostics{diag})
}

func (e *DiagnosticError) Extend(key string, diag hcl.Diagnostics) {
	if e.KeyedDiagnostics == nil {
		e.KeyedDiagnostics = make(map[string]hcl.Diagnostics)
	}
	if _, ok := e.KeyedDiagnostics[key]; !ok {
		e.KeyedDiagnostics[key] = hcl.Diagnostics{}
	}
	e.KeyedDiagnostics[key] = e.KeyedDiagnostics[key].Extend(diag)
}

func (e *DiagnosticError) Response() (int, codersdk.Response) {
	resp := codersdk.Response{
		Message:     e.Message,
		Validations: nil,
	}

	// Sort the parameter names so that the order is consistent.
	sortedNames := make([]string, 0, len(e.KeyedDiagnostics))
	for name := range e.KeyedDiagnostics {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	for _, name := range sortedNames {
		diag := e.KeyedDiagnostics[name]
		resp.Validations = append(resp.Validations, codersdk.ValidationError{
			Field:  name,
			Detail: DiagnosticsErrorString(diag),
		})
	}

	if e.Diagnostics.HasErrors() {
		resp.Detail = DiagnosticsErrorString(e.Diagnostics)
	}

	return http.StatusBadRequest, resp
}

func DiagnosticErrorString(d *hcl.Diagnostic) string {
	return fmt.Sprintf("%s; %s", d.Summary, d.Detail)
}

func DiagnosticsErrorString(d hcl.Diagnostics) string {
	count := len(d)
	switch {
	case count == 0:
		return "no diagnostics"
	case count == 1:
		return DiagnosticErrorString(d[0])
	default:
		for _, d := range d {
			// Render the first error diag.
			// If there are warnings, do not priority them over errors.
			if d.Severity == hcl.DiagError {
				return fmt.Sprintf("%s, and %d other diagnostic(s)", DiagnosticErrorString(d), count-1)
			}
		}

		// All warnings? ok...
		return fmt.Sprintf("%s, and %d other diagnostic(s)", DiagnosticErrorString(d[0]), count-1)
	}
}
