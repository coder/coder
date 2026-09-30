package docgenenv

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/scripts/atomicwrite"
)

const (
	// ManifestSourcesDir is the directory, relative to the docs directory,
	// that holds the hand-edited YAML sidebar sources compiled into
	// manifest.json.
	ManifestSourcesDir = "manifest"
	// ManifestIndexFile is the sources file that lists the manifest versions
	// and the top-level section files in sidebar order.
	ManifestIndexFile = "index.yml"
)

// ManifestIndex is the decoded ManifestIndexFile. Its field order is the key
// order enforced in the source file.
type ManifestIndex struct {
	Versions []string `yaml:"versions"`
	Sections []string `yaml:"sections"`
}

// LoadManifestSources reads the sidebar sources in dir: the index plus one
// top-level route per section file. Routes that set children_from are returned
// unresolved, so their Children hold only the curated metadata overlays. The
// doc generators use this to read curated metadata without depending on the
// generated fragments they are about to rewrite.
func LoadManifestSources(dir string) (*Manifest, error) {
	m, _, err := loadSources(dir)
	return m, err
}

// BuildManifest reads the sidebar sources in dir, splices every children_from
// fragment into its route, and validates the result. When docsDir is not
// empty, every route path must name an existing file under it. It fails on any
// file in dir that nothing references, so a section can't silently drop out
// of the sidebar.
func BuildManifest(dir, docsDir string) (*Manifest, error) {
	m, used, err := loadSources(dir)
	if err != nil {
		return nil, err
	}
	var errs []error
	m.Routes = resolveRoutes(dir, m.Routes, nil, used, &errs)
	if docsDir != "" {
		checkPaths(docsDir, m.Routes, nil, &errs)
	}
	errs = append(errs, unreferencedFiles(dir, used)...)
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return m, nil
}

// WriteRouteFragment writes routes to the generated fragment at path, as YAML
// when the extension is .yml or .yaml and as JSON otherwise.
func WriteRouteFragment(path string, routes []Route) error {
	var (
		b   []byte
		err error
	)
	if isYAMLFile(path) {
		b, err = MarshalYAML(routes)
	} else {
		b, err = MarshalJSON(routes)
	}
	if err != nil {
		return xerrors.Errorf("marshal fragment %q: %w", path, err)
	}
	if err := atomicwrite.File(path, b); err != nil {
		return xerrors.Errorf("write fragment %q: %w", path, err)
	}
	return nil
}

// MarshalYAML renders v as YAML with two-space indentation.
func MarshalYAML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func loadSources(dir string) (*Manifest, map[string]bool, error) {
	used := map[string]bool{ManifestIndexFile: true}
	var idx ManifestIndex
	if err := DecodeSourceFile(filepath.Join(dir, ManifestIndexFile), &idx); err != nil {
		return nil, nil, err
	}
	var errs []error
	if len(idx.Versions) == 0 {
		errs = append(errs, xerrors.Errorf("%s: versions must list at least one version", ManifestIndexFile))
	}
	if len(idx.Sections) == 0 {
		errs = append(errs, xerrors.Errorf("%s: sections must list at least one section file", ManifestIndexFile))
	}
	m := &Manifest{Versions: idx.Versions}
	for _, section := range idx.Sections {
		name, err := sourceRel(section)
		if err != nil {
			errs = append(errs, xerrors.Errorf("%s: section %q: %w", ManifestIndexFile, section, err))
			continue
		}
		if !isYAMLFile(name) {
			errs = append(errs, xerrors.Errorf("%s: section %q must be a .yml file", ManifestIndexFile, section))
			continue
		}
		if used[name] {
			errs = append(errs, xerrors.Errorf("%s: section %q is listed more than once", ManifestIndexFile, section))
			continue
		}
		used[name] = true
		var r Route
		if err := DecodeSourceFile(filepath.Join(dir, name), &r); err != nil {
			errs = append(errs, err)
			continue
		}
		validateSourceRoute(name, r, nil, &errs)
		m.Routes = append(m.Routes, r)
	}
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}
	return m, used, nil
}

