// Package shellparse extracts command steps from shell scripts.
package shellparse

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Parse returns one slice per simple command in src, in source order.
// Each is [program] or [program, arg], where arg is the first non-flag
// positional argument. Program names are normalized to their base name
// (e.g. /usr/bin/go becomes go).
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
		step := []string{cmdBase(prog)}
		if arg := firstNonFlagLiteral(call.Args[1:]); arg != "" {
			step = append(step, arg)
		}
		out = append(out, step)
		return true
	})
	return out, err
}

// wordLiteral returns the literal content of w by concatenating the
// literal pieces of its parts. Bare literals, single-quoted strings,
// and double-quoted strings (when the inner parts are all literals)
// contribute their text. Any part involving variable expansion,
// command substitution, or arithmetic returns "" for the whole word
// because we cannot resolve those without executing the shell.
func wordLiteral(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			_, _ = sb.WriteString(p.Value)
		case *syntax.SglQuoted:
			_, _ = sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return ""
				}
				_, _ = sb.WriteString(lit.Value)
			}
		default:
			return ""
		}
	}
	return sb.String()
}

// cmdBase returns the base name of a command path, handling both
// forward and back slashes since commands may originate from Windows
// workspaces while this code runs on a Linux server.
func cmdBase(prog string) string {
	if i := strings.LastIndexAny(prog, `/\`); i >= 0 {
		return prog[i+1:]
	}
	return prog
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
