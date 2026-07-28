package templatebuilder

import "strings"

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
