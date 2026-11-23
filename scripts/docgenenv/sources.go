package docgenenv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/scripts/atomicwrite"
)

const (
	// SidebarFile is the name of the sidebar source file. The docs root holds
	// one, which lists the manifest versions and the top-level routes, and
	// each docs folder with pages to order holds one next to them.
	SidebarFile = "sidebar.yml"
)

// ManifestIndex is the decoded root SidebarFile. Its field order is the key
// order enforced in the source file.
type ManifestIndex struct {
	Versions []string `yaml:"versions"`
	Routes   []Route  `yaml:"routes"`
}

// Sources is the decoded sidebar source tree under a docs directory.
type Sources struct {
	// Manifest holds the top-level routes with every folder entry resolved.
	// Routes whose children come from a generator are returned unresolved:
	// Folder holds the generated folder, relative to the docs directory, and
	// Children holds only the curated metadata overlays.
	Manifest *Manifest
	// ListedBy maps each page that a hand-written sidebar lists, as a slash
	// path relative to the docs directory, to the folder of the sidebar that
	// lists it.
	ListedBy map[string]string
	// GeneratedDirs lists the folders, relative to the docs directory, whose
	// sidebar file a doc generator writes.
	GeneratedDirs []string

	used map[string]bool
}

// LoadManifestSources reads the sidebar sources under docsDir: the root
// sidebar plus the sidebar of every folder entry it reaches. The doc
// generators use this to read curated metadata without depending on the
// generated sidebar files they are about to rewrite.
func LoadManifestSources(docsDir string) (*Manifest, error) {
	s, err := LoadSources(docsDir)
	if err != nil {
		return nil, err
	}
	return s.Manifest, nil
}

