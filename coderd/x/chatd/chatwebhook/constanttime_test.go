package chatwebhook_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecretComparisonsAreConstantTime fails when the package compares
// secrets or signatures in variable time. A compare that exits at the first
// differing byte gives the same answers as a constant-time one, so no
// request-level test can tell them apart. This guard reads the source
// instead. Comparisons must go through crypto/subtle or hmac.Equal.
func TestSecretComparisonsAreConstantTime(t *testing.T) {
	t.Parallel()

	variableTimeCalls := map[string]bool{
		"bytes.Equal":       true,
		"bytes.Compare":     true,
		"strings.Compare":   true,
		"strings.EqualFold": true,
		"slices.Equal":      true,
		"reflect.DeepEqual": true,
	}
	// A comparison against a literal, nil, or a length cannot compare two
	// secrets.
	isBenign := func(e ast.Expr) bool {
		switch e := e.(type) {
		case *ast.BasicLit:
			return true
		case *ast.Ident:
			return e.Name == "nil" || e.Name == "true" || e.Name == "false"
		case *ast.CallExpr:
			id, ok := e.Fun.(*ast.Ident)
			return ok && id.Name == "len"
		}
		return false
	}

	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		checked++
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		require.NoError(t, err)

		ast.Inspect(parsed, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				sel, ok := n.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if ok && variableTimeCalls[pkg.Name+"."+sel.Sel.Name] {
					t.Errorf("%s: %s.%s compares in variable time", fset.Position(n.Pos()), pkg.Name, sel.Sel.Name)
				}
			case *ast.BinaryExpr:
				if (n.Op == token.EQL || n.Op == token.NEQ) && !isBenign(n.X) && !isBenign(n.Y) {
					t.Errorf("%s: == or != between two values; compare secrets with crypto/subtle or hmac.Equal", fset.Position(n.Pos()))
				}
			}
			return true
		})
	}
	require.NotZero(t, checked, "the guard must find the package's source files")
}
