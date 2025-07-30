package dynamicparameters

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"

	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
)

func CheckTags(output *preview.Output, diags hcl.Diagnostics) *DiagnosticError {
	de := tagValidationError(diags)
	if output == nil {
		return de
	}

	failedTags := output.WorkspaceTags.UnusableTags()
	if len(failedTags) == 0 && !de.HasError() {
		return nil // No errors, all is good!
	}

	for _, tag := range failedTags {
		name := tag.KeyString()
		if name == previewtypes.UnknownStringValue {
			name = "unknown" // Best effort to get a name for the tag
		}
		de.Extend(name, failedTagDiagnostic(tag))
	}
	return de
}

// failedTagDiagnostic is a helper function that takes an invalid tag and
// returns an appropriate hcl diagnostic for it.
func failedTagDiagnostic(tag previewtypes.Tag) hcl.Diagnostics {
	const (
		key   = "key"
		value = "value"
	)

	diags := hcl.Diagnostics{}

	// TODO: It would be really nice to pull out the variable references to help identify the source of
	// the unknown or invalid tag.
	unknownErr := "Tag %s is not known, it likely refers to a variable that is not set or has no default."
	invalidErr := "Tag %s is not valid, it must be a non-null string value."

	if !tag.Key.Value.IsWhollyKnown() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf(unknownErr, key),
		})
	} else if !tag.Key.Valid() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf(invalidErr, key),
		})
	}

	if !tag.Value.Value.IsWhollyKnown() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf(unknownErr, value),
		})
	} else if !tag.Value.Valid() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf(invalidErr, value),
		})
	}

	if diags.HasErrors() {
		// Stop here if there are diags, as the diags manually created above are more
		// informative than the original tag's diagnostics.
		return diags
	}

	// If we reach here, decorate the original tag's diagnostics
	diagErr := "Tag %s: %s"
	if tag.Key.ValueDiags.HasErrors() {
		// add 'Tag key' prefix to each diagnostic
		for _, d := range tag.Key.ValueDiags {
			d.Summary = fmt.Sprintf(diagErr, key, d.Summary)
		}
	}
	diags = diags.Extend(tag.Key.ValueDiags)

	if tag.Value.ValueDiags.HasErrors() {
		// add 'Tag value' prefix to each diagnostic
		for _, d := range tag.Value.ValueDiags {
			d.Summary = fmt.Sprintf(diagErr, value, d.Summary)
		}
	}
	diags = diags.Extend(tag.Value.ValueDiags)

	if !diags.HasErrors() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Tag is invalid for some unknown reason. Please check the tag's value and key.",
		})
	}

	return diags
}
