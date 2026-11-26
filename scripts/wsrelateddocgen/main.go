// Command wsrelateddocgen generates the workspace related data reference at
// docs/reference/workspace-related-data.md from the wsrelated struct tags on
// codersdk.Workspace and its sub-structs. The page documents the
// include_related query parameter and lists, for each response field, the
// paths that return it, so it stays in sync as fields and tags change.
package main

import (
	"bytes"
	"cmp"
	_ "embed"
	"flag"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"text/template"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsrelated"
	"github.com/coder/coder/v2/scripts/atomicwrite"
	"github.com/coder/coder/v2/scripts/docgenenv"
	"github.com/coder/flog"
)

// routeTitles is the sidebar breadcrumb to this page's route. The page
// mirrors that route's metadata into its front matter, so the sidebar sources
// stay the single source of the title and description.
var routeTitles = []string{"Reference", "Workspace related data"}

// notReturned labels a field that the include_related endpoints never
// populate.
const notReturned = "Not returned"

// exampleIncludeRelated is the include_related value the page uses as its
// example. render validates it, so the example can't drift from the paths the
// API accepts.
const exampleIncludeRelated = "template,latest_build.resources.agents"

//go:embed page.md.tmpl
var pageTemplateSource string

var pageTemplate = template.Must(template.New("page").Parse(pageTemplateSource))

// row is one line of the response fields table.
type row struct {
	// Field is the JSON path of the response field.
	Field string
	// Paths describes the include_related paths that return the field.
	Paths string
}

func main() {
	sourcesDir := flag.String("manifest-sources", filepath.Join("docs", docgenenv.ManifestSourcesDir), "directory of the docs sidebar sources that supply the page metadata")
	out := flag.String("out", filepath.Join("docs", "reference", "workspace-related-data.md"), "path to write the generated reference page")
	flag.Parse()

	if err := run(*sourcesDir, *out); err != nil {
		flog.Fatalf("%v", err)
	}
	flog.Successf("wrote %s", *out)
}

func run(sourcesDir, out string) error {
	manifest, err := docgenenv.LoadManifestSources(sourcesDir)
	if err != nil {
		return err
	}
	route := manifest.FindRoute(routeTitles...)
	if route == nil {
		return xerrors.Errorf("sidebar sources %q have no route %q", sourcesDir, strings.Join(routeTitles, " > "))
	}

	content, err := render(*route)
	if err != nil {
		return xerrors.Errorf("render workspace related data reference: %w", err)
	}
	if err := atomicwrite.File(out, []byte(content)); err != nil {
		return xerrors.Errorf("write %s: %w", out, err)
	}
	return nil
}

func render(route docgenenv.Route) (string, error) {
	if _, err := wsrelated.Parse(exampleIncludeRelated); err != nil {
		return "", xerrors.Errorf("example include_related value %q: %w", exampleIncludeRelated, err)
	}
	rows, err := fieldRows(reflect.TypeFor[codersdk.Workspace]())
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = pageTemplate.Execute(&buf, struct {
		Header  string
		Example string
		Paths   []string
		Rows    []row
	}{
		Header:  docgenenv.GeneratedHeader(route),
		Example: exampleIncludeRelated,
		Paths:   includeRelatedPaths(reflect.TypeFor[codersdk.Workspace]()),
		Rows:    rows,
	})
	if err != nil {
		return "", xerrors.Errorf("execute page template: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n") + "\n", nil
}

// fieldRows returns one row per response field reachable from root, grouped
// by the object that holds the field and sorted by name within each object.
//
// A field whose struct has its own wsrelated tags is described by those
// fields instead of its own row. A struct with no wsrelated tags is covered by
// the row of the field that holds it. An untagged field of a tagged struct is
// never populated, so it is marked as not returned.
func fieldRows(root reflect.Type) ([]row, error) {
	var tagged []wsrelated.Field
	for _, f := range wsrelated.Fields(root) {
		if !f.Inherited {
			tagged = append(tagged, f)
		}
	}

	var rows []row
	seen := map[string]bool{}
	for _, f := range tagged {
		isContainer := slices.ContainsFunc(tagged, func(g wsrelated.Field) bool {
			return strings.HasPrefix(g.JSONPath, f.JSONPath+".") || strings.HasPrefix(g.JSONPath, f.JSONPath+"[].")
		})
		if !isContainer {
			paths, err := describeTag(f)
			if err != nil {
				return nil, err
			}
			rows = append(rows, row{Field: f.JSONPath, Paths: paths})
		}

		parent, _ := splitJSONPath(f.JSONPath)
		prefix := ""
		if parent != "" {
			prefix = parent + "."
		}
		for i := range f.Struct.NumField() {
			sf := f.Struct.Field(i)
			if _, ok := sf.Tag.Lookup(wsrelated.TagKey); ok || !sf.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = sf.Name
			}
			if p := prefix + name; !seen[p] {
				seen[p] = true
				rows = append(rows, row{Field: p, Paths: notReturned})
			}
		}
	}

	slices.SortFunc(rows, func(a, b row) int {
		aParent, aName := splitJSONPath(a.Field)
		bParent, bName := splitJSONPath(b.Field)
		return cmp.Or(cmp.Compare(aParent, bParent), cmp.Compare(aName, bName))
	})
	return rows, nil
}

// includeRelatedPaths returns every include_related path that some field
// reachable from root is tagged with, sorted so each path follows its
// ancestors. fieldRows validates the same tags.
func includeRelatedPaths(root reflect.Type) []string {
	var paths []string
	for _, f := range wsrelated.Fields(root) {
		if f.Tag == "" {
			continue
		}
		for _, p := range strings.Split(f.Tag, ",") {
			if !slices.Contains(paths, p) {
				paths = append(paths, p)
			}
		}
	}
	slices.Sort(paths)
	return paths
}

// describeTag renders the include_related paths in f's tag as a table cell.
// It rejects a tag that include_related would not accept, so a typo in a tag
// fails generation instead of publishing a path the API rejects.
func describeTag(f wsrelated.Field) (string, error) {
	if f.Tag == "" {
		return "Always returned", nil
	}
	if _, err := wsrelated.Parse(f.Tag); err != nil {
		return "", xerrors.Errorf("field %s has an invalid %s tag %q: %w", f.JSONPath, wsrelated.TagKey, f.Tag, err)
	}
	paths := strings.Split(f.Tag, ",")
	for i, p := range paths {
		paths[i] = "`" + p + "`"
	}
	return strings.Join(paths, " and "), nil
}

// splitJSONPath splits a dotted JSON path into the path of the object that
// holds the field and the field's name.
func splitJSONPath(path string) (parent, name string) {
	i := strings.LastIndex(path, ".")
	if i < 0 {
		return "", path
	}
	return path[:i], path[i+1:]
}
