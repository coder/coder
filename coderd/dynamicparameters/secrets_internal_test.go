package dynamicparameters

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"

	previewtypes "github.com/coder/preview/types"
)

func TestSecretValidationBlockerCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   hcl.Diagnostics
		want string
	}{
		{
			name: "Empty",
			in:   hcl.Diagnostics{},
			want: "",
		},
		{
			name: "MissingSecretIsNotBlocking",
			in: hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Missing required secrets",
				Extra: previewtypes.DiagnosticExtra{
					Code: DiagCodeMissingSecretEnv,
				},
			}},
			want: "",
		},
		{
			name: "Forbidden",
			in: hcl.Diagnostics{{
				Severity: hcl.DiagWarning,
				Summary:  "Cannot validate secret requirements",
				Extra: previewtypes.DiagnosticExtra{
					Code: DiagCodeSecretValidationForbidden,
				},
			}},
			want: DiagCodeSecretValidationForbidden,
		},
		{
			name: "FetchFailed",
			in: hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Failed to fetch owner secrets",
				Extra: previewtypes.DiagnosticExtra{
					Code: DiagCodeOwnerSecretsFetchFailed,
				},
			}},
			want: DiagCodeOwnerSecretsFetchFailed,
		},
		{
			name: "DiagnosticWithNoExtraIsIgnored",
			in: hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Some other error",
			}},
			want: "",
		},
		{
			name: "MixedKeepsLookingUntilMatch",
			in: hcl.Diagnostics{
				{
					Severity: hcl.DiagError,
					Summary:  "Missing required secrets",
					Extra: previewtypes.DiagnosticExtra{
						Code: DiagCodeMissingSecretEnv,
					},
				},
				{
					Severity: hcl.DiagError,
					Summary:  "Failed to fetch owner secrets",
					Extra: previewtypes.DiagnosticExtra{
						Code: DiagCodeOwnerSecretsFetchFailed,
					},
				},
			},
			want: DiagCodeOwnerSecretsFetchFailed,
		},
		{
			// SetDiagnosticExtra wraps any pre-existing extra into
			// previewtypes.DiagnosticExtra.Wrapped. ExtractDiagnosticExtra
			// walks that chain. A naive type assertion would miss it.
			name: "WrappedExtraIsDetected",
			in: func() hcl.Diagnostics {
				d := &hcl.Diagnostic{
					Severity: hcl.DiagWarning,
					Summary:  "Cannot validate secret requirements",
					Extra:    "some other extra",
				}
				previewtypes.SetDiagnosticExtra(d, previewtypes.DiagnosticExtra{
					Code: DiagCodeSecretValidationForbidden,
				})
				return hcl.Diagnostics{d}
			}(),
			want: DiagCodeSecretValidationForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, secretValidationBlockerCode(tc.in))
		})
	}
}
