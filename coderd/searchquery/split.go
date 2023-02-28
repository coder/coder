package searchquery

import "strings"

// splitQueryParameterByDelimiter takes a query string and splits it into the individual elements
// of the query. Each element is separated by a delimiter. All quoted strings are
// kept as a single element.
//
// Although all our names cannot have spaces, that is a validation error.
// We should still parse the quoted string as a single value so that validation
// can properly fail on the space. If we do not, a value of `template:"my name"`
// will search `template:"my name:name"`, which produces an empty list instead of
// an error.
// nolint:revive
func splitQueryParameterByDelimiter(query string, delimiter rune, maintainQuotes bool) []string {
	quoted := false
	parts := strings.FieldsFunc(query, func(r rune) bool {
		if r == '"' {
			quoted = !quoted
		}
		return !quoted && r == delimiter
	})
	if !maintainQuotes {
		for i, part := range parts {
			parts[i] = strings.Trim(part, "\"")
		}
	}

	return parts
}
