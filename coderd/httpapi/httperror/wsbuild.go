package httperror

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/hcl/v2"

	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
)

func WriteWorkspaceBuildError(ctx context.Context, rw http.ResponseWriter, err error) {
	var buildErr wsbuilder.BuildError
	if errors.As(err, &buildErr) {
		if httpapi.IsUnauthorizedError(err) {
			buildErr.Status = http.StatusForbidden
		}

		httpapi.Write(ctx, rw, buildErr.Status, codersdk.Response{
			Message: buildErr.Message,
			Detail:  buildErr.Error(),
		})
		return
	}

	var parameterErr *dynamicparameters.ResolverError
	if errors.As(err, &parameterErr) {
		resp := codersdk.Response{
			Message:     "Unable to validate parameters",
			Validations: nil,
		}

		for name, diag := range parameterErr.Parameter {
			resp.Validations = append(resp.Validations, codersdk.ValidationError{
				Field:  name,
				Detail: DiagnosticsErrorString(diag),
			})
		}

		if parameterErr.Diagnostics.HasErrors() {
			resp.Detail = DiagnosticsErrorString(parameterErr.Diagnostics)
		}

		httpapi.Write(ctx, rw, http.StatusBadRequest, resp)
		return
	}

	httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
		Message: "Internal error creating workspace build.",
		Detail:  err.Error(),
	})
}

func DiagnosticError(d *hcl.Diagnostic) string {
	return fmt.Sprintf("%s; %s", d.Summary, d.Detail)
}

func DiagnosticsErrorString(d hcl.Diagnostics) string {
	count := len(d)
	switch {
	case count == 0:
		return "no diagnostics"
	case count == 1:
		return DiagnosticError(d[0])
	default:
		for _, d := range d {
			// Render the first error diag.
			// If there are warnings, do not priority them over errors.
			if d.Severity == hcl.DiagError {
				return fmt.Sprintf("%s, and %d other diagnostic(s)", DiagnosticError(d), count-1)
			}
		}

		// All warnings? ok...
		return fmt.Sprintf("%s, and %d other diagnostic(s)", DiagnosticError(d[0]), count-1)
	}
}
