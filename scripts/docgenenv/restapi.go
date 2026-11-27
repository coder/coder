package docgenenv

import (
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"golang.org/x/xerrors"
)

// APIReferenceDir is the directory, relative to the docs directory, that holds
// the REST API reference. The docs site renders one page per Swagger tag and a
// schemas page into it from coderd/apidoc/swagger.json when it syncs a version,
// so the repository keeps only the index page and handWrittenAPIPages there.
const APIReferenceDir = "reference/api"

// handWrittenAPIPages are the REST API sidebar entries, other than the index
// page, whose pages live in APIReferenceDir. The docs site keeps them as
// written.
var handWrittenAPIPages = []Route{
	{Title: "Authentication", Path: apiPagePath("authentication")},
}

// apiGeneralTag is the tag the docs site files an operation under when it has
// none, and the first entry in the REST API sidebar.
const apiGeneralTag = "General"

// swaggerMethods are the operation keys of a Swagger path item, as the docs
// site reads them.
var swaggerMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// RESTAPIRoutes returns the REST API reference's sidebar entries for a Swagger
// 2.0 document: one per tag that has an operation without
// `x-apidocgen: {"skip": true}`, the schemas page, and the hand-written pages.
// The docs site files an operation under its first tag, matches these entries
// to the pages it renders by path, and lists those pages in this order, so
// each tag's path uses the site's naming: the tag lowercased, keeping only a-z
// and 0-9. The General entry comes first and the rest follow by title.
func RESTAPIRoutes(swagger []byte) ([]Route, error) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(swagger, &doc); err != nil {
		return nil, xerrors.Errorf("decode the Swagger document: %w", err)
	}

	tags := make(map[string]bool)
	// Walk paths in a fixed order so a bad operation is always the one
	// reported.
	for _, route := range slices.Sorted(maps.Keys(doc.Paths)) {
		for _, method := range swaggerMethods {
			raw, ok := doc.Paths[route][method]
			if !ok {
				continue
			}
			var op struct {
				Tags      []string `json:"tags"`
				APIDocGen struct {
					Skip bool `json:"skip"`
				} `json:"x-apidocgen"`
			}
			if err := json.Unmarshal(raw, &op); err != nil {
				return nil, xerrors.Errorf("decode %s %s: %w", strings.ToUpper(method), route, err)
			}
			if op.APIDocGen.Skip {
				continue
			}
			tag := apiGeneralTag
			if len(op.Tags) > 0 {
				tag = op.Tags[0]
			}
			tags[tag] = true
		}
	}

	schemas := Route{Title: "Schemas", Path: apiPagePath("schemas")}
	routes := append([]Route{schemas}, handWrittenAPIPages...)
	owner := map[string]string{schemas.Path: "the schemas page"}
	for _, r := range handWrittenAPIPages {
		owner[r.Path] = fmt.Sprintf("the hand-written %s page", r.Title)
	}
	for _, tag := range slices.Sorted(maps.Keys(tags)) {
		slug := apiPageSlug(tag)
		if slug == "" {
			return nil, xerrors.Errorf("Swagger tag %q has no a-z or 0-9 characters to name its page by; rename the tag", tag)
		}
		p := apiPagePath(slug)
		if o, ok := owner[p]; ok {
			return nil, xerrors.Errorf("Swagger tag %q would have the page %s, which is %s; rename the tag", tag, p, o)
		}
		owner[p] = fmt.Sprintf("the page of Swagger tag %q", tag)
		routes = append(routes, Route{Title: tag, Path: p})
	}

	slices.SortFunc(routes, func(a, b Route) int {
		switch {
		case a.Title == b.Title:
			return 0
		case a.Title == apiGeneralTag:
			return -1
		case b.Title == apiGeneralTag:
			return 1
		}
		return strings.Compare(a.Title, b.Title)
	})
	return routes, nil
}

// apiPageSlug is the file name, without the extension, that the docs site
// gives the page of the Swagger tag tag.
func apiPageSlug(tag string) string {
	return strings.Map(func(r rune) rune {
		if ('a' <= r && r <= 'z') || ('0' <= r && r <= '9') {
			return r
		}
		return -1
	}, strings.ToLower(tag))
}

// apiPagePath is the manifest path of the REST API reference page named slug.
func apiPagePath(slug string) string {
	return "./" + path.Join(APIReferenceDir, slug+".md")
}

// isRenderedAPIPage reports whether the manifest path p names a page that the
// docs site renders from coderd/apidoc/swagger.json, which therefore has no
// file in the repository. p is in the slash-separated form manifest paths use.
func isRenderedAPIPage(p string) bool {
	clean := path.Clean(p)
	if path.Dir(clean) != APIReferenceDir || path.Ext(clean) != ".md" || path.Base(clean) == "index.md" {
		return false
	}
	return !slices.ContainsFunc(handWrittenAPIPages, func(r Route) bool {
		return path.Clean(r.Path) == clean
	})
}