// validateSourceRoute checks the rules that hold for hand-edited routes before
// any fragment is resolved.
func validateSourceRoute(file string, r Route, crumb []string, errs *[]error) {
	crumb = append(slices.Clip(crumb), r.Title)
	where := file + ": " + breadcrumb(crumb)
	if r.Title == "" {
		*errs = append(*errs, xerrors.Errorf("%s: route is missing a title", where))
	}
	if r.Path == "" {
		*errs = append(*errs, xerrors.Errorf("%s: route is missing a path", where))
	}
	if r.ChildrenFrom == "" {
		for _, c := range r.Children {
			validateSourceRoute(file, c, crumb, errs)
		}
		return
	}
	if _, err := sourceRel(r.ChildrenFrom); err != nil {
		*errs = append(*errs, xerrors.Errorf("%s: children_from %q: %w", where, r.ChildrenFrom, err))
	}
	// With children_from, children are metadata overlays matched by title
	// onto the generated routes, so they can't carry structure of their own.
	for _, o := range r.Children {
		ow := where + " > " + o.Title
		switch {
		case o.Title == "":
			*errs = append(*errs, xerrors.Errorf("%s: overlay is missing a title", where))
		case o.Path != "" || len(o.Children) > 0 || o.ChildrenFrom != "":
			*errs = append(*errs, xerrors.Errorf("%s: overlays under children_from may set only title, description, icon_path, and state", ow))
		}
	}
}

// resolveRoutes returns routes with every children_from fragment spliced in
// and its overlays applied.
func resolveRoutes(dir string, routes []Route, crumb []string, used map[string]bool, errs *[]error) []Route {
	out := slices.Clone(routes)
	for i := range out {
		r := &out[i]
		c := append(slices.Clip(crumb), r.Title)
		if r.ChildrenFrom == "" {
			r.Children = resolveRoutes(dir, r.Children, c, used, errs)
			continue
		}
		name, _ := sourceRel(r.ChildrenFrom) // Validated by loadSources.
		used[name] = true
		var generated []Route
		if err := DecodeSourceFile(filepath.Join(dir, name), &generated); err != nil {
			*errs = append(*errs, xerrors.Errorf("%s: %w", breadcrumb(c), err))
			continue
		}
		validateFragment(name, generated, nil, errs)
		r.Children = applyOverlays(name, generated, r.Children, breadcrumb(c), errs)
		r.ChildrenFrom = ""
	}
	return out
}

func validateFragment(file string, routes []Route, crumb []string, errs *[]error) {
	for _, r := range routes {
		c := append(slices.Clip(crumb), r.Title)
		switch {
		case r.Title == "":
			*errs = append(*errs, xerrors.Errorf("%s: %s: route is missing a title", file, breadcrumb(c)))
		case r.Path == "":
			*errs = append(*errs, xerrors.Errorf("%s: %s: route is missing a path", file, breadcrumb(c)))
		case r.ChildrenFrom != "":
			*errs = append(*errs, xerrors.Errorf("%s: %s: fragments can't nest children_from", file, breadcrumb(c)))
		}
		validateFragment(file, r.Children, c, errs)
	}
}

