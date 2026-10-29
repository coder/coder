package main

import (
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

// stateOrigin records the route that first sets a state value.
type stateOrigin struct {
	file  string
	crumb string
}

// repeatedStates reports every route in the sidebar sources under dir whose
// state repeats a value already set on an ancestor route. A state applies to
// all of a route's descendants, so a descendant sets only the states its
// ancestors lack. Includes are followed, so an ancestor in another file
// counts. Children of a children_from route are overlays and are checked the
// same way; the generated fragment itself is not. Files that fail to decode
// are skipped, because BuildManifest already reports them.
func repeatedStates(dir string) []error {
	var idx docgenenv.ManifestIndex
	if err := docgenenv.DecodeSourceFile(filepath.Join(dir, docgenenv.ManifestIndexFile), &idx); err != nil {
		return nil
	}
	var errs []error
	for _, section := range idx.Sections {
		checkStateFile(dir, path.Clean(section), nil, nil, map[string]bool{}, &errs)
	}
	return errs
}

func checkStateFile(dir, name string, crumb []string, inherited map[string]stateOrigin, seen map[string]bool, errs *[]error) {
	if seen[name] {
		return
	}
	seen[name] = true
	var r docgenenv.Route
	if err := docgenenv.DecodeSourceFile(filepath.Join(dir, filepath.FromSlash(name)), &r); err != nil {
		return
	}
	checkStateRoute(dir, name, r, crumb, inherited, seen, errs)
}

func checkStateRoute(dir, name string, r docgenenv.Route, crumb []string, inherited map[string]stateOrigin, seen map[string]bool, errs *[]error) {
	crumb = append(crumb[:len(crumb):len(crumb)], r.Title)
	where := strings.Join(crumb, " > ")
	own := make(map[string]stateOrigin, len(inherited)+len(r.State))
	for k, v := range inherited {
		own[k] = v
	}
	for _, s := range r.State {
		if o, ok := inherited[s]; ok {
			*errs = append(*errs, xerrors.Errorf("%s: %s: state %q is already set on ancestor %q in %s; remove it, because a route inherits its ancestors' states", name, where, s, o.crumb, o.file))
			continue
		}
		own[s] = stateOrigin{file: name, crumb: where}
	}
	for _, c := range r.Children {
		if c.Include != "" {
			checkStateFile(dir, path.Clean(c.Include), crumb, own, seen, errs)
			continue
		}
		checkStateRoute(dir, name, c, crumb, own, seen, errs)
	}
}
