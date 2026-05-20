// Package shellparse extracts command steps from shell scripts.
package shellparse

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Parse returns one slice per simple command in src, in source order.
// Each is [program] or [program, arg], where arg is the first non-flag
// positional argument.
//
// Some malformed inputs (e.g. trailing unterminated tokens after valid
// semicolon-separated commands) yield partial results alongside a
// non-nil error. Callers that show parsed output to users should treat
// a non-nil err as a signal to fall back to the raw input rather than
// display the partial.
func Parse(src string) ([][]string, error) {
	if src == "" {
		return nil, nil
	}
	f, err := syntax.NewParser().Parse(strings.NewReader(src), "")
	if f == nil {
		return nil, err
	}

	var out [][]string
	syntax.Walk(f, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		prog := wordLiteral(call.Args[0])
		if prog == "" {
			return true
		}
		step := []string{prog}
		if arg := firstNonFlagLiteral(call.Args[1:]); arg != "" {
			step = append(step, arg)
		}
		out = append(out, step)
		return true
	})
	return out, err
}

func wordLiteral(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	return w.Lit()
}

// firstNonFlagLiteral returns the literal value of the first word in
// ws that does not start with "-", or "" if none qualifies.
//
// Known limitation: no flag-arity knowledge. For programs whose global
// flags take a separate-word value ("git -C path verb", "kubectl -n ns
// verb", "docker --context X verb"), this returns the flag's value as
// the first positional, not the actual verb. Consumers that need the
// verb in those cases need per-program awareness; this function does
// not provide it.
func firstNonFlagLiteral(ws []*syntax.Word) string {
	for _, w := range ws {
		lit := wordLiteral(w)
		if lit == "" || strings.HasPrefix(lit, "-") {
			continue
		}
		return lit
	}
	return ""
}
