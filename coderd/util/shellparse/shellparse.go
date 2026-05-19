// Package shellparse extracts command steps from shell scripts.
package shellparse

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Parse returns one slice per simple command in src, in source order.
// Each is [program] or [program, arg], where arg is the first
// non-flag positional argument.
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
