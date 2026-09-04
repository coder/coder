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

// prerequisitesMarkerLine matches either prerequisites marker on its own
// line, including the trailing newline and an optional following blank
// line, so removing it does not leave an extra gap in the rendered README.
var prerequisitesMarkerLine = regexp.MustCompile(
	`(?m)^[ \t]*(?:` +
		regexp.QuoteMeta(prerequisitesStartMarker) + `|` +
		regexp.QuoteMeta(prerequisitesEndMarker) +
		`)[ \t]*\n?\n?`)

// Remove the prerequisites comment markers from a README body,
// preserving the content between the markers.
func StripPrerequisitesMarkers(readme string) string {
	return prerequisitesMarkerLine.ReplaceAllString(readme, "")
}