// BuildManifest reads the sidebar sources under docsDir, splices every
// generated sidebar into its route, and validates the result. Every route
// path must name an existing file under docsDir. It fails on any sidebar file
// that nothing references, so a folder can't silently drop out of the
// sidebar.
func BuildManifest(docsDir string) (*Manifest, error) {
	s, err := LoadSources(docsDir)
	if err != nil {
		return nil, err
	}
	m := s.Manifest
	var errs []error
	m.Routes = resolveRoutes(docsDir, m.Routes, nil, s.used, &errs)
	checkPaths(docsDir, m.Routes, nil, &errs)
	errs = append(errs, unreferencedSidebars(docsDir, s.used)...)
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

// LoadSources reads the sidebar sources under docsDir. See Sources.
func LoadSources(docsDir string) (*Sources, error) {
	l := &loader{
		docsDir:  docsDir,
		used:     map[string]bool{SidebarFile: true},
		listedBy: map[string]string{},
	}
	var idx ManifestIndex
	if err := DecodeSourceFile(filepath.Join(docsDir, SidebarFile), &idx); err != nil {
		return nil, err
	}
	if len(idx.Versions) == 0 {
		l.errs = append(l.errs, xerrors.Errorf("%s: versions must list at least one version", SidebarFile))
	}
	if len(idx.Routes) == 0 {
		l.errs = append(l.errs, xerrors.Errorf("%s: routes must list at least one route", SidebarFile))
	}
	m := &Manifest{Versions: idx.Versions}
	for _, e := range idx.Routes {
		if r, ok := l.entry(".", SidebarFile, e, nil, true); ok {
			m.Routes = append(m.Routes, r)
		}
	}
	if len(l.errs) > 0 {
		return nil, errors.Join(l.errs...)
	}
	return &Sources{Manifest: m, ListedBy: l.listedBy, GeneratedDirs: l.generated, used: l.used}, nil
}

type loader struct {
	docsDir   string
	used      map[string]bool // Sidebar files read, relative to docsDir.
	listedBy  map[string]string
	generated []string
	errs      []error
}

// onlyFolder reports whether r is a folder entry: it sets folder and no other
// key, so the route lives in that folder's sidebar file.
func (r Route) onlyFolder() bool {
	return r.Folder != "" && r.Title == "" && r.Description == "" && r.Path == "" && r.IconPath == "" && len(r.State) == 0 && len(r.Children) == 0
}

// entry loads the route for one entry in the sidebar file named file, which
// sits in dir. A folder entry is replaced with the route in that folder's
// sidebar file. top is true for the route a file describes and false for its
// children, which can't have children of their own.
func (l *loader) entry(dir, file string, e Route, crumb []string, top bool) (Route, bool) {
	if e.onlyFolder() {
		return l.folderFile(dir, file, e.Folder, crumb)
	}
	crumb = append(slices.Clip(crumb), e.Title)
	where := file + ": " + breadcrumb(crumb)
	return l.route(dir, file, where, e, crumb, top)
}

// folderFile loads the sidebar file of the folder subfolder of dir. The file
// holds one route and its direct children, with paths relative to the folder.
func (l *loader) folderFile(dir, file, subfolder string, crumb []string) (Route, bool) {
	iw := fmt.Sprintf("%s > folder %q", file, subfolder)
	rel, name, ok := l.claimFolder(dir, subfolder, iw)
	if !ok {
		return Route{}, false
	}
	var r Route
	if err := DecodeSourceFile(filepath.Join(l.docsDir, filepath.FromSlash(name)), &r); err != nil {
		l.errs = append(l.errs, xerrors.Errorf("%s: %w", name, err))
		return Route{}, false
	}
	if r.Folder != "" {
		l.errs = append(l.errs, xerrors.Errorf("%s: folder is allowed only on a child entry", name))
		return Route{}, false
	}
	crumb = append(slices.Clip(crumb), r.Title)
	return l.route(rel, name, name+": "+breadcrumb(crumb), r, crumb, true)
}

// claimFolder validates subfolder as a folder inside dir and marks its sidebar
// file as used. It returns the folder and the sidebar file, both relative to
// the docs directory.
func (l *loader) claimFolder(dir, subfolder, where string) (rel, name string, ok bool) {
	rel = path.Join(dir, subfolder)
	if path.IsAbs(subfolder) || rel == dir || !filepath.IsLocal(filepath.FromSlash(rel)) {
		l.errs = append(l.errs, xerrors.Errorf("%s: folder must be a relative path to a folder inside the docs directory", where))
		return "", "", false
	}
	name = path.Join(rel, SidebarFile)
	if l.used[name] {
		l.errs = append(l.errs, xerrors.Errorf("%s: %s is already listed elsewhere", where, name))
		return "", "", false
	}
	l.used[name] = true
	return rel, name, true
}

// route checks the hand-written route r from the sidebar file named file,
// which sits in dir, and makes its paths and its children's relative to the
// docs directory. A route that sets folder and other keys takes its children
// from the sidebar file a doc generator writes in that folder; its children
// here are metadata overlays.
func (l *loader) route(dir, file, where string, r Route, crumb []string, top bool) (Route, bool) {
	checkSourceRoute(where, r, &l.errs)
	if r.Path != "" {
		p, err := resolveSourcePath(dir, r.Path)
		if err != nil {
			l.errs = append(l.errs, xerrors.Errorf("%s: path %q: %w", where, r.Path, err))
		} else {
			r.Path = p
			if strings.HasSuffix(p, ".md") {
				l.listedBy[strings.TrimPrefix(p, "./")] = dir
			}
		}
	}
	if r.Folder != "" {
		checkOverlays(where, r, &l.errs)
		rel, _, ok := l.claimFolder(dir, r.Folder, where)
		if !ok {
			return Route{}, false
		}
		l.generated = append(l.generated, rel)
		r.Folder = rel
		return r, true
	}
	if !top && len(r.Children) > 0 {
		l.errs = append(l.errs, xerrors.Errorf("%s: a sidebar file holds one level of children; move this route into its own folder", where))
		return r, true
	}
	children := make([]Route, 0, len(r.Children))
	for _, c := range r.Children {
		if child, ok := l.entry(dir, file, c, crumb, false); ok {
			children = append(children, child)
		}
	}
	if len(r.Children) > 0 {
		r.Children = children
	}
	return r, true
}

// resolveSourcePath returns the docs-relative form of page path p written in a
// sidebar file in dir. A path that starts with a dot keeps the "./" prefix and
// a bare path stays bare, so the compiled manifest keeps each path's spelling.
func resolveSourcePath(dir, p string) (string, error) {
	joined := path.Join(dir, p)
	if path.IsAbs(p) || !filepath.IsLocal(filepath.FromSlash(joined)) {
		return "", xerrors.New("must be relative and stay inside the docs directory")
	}
	if strings.HasPrefix(p, ".") {
		return "./" + joined, nil
	}
	return joined, nil
}

// checkSourceRoute checks the keys every hand-edited route needs.
func checkSourceRoute(where string, r Route, errs *[]error) {
	if r.Title == "" {
		*errs = append(*errs, xerrors.Errorf("%s: route is missing a title", where))
	}
	if r.Path == "" {
		*errs = append(*errs, xerrors.Errorf("%s: route is missing a path", where))
	}
	checkDescription(where, "description", r.Description, errs)
}

// checkDescription requires a non-empty page description to end with a
// period. The docs site shows it under the page title and in search and
// social previews, where it reads as a sentence.
func checkDescription(where, field, desc string, errs *[]error) {
	if desc != "" && !strings.HasSuffix(desc, ".") {
		*errs = append(*errs, xerrors.Errorf("%s: %s must end with a period: %q", where, field, desc))
	}
}

// checkOverlays checks a generated route's overlays before its sidebar is
// resolved. With a generated sidebar, children are metadata overlays matched by
// title onto the generated routes, so they can't carry structure of their own.
func checkOverlays(where string, r Route, errs *[]error) {
	for _, o := range r.Children {
		ow := where + " > " + o.Title
		switch {
		case o.Title == "":
			*errs = append(*errs, xerrors.Errorf("%s: overlay is missing a title", where))
		case o.Path != "" || len(o.Children) > 0 || o.Folder != "":
			*errs = append(*errs, xerrors.Errorf("%s: overlays under a generated folder may set only title, description, icon_path, and state", ow))
		}
		checkDescription(ow, "description", o.Description, errs)
	}
}

// resolveRoutes returns routes with every generated sidebar spliced in and its
// overlays applied. Paths in a generated sidebar are relative to its folder.
func resolveRoutes(docsDir string, routes []Route, crumb []string, used map[string]bool, errs *[]error) []Route {
	out := slices.Clone(routes)
	for i := range out {
		r := &out[i]
		c := append(slices.Clip(crumb), r.Title)
		if r.Folder == "" {
			r.Children = resolveRoutes(docsDir, r.Children, c, used, errs)
			continue
		}
		name := path.Join(r.Folder, SidebarFile)
		var generated []Route
		if err := DecodeSourceFile(filepath.Join(docsDir, filepath.FromSlash(name)), &generated); err != nil {
			*errs = append(*errs, xerrors.Errorf("%s: %w", breadcrumb(c), err))
			continue
		}
		validateFragment(name, generated, nil, errs)
		generated = resolveFragmentPaths(r.Folder, generated, name, errs)
		r.Children = applyOverlays(name, generated, r.Children, breadcrumb(c), errs)
		r.Folder = ""
	}
	return out
}

// resolveFragmentPaths makes the paths in a generated sidebar in dir relative
// to the docs directory.
func resolveFragmentPaths(dir string, routes []Route, file string, errs *[]error) []Route {
	out := slices.Clone(routes)
	for i := range out {
		r := &out[i]
		if p, err := resolveSourcePath(dir, r.Path); err != nil {
			*errs = append(*errs, xerrors.Errorf("%s: %s: path %q: %w", file, r.Title, r.Path, err))
		} else {
			r.Path = p
		}
		r.Children = resolveFragmentPaths(dir, r.Children, file, errs)
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
		case r.Folder != "":
			*errs = append(*errs, xerrors.Errorf("%s: %s: a generated sidebar can't set folder", file, breadcrumb(c)))
		}
		checkDescription(file+": "+breadcrumb(c), "description", r.Description, errs)
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
		page := filepath.Join(docsDir, filepath.FromSlash(r.Path))
		info, err := os.Stat(page)
		if err != nil || !info.Mode().IsRegular() {
			*errs = append(*errs, xerrors.Errorf("%s: path %q does not exist under %s", breadcrumb(c), r.Path, docsDir))
		} else {
			checkFrontMatterDescription(page, strings.TrimPrefix(r.Path, "./")+": "+breadcrumb(c), errs)
		}
		checkPaths(docsDir, r.Children, c, errs)
	}
}

