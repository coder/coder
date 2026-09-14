package dynamicparameters

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/coder/preview"
)

// CheckPresets extracts the preset related diagnostics from a template version preset
func CheckPresets(output *preview.Output, diags hcl.Diagnostics) *DiagnosticError {
	de := presetValidationError(diags)
	if output == nil {
		return de
	}

	presets := output.Presets
	for _, preset := range presets {
		if hcl.Diagnostics(preset.Diagnostics).HasErrors() {
			de.Extend(preset.Name, hcl.Diagnostics(preset.Diagnostics))
		}
	}

	if de.HasError() {
		return de
	}

	return nil
}
