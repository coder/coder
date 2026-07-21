package docgenenv

import "strings"

// FrontMatter renders r's metadata (title plus any description, icon_path, and
// state) as a YAML front matter block, including the opening and closing fences
// and the blank line that separates the block from the Markdown body. Optional
// fields are omitted when empty.
//
// Both documentation generators emit their front matter through this single
// function so a new per-page metadata field is wired in one place instead of
// drifting between the CLI template and the API generator. Structural manifest
// fields (path, children) are intentionally not mirrored here.
func FrontMatter(r Route) string {
	// strings.Builder and bytes.Buffer expose only error-returning Write
	// methods, which the revive unhandled-error linter flags, so assemble the
	// block as a []string and join it.
	lines := []string{
		"---",
		"title: " + YAMLScalar(r.Title),
	}
	if r.Description != "" {
		lines = append(lines, "description: "+YAMLScalar(r.Description))
	}
	if r.IconPath != "" {
		lines = append(lines, "icon_path: "+YAMLScalar(r.IconPath))
	}
	if len(r.State) > 0 {
		lines = append(lines, "state:")
		for _, s := range r.State {
			lines = append(lines, "  - "+YAMLScalar(s))
		}
	}
	// The trailing empty strings produce the closing fence followed by the
	// blank line that must separate front matter from the body.
	lines = append(lines, "---", "", "")
	return strings.Join(lines, "\n")
}
