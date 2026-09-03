package templatebuilder

import (
	"regexp"
	"strings"
)

const (
	// prerequisitesStartMarker delimits the beginning of the prerequisites
	// section inside a base template README.md.
	prerequisitesStartMarker = "<!-- prerequisites:start -->"
	// prerequisitesEndMarker delimits the end of the prerequisites section.
	prerequisitesEndMarker = "<!-- prerequisites:end -->"
)

// ExtractPrerequisites returns the content between the prerequisites
// comment markers in a README body. Returns an empty string when either
// marker is absent.
func ExtractPrerequisites(readme string) string {
	startIdx := strings.Index(readme, prerequisitesStartMarker)
	if startIdx < 0 {
		return ""
	}
	after := readme[startIdx+len(prerequisitesStartMarker):]
	endIdx := strings.Index(after, prerequisitesEndMarker)
	if endIdx < 0 {
		return ""
	}
	return strings.TrimSpace(after[:endIdx])
}

// prerequisiteMarkerLine matches either prerequisites marker on its own
// line, including the trailing newline and an optional following blank
// line, so removing it does not leave an extra gap in the rendered README.
// The marker text is derived from the constants above so there is a single
// source of truth.
var prerequisiteMarkerLine = regexp.MustCompile(
	`(?m)^[ \t]*(?:` +
		regexp.QuoteMeta(prerequisitesStartMarker) + `|` +
		regexp.QuoteMeta(prerequisitesEndMarker) +
		`)[ \t]*\n?\n?`)

// StripPrerequisiteMarkers removes the prerequisites comment markers from a
// README body. The content between the markers is preserved. The markers
// exist only to delimit the prerequisites section for ExtractPrerequisites;
// leaving them in the delivered README causes them to render as literal text
// in the Markdown viewer.
func StripPrerequisiteMarkers(readme string) string {
	return prerequisiteMarkerLine.ReplaceAllString(readme, "")
}
