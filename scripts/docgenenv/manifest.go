package docgenenv

import (
	"encoding/json"
	"os"

	"golang.org/x/xerrors"
)

// Route is an individual page object in the docs manifest.json. Per-page
// metadata (title, description, icon_path, state) is mirrored into page front
// matter by the doc generators; the structural fields (path, children) stay in
// the manifest.
type Route struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Path        string   `json:"path,omitempty"`
	IconPath    string   `json:"icon_path,omitempty"`
	State       []string `json:"state,omitempty"`
	Children    []Route  `json:"children,omitempty"`
}

// Manifest describes the entire documentation index (docs/manifest.json).
type Manifest struct {
	Versions []string `json:"versions,omitempty"`
	Routes   []Route  `json:"routes,omitempty"`
}

// LoadManifest reads and unmarshals the manifest.json at path. Its errors wrap
// the path and cause, so callers should return the error as-is rather than
// wrapping it again.
func LoadManifest(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, xerrors.Errorf("read manifest %q: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, xerrors.Errorf("unmarshal manifest %q: %w", path, err)
	}
	return &m, nil
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
