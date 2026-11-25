package main

import (
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

// stateOrigin records the route that first sets a state value.
type stateOrigin struct {
	crumb string
}

// repeatedStates reports every route in the sidebar sources under docsDir
// whose state repeats a value already set on an ancestor route, or lists a
// value more than once. A state applies to all of a route's descendants, so a
// descendant sets only the states its ancestors lack. Folder entries are
// followed, so an ancestor in another sidebar file counts. Children of a
// generated folder are overlays and are checked the same way; the generated
// sidebar itself is not. Call it only after docgenenv.BuildManifest succeeds,
// so every file decodes and every folder entry resolves exactly once.
func repeatedStates(docsDir string) []error {
	m, err := docgenenv.LoadManifestSources(docsDir)
	if err != nil {
		return []error{err}
	}
	var errs []error
	for _, r := range m.Routes {
		checkStateRoute(r, nil, nil, &errs)
	}
	return errs
}

func checkStateRoute(r docgenenv.Route, crumb []string, inherited map[string]stateOrigin, errs *[]error) {
	crumb = append(slices.Clip(crumb), r.Title)
	where := strings.Join(crumb, " > ")
	states := make(map[string]stateOrigin, len(inherited)+len(r.State))
	for k, v := range inherited {
		states[k] = v
	}
	for _, s := range r.State {
		if o, ok := inherited[s]; ok {
			*errs = append(*errs, xerrors.Errorf("%s: state %q is already set on ancestor %q; remove it, because a route inherits its ancestors' states", where, s, o.crumb))
			continue
		}
		if _, ok := states[s]; ok {
			*errs = append(*errs, xerrors.Errorf("%s: state %q is listed more than once", where, s))
			continue
		}
		states[s] = stateOrigin{crumb: where}
	}
	for _, c := range r.Children {
		checkStateRoute(c, crumb, states, errs)
	}
}
