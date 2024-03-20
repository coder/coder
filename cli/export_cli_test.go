//nolint:testpackage // Exports needed to test internal functions without exposing.
package cli

var (
	ExportNewPrettyErrorFormatter = newPrettyErrorFormatter
	ExportFormat                  = (*ExportPrettyErrorFormatter).format
)

type ExportPrettyErrorFormatter = prettyErrorFormatter
