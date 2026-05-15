package dynamicparameters

import (
	"net/http"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	previewtypes "github.com/coder/preview/types"
)

func TestDiagnosticErrorResponse(t *testing.T) {
	t.Parallel()

	t.Run("ParameterValidationOnly", func(t *testing.T) {
		t.Parallel()

		de := parameterValidationError(nil)
		de.Append("region", &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "region is required",
		})

		status, resp := de.Response()
		require.Equal(t, http.StatusBadRequest, status)
		require.Len(t, resp.Validations, 1)
		require.Equal(t, "region", resp.Validations[0].Field)
	})

	t.Run("HasErrorWithOnlyMissingSecrets", func(t *testing.T) {
		t.Parallel()

		// A call site that adds only missing-secret diagnostics must
		// still be recognized as a real error so the caller does not
		// proceed past the validation gate.
		de := parameterValidationError(nil)
		de.Append("GITHUB_TOKEN", &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Missing required secret",
			Extra:    previewtypes.DiagnosticExtra{Code: DiagCodeMissingSecretEnv},
		})
		require.True(t, de.HasError())
	})
}

// TestDiagnosticErrorBuildResponse covers the kinded envelope used by
// workspace build endpoints. Generic consumers route through Response;
// only WriteWorkspaceBuildError calls BuildResponse, which propagates
// the per-validation Kind so the frontend can branch.
func TestDiagnosticErrorBuildResponse(t *testing.T) {
	t.Parallel()

	t.Run("ParameterValidationLeavesKindEmpty", func(t *testing.T) {
		t.Parallel()

		de := parameterValidationError(nil)
		de.Append("region", &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "region is required",
		})

		status, resp := de.BuildResponse()
		require.Equal(t, http.StatusBadRequest, status)
		require.Len(t, resp.Validations, 1)
		require.Equal(t, "region", resp.Validations[0].Field)
		require.Empty(t, resp.Validations[0].Kind,
			"parameter validation must leave Kind empty for backward compatibility")
	})

	t.Run("MissingSecretsTranslatedToValidationEntries", func(t *testing.T) {
		t.Parallel()

		de := parameterValidationError(nil)
		de.Append("GITHUB_TOKEN", &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Missing required secret",
			Detail:   "env GITHUB_TOKEN: Create a PAT with env=GITHUB_TOKEN",
			Extra:    previewtypes.DiagnosticExtra{Code: DiagCodeMissingSecretEnv},
		})
		de.Append("/run/secrets/aws", &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Missing required secret",
			Detail:   "file /run/secrets/aws: Mount AWS credentials",
			Extra:    previewtypes.DiagnosticExtra{Code: DiagCodeMissingSecretFile},
		})

		_, resp := de.BuildResponse()
		require.Len(t, resp.Validations, 2)

		// KeyedDiagnostics are sorted by field name. "/run/secrets/aws"
		// sorts before "GITHUB_TOKEN" because '/' < 'G' in ASCII.
		fileEntry := resp.Validations[0]
		require.Equal(t, "/run/secrets/aws", fileEntry.Field)
		require.Equal(t, codersdk.WorkspaceBuildValidationErrorKindMissingSecretFile, fileEntry.Kind)
		require.Contains(t, fileEntry.Detail, "Mount AWS credentials")

		envEntry := resp.Validations[1]
		require.Equal(t, "GITHUB_TOKEN", envEntry.Field)
		require.Equal(t, codersdk.WorkspaceBuildValidationErrorKindMissingSecretEnv, envEntry.Kind)
		require.Contains(t, envEntry.Detail, "Create a PAT with env=GITHUB_TOKEN")
	})

	t.Run("ParametersAndSecretsCoexist", func(t *testing.T) {
		t.Parallel()

		// A response that mixes parameter validations and missing
		// secrets must carry the correct Kind per entry so consumers
		// can route each one to the appropriate UX.
		de := parameterValidationError(nil)
		de.Append("region", &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "region is required",
		})
		de.Append("GITHUB_TOKEN", &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Missing required secret",
			Detail:   "env GITHUB_TOKEN: Create a PAT",
			Extra:    previewtypes.DiagnosticExtra{Code: DiagCodeMissingSecretEnv},
		})

		_, resp := de.BuildResponse()
		require.Len(t, resp.Validations, 2)

		// Entries are sorted by field name. "GITHUB_TOKEN" < "region".
		require.Equal(t, "GITHUB_TOKEN", resp.Validations[0].Field)
		require.Equal(t, codersdk.WorkspaceBuildValidationErrorKindMissingSecretEnv,
			resp.Validations[0].Kind)
		require.Equal(t, "region", resp.Validations[1].Field)
		require.Empty(t, resp.Validations[1].Kind)
	})
}
