package main

import (
	"path"
	"path/filepath"
	"slices"
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
// state repeats a value already set on an ancestor route, or lists a value
// more than once. A state applies to all of a route's descendants, so a
// descendant sets only the states its ancestors lack. Includes are followed,
// so an ancestor in another file counts. Children of a children_from route
// are overlays and are checked the same way; the generated fragment itself is
// not. Call it only after docgenenv.BuildManifest succeeds, so every file
// decodes and every include resolves exactly once.
func repeatedStates(dir string) []error {
	var idx docgenenv.ManifestIndex
	if err := docgenenv.DecodeSourceFile(filepath.Join(dir, docgenenv.ManifestIndexFile), &idx); err != nil {
		return []error{err}
	}
	var errs []error
	for _, section := range idx.Sections {
		checkStateFile(dir, path.Clean(section), nil, nil, &errs)
	}
	return errs
}

func checkStateFile(dir, name string, crumb []string, inherited map[string]stateOrigin, errs *[]error) {
	var r docgenenv.Route
	if err := docgenenv.DecodeSourceFile(filepath.Join(dir, filepath.FromSlash(name)), &r); err != nil {
		*errs = append(*errs, err)
		return
	}
	checkStateRoute(dir, name, r, crumb, inherited, errs)
}

func checkStateRoute(dir, name string, r docgenenv.Route, crumb []string, inherited map[string]stateOrigin, errs *[]error) {
	crumb = append(slices.Clip(crumb), r.Title)
	where := strings.Join(crumb, " > ")
	states := make(map[string]stateOrigin, len(inherited)+len(r.State))
	for k, v := range inherited {
		states[k] = v
	}
	for _, s := range r.State {
		if o, ok := inherited[s]; ok {
			*errs = append(*errs, xerrors.Errorf("%s: %s: state %q is already set on ancestor %q in %s; remove it, because a route inherits its ancestors' states", name, where, s, o.crumb, o.file))
			continue
		}
		if _, ok := states[s]; ok {
			*errs = append(*errs, xerrors.Errorf("%s: %s: state %q is listed more than once", name, where, s))
			continue
		}
		states[s] = stateOrigin{file: name, crumb: where}
	}
	for _, c := range r.Children {
		if c.Include != "" {
			checkStateFile(dir, path.Clean(c.Include), crumb, states, errs)
			continue
		}
		checkStateRoute(dir, name, c, crumb, states, errs)
	}
}
