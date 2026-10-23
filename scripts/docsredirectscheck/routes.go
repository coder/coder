package main

import (
	"encoding/json"
	"strings"

	"golang.org/x/xerrors"
)

// docsRepoDir is the directory in the repository that holds the docs pages the
// manifest paths are relative to.
const docsRepoDir = "docs"

// convertPathToRoute turns a manifest page path into the route the docs
// websites serve it at, for example "./admin/external-auth.md" becomes
// "admin/external-auth" and "./ai-coder/index.md" becomes "ai-coder". The docs
// root is the empty route.
//
// It mirrors the docs websites' convertPathToRoute exactly, including its
// quirk: the README.md, index.md and .md suffixes are stripped one after
// another with a plain suffix match, not per path segment, so a file named
// "my-index.md" becomes the route "my-". The check has to agree with the
// websites about which URLs exist, so the quirk is kept on purpose.
func convertPathToRoute(pathStr string) string {
	route := strings.TrimPrefix(pathStr, "./")
	for _, suffix := range []string{"README.md", "index.md", ".md"} {
		route = strings.TrimSuffix(route, suffix)
	}
	route = strings.TrimLeft(route, "/")
	return strings.TrimRight(route, "/")
}

type manifestRoute struct {
	Title    string          `json:"title"`
	Path     string          `json:"path"`
	Children []manifestRoute `json:"children"`
}

type manifest struct {
	Routes []manifestRoute `json:"routes"`
}

func parseManifest(data []byte) (manifest, error) {
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, xerrors.Errorf("invalid JSON: %w", err)
	}
	if m.Routes == nil {
		return manifest{}, xerrors.New(`no "routes" array`)
	}
	return m, nil
}

// routeSet maps each route a docs version serves to the repository file that
// provides it, or "" when no manifest entry does (the docs root).
type routeSet map[string]string

func (s routeSet) has(route string) bool {
	_, ok := s[route]
	return ok
}

// liveRoutes returns every route the docs websites serve for the manifest. The
// websites prerender a page for every manifest entry that has a path, whether
// or not it is shown in the sidebar, so all of them count. The docs root, the
// empty route, always exists.
func liveRoutes(m manifest) routeSet {
	live := routeSet{"": ""}
	var walk func(routes []manifestRoute)
	walk = func(routes []manifestRoute) {
		for _, r := range routes {
			if r.Path != "" {
				route := convertPathToRoute(r.Path)
				if file := docsRepoDir + "/" + strings.TrimPrefix(r.Path, "./"); live[route] == "" {
					live[route] = file
				}
			}
			walk(r.Children)
		}
	}
	walk(m.Routes)
	return live
}

// hyphenated returns a copy of the routes with "." replaced by "-" in every
// route. beta-docs serves dotted segments with hyphens, so a live route that
// equals one of these counts as present.
func (s routeSet) hyphenated() map[string]bool {
	out := make(map[string]bool, len(s))
	for route := range s {
		out[strings.ReplaceAll(route, ".", "-")] = true
	}
	return out
}
