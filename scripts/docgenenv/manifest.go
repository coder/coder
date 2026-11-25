package docgenenv

import (
	"bytes"
	"encoding/json"
)

// Route is an individual page object in the docs manifest.json. Per-page
// metadata (title, description, icon_path, state) is mirrored into page front
// matter by the doc generators; the structural fields (path, children) stay in
// the manifest.
//
// The same type decodes the YAML sidebar sources under docs/manifest, so the
// field order here is the key order enforced in both the sources and the
// compiled manifest.json.
type Route struct {
	Title       string   `json:"title,omitempty" yaml:"title,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Path        string   `json:"path,omitempty" yaml:"path,omitempty"`
	IconPath    string   `json:"icon_path,omitempty" yaml:"icon_path,omitempty"`
	State       []string `json:"state,omitempty" yaml:"state,omitempty"`
	// ChildrenFrom names a generated fragment, relative to the sources
	// directory, that supplies this route's children. It exists only in the
	// sources; BuildManifest replaces it with the fragment's routes.
	ChildrenFrom string `json:"-" yaml:"children_from,omitempty"`
	// Include names another source file, relative to the sources directory,
	// that holds this child route. It exists only in the sources, where an
	// include entry sets no other key; the loader replaces the entry with the
	// route in the named file.
	Include  string  `json:"-" yaml:"include,omitempty"`
	Children []Route `json:"children,omitempty" yaml:"children,omitempty"`
}

// Manifest describes the entire documentation index (docs/manifest.json).
type Manifest struct {
	Versions []string `json:"versions,omitempty"`
	Routes   []Route  `json:"routes,omitempty"`
}

// FindRoute walks the manifest, following titles as a breadcrumb from the
// top-level routes, and returns a pointer to the matching route (or nil if any
// title in the path has no match). The returned pointer aliases the manifest,
// so mutating it (for example, replacing Children) updates the manifest in place.
//
// Both documentation generators resolve their target route through this single
// traversal so the route they read metadata from and the route they rewrite
// cannot drift apart.
func (m *Manifest) FindRoute(titles ...string) *Route {
	if m == nil || len(titles) == 0 {
		return nil
	}
	routes := m.Routes
	var match *Route
	for _, title := range titles {
		match = nil
		for i := range routes {
			if routes[i].Title == title {
				match = &routes[i]
				break
			}
		}
		if match == nil {
			return nil
		}
		routes = match.Children
	}
	return match
}

// MarshalJSON renders v as tab-indented JSON with a trailing newline. The
// Makefile runs biome over the result, so this only needs to be stable, not
// final. It leaves &, < and > unescaped so titles such as "Groups & Roles"
// stay readable in the file; both forms decode identically.
func MarshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "\t")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