// checkFrontMatterDescription checks the description in a Markdown page's
// front matter, which the docs site shows in place of the manifest's.
func checkFrontMatterDescription(page, where string, errs *[]error) {
	if !strings.HasSuffix(page, ".md") {
		return
	}
	b, err := os.ReadFile(page)
	if err != nil {
		*errs = append(*errs, xerrors.Errorf("%s: %w", where, err))
		return
	}
	body, ok := bytes.CutPrefix(b, []byte("---\n"))
	if !ok {
		return
	}
	end := bytes.Index(body, []byte("\n---\n"))
	if end < 0 {
		return
	}
	var fm struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(body[:end+1], &fm); err != nil {
		*errs = append(*errs, xerrors.Errorf("%s: front matter: %w", where, err))
		return
	}
	checkDescription(where, "front matter description", fm.Description, errs)
}

// unreferencedSidebars reports every sidebar file under docsDir that is not in
// used, because no entry names its folder.
func unreferencedSidebars(docsDir string, used map[string]bool) []error {
	var errs []error
	names, err := SidebarFiles(docsDir)
	if err != nil {
		return []error{err}
	}
	for _, name := range names {
		if !used[name] {
			errs = append(errs, xerrors.Errorf("%s: no folder entry in the parent sidebar names this folder", name))
		}
	}
	return errs
}

// SidebarFiles returns every sidebar file under docsDir as a slash path
// relative to docsDir, in lexical order. It skips hidden and node_modules
// directories.
func SidebarFiles(docsDir string) ([]string, error) {
	var names []string
	err := filepath.WalkDir(docsDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != docsDir && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != SidebarFile {
			return nil
		}
		rel, err := filepath.Rel(docsDir, p)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("walk %q: %w", docsDir, err)
	}
	return names, nil
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

func isYAMLFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".yml" || ext == ".yaml"
}

func breadcrumb(crumb []string) string {
	return strings.Join(crumb, " > ")
}