// applyOverlays fills empty metadata on the generated routes from the curated
// overlays with the same title. A field set on both sides must match, so the
// manifest and the generated page front matter can't disagree. An overlay
// that matches no generated route is an error rather than silently dropped
// metadata.
func applyOverlays(file string, generated, overlays []Route, where string, errs *[]error) []Route {
	byTitle := make(map[string]Route, len(overlays))
	for _, o := range overlays {
		if _, dup := byTitle[o.Title]; dup {
			*errs = append(*errs, xerrors.Errorf("%s: overlay %q is listed more than once", where, o.Title))
		}
		byTitle[o.Title] = o
	}
	matched := make(map[string]bool, len(overlays))
	out := slices.Clone(generated)
	for i := range out {
		g := &out[i]
		o, ok := byTitle[g.Title]
		if !ok {
			continue
		}
		matched[g.Title] = true
		ow := where + " > " + g.Title
		fill := func(field string, dst *string, src string) {
			switch {
			case src == "":
			case *dst == "":
				*dst = src
			case *dst != src:
				*errs = append(*errs, xerrors.Errorf("%s: overlay %s %q conflicts with generated %q in %s", ow, field, src, *dst, file))
			}
		}
		fill("description", &g.Description, o.Description)
		fill("icon_path", &g.IconPath, o.IconPath)
		switch {
		case len(o.State) == 0:
		case len(g.State) == 0:
			g.State = o.State
		case !slices.Equal(g.State, o.State):
			*errs = append(*errs, xerrors.Errorf("%s: overlay state %q conflicts with generated %q in %s", ow, o.State, g.State, file))
		}
	}
	for _, o := range overlays {
		if !matched[o.Title] {
			*errs = append(*errs, xerrors.Errorf("%s: overlay %q matches no route in %s; rename or remove it", where, o.Title, file))
		}
	}
	return out
}

func checkPaths(docsDir string, routes []Route, crumb []string, errs *[]error) {
	for _, r := range routes {
		c := append(slices.Clip(crumb), r.Title)
		info, err := os.Stat(filepath.Join(docsDir, filepath.FromSlash(r.Path)))
		if err != nil || !info.Mode().IsRegular() {
			*errs = append(*errs, xerrors.Errorf("%s: path %q does not exist under %s", breadcrumb(c), r.Path, docsDir))
		}
		checkPaths(docsDir, r.Children, c, errs)
	}
}

func unreferencedFiles(dir string, used map[string]bool) []error {
	var errs []error
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel = filepath.ToSlash(rel); !used[rel] {
			errs = append(errs, xerrors.Errorf("%s: not listed in %s sections or any children_from", rel, ManifestIndexFile))
		}
		return nil
	})
	if err != nil {
		errs = append(errs, xerrors.Errorf("walk %q: %w", dir, err))
	}
	return errs
}

// DecodeSourceFile strictly decodes one sidebar source or fragment into v:
// YAML for .yml and .yaml files and JSON otherwise. Unknown keys, extra
// documents, and trailing data are errors. In YAML, a comment on the same line
// as a title or description is rejected, because an unquoted " #" there
// silently truncates the value.
func DecodeSourceFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return xerrors.Errorf("read %q: %w", path, err)
	}
	name := filepath.Base(path)
	if !isYAMLFile(path) {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(v); err != nil {
			return xerrors.Errorf("%s: %w", name, err)
		}
		if _, err := dec.Token(); !errors.Is(err, io.EOF) {
			return xerrors.Errorf("%s: unexpected data after the top-level value", name)
		}
		return nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return xerrors.Errorf("%s: file is empty", name)
		}
		return xerrors.Errorf("%s: %w", name, err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return xerrors.Errorf("%s: file must contain a single YAML document", name)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return xerrors.Errorf("%s: %w", name, err)
	}
	var errs []error
	checkProseComments(name, &doc, &errs)
	return errors.Join(errs...)
}

func checkProseComments(file string, n *yaml.Node, errs *[]error) {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if (k.Value == "title" || k.Value == "description") && v.Kind == yaml.ScalarNode && (k.LineComment != "" || v.LineComment != "") {
				*errs = append(*errs, xerrors.Errorf("%s: line %d: %s has a trailing comment; if the value contains \" #\", quote it", file, v.Line, k.Value))
			}
		}
	}
	for _, c := range n.Content {
		checkProseComments(file, c, errs)
	}
}

// sourceRel validates a sources-relative file name and returns it in slash
// form.
func sourceRel(name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if name == "" || !filepath.IsLocal(clean) {
		return "", xerrors.New("must be a relative path inside the sources directory")
	}
	return filepath.ToSlash(clean), nil
}

func isYAMLFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".yml" || ext == ".yaml"
}

func breadcrumb(crumb []string) string {
	return strings.Join(crumb, " > ")
}
