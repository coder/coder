package main

import (
	"encoding/json"
	"strings"

	"golang.org/x/xerrors"
)

// docsRoot is the public path every docs page lives under.
const docsRoot = "/docs"

// convertPathToRoute turns a manifest page path into the URL slug the docs
// website serves it at, for example "./admin/external-auth.md" becomes
// "admin/external-auth" and "./ai-coder/index.md" becomes "ai-coder".
//
// It mirrors the docs website's convertPathToRoute exactly, including its
// quirk: the README.md, index.md and .md suffixes are stripped one after
// another with a plain suffix match, not per path segment, so a file named
// "my-index.md" becomes the slug "my-". The check has to agree with the
// website about which URLs exist, so the quirk is kept on purpose.
func convertPathToRoute(pathStr string) string {
	route := strings.TrimPrefix(pathStr, "./")
	for _, suffix := range []string{"README.md", "index.md", ".md"} {
		route = strings.TrimSuffix(route, suffix)
	}
	route = strings.TrimLeft(route, "/")
	return strings.TrimRight(route, "/")
}

// docsPagePath returns the public path of a manifest page, or false when the
// manifest entry has no page of its own (a section heading).
func docsPagePath(routePath string) (string, bool) {
	if routePath == "" {
		return "", false
	}
	slug := convertPathToRoute(routePath)
	if slug == "" {
		return docsRoot, true
	}
	return docsRoot + "/" + slug, true
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

// liveRoutes returns every public path the docs website serves for the
// manifest. The website prerenders a page for every manifest entry that has a
// path, whether or not it is shown in the sidebar, so all of them count.
//
// Two routes exist without a matching manifest entry of their own: the landing
// page at /docs, and /docs/about, which the website also serves for the root
// page when it is titled "About".
func liveRoutes(m manifest) map[string]bool {
	live := map[string]bool{docsRoot: true}
	var walk func(routes []manifestRoute)
	walk = func(routes []manifestRoute) {
		for _, r := range routes {
			if p, ok := docsPagePath(r.Path); ok {
				live[p] = true
				if p == docsRoot && r.Title == "About" {
					live[docsRoot+"/about"] = true
				}
			}
			walk(r.Children)
		}
	}
	walk(m.Routes)
	return live
}
