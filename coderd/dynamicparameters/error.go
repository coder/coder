package dynamicparameters

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/hashicorp/hcl/v2"

	"github.com/coder/coder/v2/codersdk"
	previewtypes "github.com/coder/preview/types"
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
	// be a parameter name, a tag name, or the env / file name of a missing
	// coder_secret requirement. The diagnostic's Extra carries a
	// previewtypes.DiagnosticExtra whose Code is propagated to the
	// resulting WorkspaceBuildValidationError.Kind when known by
	// BuildResponse.
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

// Response is the generic SDK response that consumers outside the
// workspace build endpoints rely on (tag validation, preset validation,
// chatd diagnostic introspection, etc.). It does not carry per-entry
// kind information; callers that need to distinguish between parameter
// failures and missing coder_secret requirements use BuildResponse.
func (e *DiagnosticError) Response() (int, codersdk.Response) {
	resp := codersdk.Response{
		Message:     e.Message,
		Validations: nil,
	}

	for _, name := range e.sortedKeyedDiagnosticNames() {
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

// BuildResponse emits the kinded envelope used by workspace build
// endpoints. Each keyed diagnostic group is translated to a
// WorkspaceBuildValidationError whose Kind is derived from the first
// error-severity diagnostic's previewtypes.DiagnosticExtra code, when
// known. Top-level diagnostics flow into the response Detail.
func (e *DiagnosticError) BuildResponse() (int, codersdk.WorkspaceBuildErrorResponse) {
	resp := codersdk.WorkspaceBuildErrorResponse{
		Message: e.Message,
	}

	for _, name := range e.sortedKeyedDiagnosticNames() {
		diag := e.KeyedDiagnostics[name]
		resp.Validations = append(resp.Validations, codersdk.WorkspaceBuildValidationError{
			Field:  name,
			Detail: DiagnosticsErrorString(diag),
			Kind:   keyedDiagnosticsKind(diag),
		})
	}

	if e.Diagnostics.HasErrors() {
		resp.Detail = DiagnosticsErrorString(e.Diagnostics)
	}

	return http.StatusBadRequest, resp
}

// sortedKeyedDiagnosticNames returns the keys of KeyedDiagnostics in a
// stable order so that Response and BuildResponse emit entries
// deterministically.
func (e *DiagnosticError) sortedKeyedDiagnosticNames() []string {
	sorted := make([]string, 0, len(e.KeyedDiagnostics))
	for name := range e.KeyedDiagnostics {
		sorted = append(sorted, name)
	}
	slices.Sort(sorted)
	return sorted
}

// keyedDiagnosticsKind iterates the given diagnostics in slice order
// and returns the WorkspaceBuildValidationErrorKind for the first
// error-severity diagnostic whose previewtypes.DiagnosticExtra code
// matches a known missing-secret code. Diagnostics whose severity is
// not hcl.DiagError, and error-severity diagnostics whose Extra code
// is unknown, are skipped. The empty kind is returned when no
// diagnostic in the group carries a known code, which is the default
// for parameter validation entries.
//
// Callers must not insert diagnostics with mixed codes under a single
// key. Today every keyed group comes from exactly one producer
// (appendMissingSecretDiagnostic for missing-secret groups,
// parameter-validation paths for everything else), so the
// "first known code wins" rule is unambiguous; a future producer
// that mixes sources under one key would need to revisit this
// contract.
func keyedDiagnosticsKind(diags hcl.Diagnostics) codersdk.WorkspaceBuildValidationErrorKind {
	for _, d := range diags {
		if d.Severity != hcl.DiagError {
			continue
		}
		extra := previewtypes.ExtractDiagnosticExtra(d)
		switch extra.Code {
		case DiagCodeMissingSecretEnv:
			return codersdk.WorkspaceBuildValidationErrorKindMissingSecretEnv
		case DiagCodeMissingSecretFile:
			return codersdk.WorkspaceBuildValidationErrorKindMissingSecretFile
		}
	}
	return ""
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
