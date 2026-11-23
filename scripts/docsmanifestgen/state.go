package main

import (
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

// stateOrigin records the route that first sets a state value.
type stateOrigin struct {
	page  string
	crumb string
}

// repeatedStates reports every route in the built manifest whose state repeats
// a value already set on an ancestor route, or lists a value more than once. A
// state applies to all of a route's descendants, so a descendant sets only the
// states its ancestors lack. A route's states come from its page's front
// matter, so each error names that page.
func repeatedStates(m *docgenenv.Manifest) []error {
	var errs []error
	for i := range m.Routes {
		checkStateRoute(m.Routes[i], nil, nil, &errs)
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
			*errs = append(*errs, xerrors.Errorf("%s: %s: state %q is already set on ancestor %q in %s; remove it, because a route inherits its ancestors' states", r.Path, where, s, o.crumb, o.page))
			continue
		}
		if _, ok := states[s]; ok {
			*errs = append(*errs, xerrors.Errorf("%s: %s: state %q is listed more than once", r.Path, where, s))
			continue
		}
		states[s] = stateOrigin{page: r.Path, crumb: where}
	}
	for _, c := range r.Children {
		checkStateRoute(c, crumb, states, errs)
	}
}
