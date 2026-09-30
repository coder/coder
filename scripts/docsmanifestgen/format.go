package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/scripts/atomicwrite"
	"github.com/coder/coder/v2/scripts/docgenenv"
)

// routeKeyOrder and indexKeyOrder are the canonical key orders for source
// routes and the index file. They match the field order of docgenenv.Route
// and docgenenv.ManifestIndex, which TestKeyOrderMatchesStructs enforces.
var (
	routeKeyOrder = []string{"title", "description", "path", "icon_path", "state", "children_from", "include", "children"}
	indexKeyOrder = []string{"versions", "sections"}
)

// formattedFile is a source file whose canonical form differs from its
// contents on disk.
type formattedFile struct {
	rel  string // Sources-relative name, for messages.
	path string
	out  []byte // Canonical contents.
}

// unformattedSources returns every YAML file under dir whose contents differ
// from its canonical form, without modifying anything.
func unformattedSources(dir string) ([]formattedFile, error) {
	var (
		changed []formattedFile
		errs    []error
	)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isYAML(path) {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		in, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out, err := formatFile(rel, in)
		if err != nil {
			errs = append(errs, xerrors.Errorf("%s: %w", rel, err))
			return nil
		}
		if !bytes.Equal(in, out) {
			changed = append(changed, formattedFile{rel: rel, path: path, out: out})
		}
		return nil
	})
	if err != nil {
		errs = append(errs, xerrors.Errorf("format %q: %w", dir, err))
	}
	return changed, errors.Join(errs...)
}

// formatSources rewrites every YAML file under dir in its canonical form.
func formatSources(dir string) error {
	changed, err := unformattedSources(dir)
	if err != nil {
		return err
	}
	for _, f := range changed {
		if err := atomicwrite.File(f.path, f.out); err != nil {
			return xerrors.Errorf("write %s: %w", f.rel, err)
		}
	}
	return nil
}

// formatFile returns the canonical form of one source file: keys in struct
// order, two-space indentation, string lists such as state inline, and every
// scalar plain unless quoting is needed to keep it a string. Comments are
// preserved and move with the key they are attached to.
func formatFile(rel string, in []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(in, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, xerrors.New("file must contain a single YAML document")
	}
	root := doc.Content[0]
	var err error
	switch {
	case rel == docgenenv.ManifestIndexFile:
		err = formatIndex(root)
	case root.Kind == yaml.MappingNode:
		err = formatRoute(root)
	case root.Kind == yaml.SequenceNode:
		err = formatRouteList(root)
	default:
		err = xerrors.New("top level must be a route or a list of routes")
	}
	if err != nil {
		return nil, err
	}
	return docgenenv.MarshalYAML(&doc)
}

func formatIndex(n *yaml.Node) error {
	if err := sortMapping(n, indexKeyOrder); err != nil {
		return err
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		v := n.Content[i+1]
		style := yaml.Style(0)
		if n.Content[i].Value == "versions" {
			style = yaml.FlowStyle
		}
		if err := formatStringList(v, style); err != nil {
			return xerrors.Errorf("%s: %w", n.Content[i].Value, err)
		}
	}
	return nil
}

func formatRouteList(n *yaml.Node) error {
	if n.Kind != yaml.SequenceNode {
		return xerrors.Errorf("line %d: expected a list of routes", n.Line)
	}
	n.Style = 0
	for _, c := range n.Content {
		if err := formatRoute(c); err != nil {
			return err
		}
	}
	return nil
}

func formatRoute(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return xerrors.Errorf("line %d: expected a route", n.Line)
	}
	n.Style = 0
	if err := sortMapping(n, routeKeyOrder); err != nil {
		return err
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		var err error
		switch k.Value {
		case "state":
			err = formatStringList(v, yaml.FlowStyle)
		case "children":
			err = formatRouteList(v)
		default:
			err = formatString(v)
		}
		if err != nil {
			return xerrors.Errorf("%s: %w", k.Value, err)
		}
	}
	return nil
}

// sortMapping reorders a mapping's key/value pairs into order. Unknown and
// duplicate keys are errors.
func sortMapping(n *yaml.Node, order []string) error {
	if n.Kind != yaml.MappingNode {
		return xerrors.Errorf("line %d: expected a mapping", n.Line)
	}
	type pair struct{ k, v *yaml.Node }
	pairs := make([]pair, 0, len(n.Content)/2)
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		if !slices.Contains(order, k.Value) {
			return xerrors.Errorf("line %d: unknown key %q", k.Line, k.Value)
		}
		if seen[k.Value] {
			return xerrors.Errorf("line %d: duplicate key %q", k.Line, k.Value)
		}
		seen[k.Value] = true
		if err := formatString(k); err != nil {
			return err
		}
		pairs = append(pairs, pair{k, n.Content[i+1]})
	}
	slices.SortStableFunc(pairs, func(a, b pair) int {
		return slices.Index(order, a.k.Value) - slices.Index(order, b.k.Value)
	})
	n.Content = n.Content[:0]
	for _, p := range pairs {
		n.Content = append(n.Content, p.k, p.v)
	}
	return nil
}

func formatStringList(n *yaml.Node, style yaml.Style) error {
	if n.Kind != yaml.SequenceNode {
		return xerrors.Errorf("line %d: expected a list", n.Line)
	}
	n.Style = style
	for _, c := range n.Content {
		if err := formatString(c); err != nil {
			return err
		}
	}
	return nil
}

// formatString makes n a string scalar that is plain when the plain form reads
// back as the same string, and double-quoted otherwise. That quotes values
// YAML would read as another type (no, 123, null) or change (": ", " #").
func formatString(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return xerrors.Errorf("line %d: expected a string", n.Line)
	}
	n.Tag = "!!str"
	n.Style = 0
	var got any
	if err := yaml.Unmarshal([]byte(n.Value), &got); err != nil || got != n.Value {
		n.Style = yaml.DoubleQuotedStyle
	}
	return nil
}

func isYAML(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".yml" || ext == ".yaml"
}
