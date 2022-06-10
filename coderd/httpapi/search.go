package httpapi

import (
	"strings"

	"golang.org/x/xerrors"
)

// WorkspaceSearchQuery takes a query string and breaks it into its queryparams
// as a set of key=value.
func WorkspaceSearchQuery(query string) (map[string]string, error) {
	searchParams := make(map[string]string)
	if query == "" {
		return searchParams, nil
	}
	// Because we do this in 2 passes, we want to maintain quotes on the first
	// pass.Further splitting occurs on the second pass and quotes will be
	// dropped.
	elements := splitElements(query, ' ', true)
	for _, element := range elements {
		parts := splitElements(element, ':', false)
		switch len(parts) {
		case 1:
			// No key:value pair. It is a workspace name, and maybe includes an owner
			parts = splitElements(element, '/', false)
			switch len(parts) {
			case 1:
				searchParams["name"] = parts[0]
			case 2:
				searchParams["owner"] = parts[0]
				searchParams["name"] = parts[1]
			default:
				return nil, xerrors.Errorf("Query element %q can only contain 1 '/'", element)
			}
		case 2:
			searchParams[parts[0]] = parts[1]
		default:
			return nil, xerrors.Errorf("Query element %q can only contain 1 ':'", element)
		}
	}

	return searchParams, nil
}

// splitElements takes a query string and splits it into the individual elements
// of the query. Each element is separated by a delimiter. All quoted strings are
// kept as a single element.
//
// Although all our names cannot have spaces, that is a validation error.
// We should still parse the quoted string as a single value so that validation
// can properly fail on the space. If we do not, a value of `template:"my name"`
// will search `template:"my name:name"`, which produces an empty list instead of
// an error.
// nolint:revive
func splitElements(query string, delimiter rune, maintainQuotes bool) []string {
	var parts []string

	quoted := false
	var current strings.Builder
	for _, c := range query {
		switch c {
		case '"':
			if maintainQuotes {
				_, _ = current.WriteRune(c)
			}
			quoted = !quoted
		case delimiter:
			if quoted {
				_, _ = current.WriteRune(c)
			} else {
				parts = append(parts, current.String())
				current = strings.Builder{}
			}
		default:
			_, _ = current.WriteRune(c)
		}
	}
	parts = append(parts, current.String())
	return parts
}
