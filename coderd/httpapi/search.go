package httpapi

import (
	"strings"

	"golang.org/x/xerrors"
)

// WorkspaceSearchQuery takes a query string and breaks it into it's queryparams
// as a set of key=value.
func WorkspaceSearchQuery(query string) (map[string]string, error) {
	searchParams := make(map[string]string)
	if query == "" {
		return searchParams, nil
	}
	elements := splitElements(query, ' ')
	for _, element := range elements {
		parts := splitElements(query, ':')
		switch len(parts) {
		case 1:
			// No key:value pair. It is a workspace name, and maybe includes an owner
			parts = splitElements(query, '/')
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
func splitElements(query string, delimiter rune) []string {
	var parts []string

	quoted := false
	var current strings.Builder
	for _, c := range query {
		switch c {
		case '"':
			quoted = !quoted
		case delimiter:
			if quoted {
				current.WriteRune(c)
			} else {
				parts = append(parts, current.String())
				current = strings.Builder{}
			}
		default:
			current.WriteRune(c)
		}
	}
	parts = append(parts, current.String())
	return parts
}
