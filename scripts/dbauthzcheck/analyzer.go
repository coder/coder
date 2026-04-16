// Package main implements a go/analysis analyzer that flags calls to
// functions from the dbauthz package whose name starts with "As". These
// helpers wrap a context with an elevated authorization subject (roughly
// "sudo for the database"), so every call site should carry an explicit
// justification.
//
// The analyzer replaces the dbauthzAuthorizationContext ruleguard rule
// that previously lived in scripts/rules.go.
package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// dbauthzPkgPath is the import path of the dbauthz package whose As*
// helpers we want to flag at every call site.
const dbauthzPkgPath = "github.com/coder/coder/v2/coderd/database/dbauthz"

// suppressionDirective is the comment token that opts a specific call
// site out of this analyzer. We use a dedicated prefix instead of
// //nolint:... because the analyzer runs outside golangci-lint and
// golangci-lint's nolintlint checker can flag unknown //nolint names.
const suppressionDirective = "dbauthzcheck:ignore"

// Analyzer reports unsuppressed dbauthz.As* calls that elevate the
// authorization context.
var Analyzer = &analysis.Analyzer{
	Name: "dbauthzcheck",
	Doc: "report calls to dbauthz.As* that elevate authorization context " +
		"without an explicit //" + suppressionDirective + " suppression",
	Run: run,
	// ResultType must be set so run can return a typed nil instead
	// of nil, nil — which the nilnil linter forbids. No downstream
	// analyzer depends on this result.
	ResultType: reflect.TypeOf((*struct{})(nil)),
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if isTestFile(pass.Fset, file) {
			continue
		}

		suppressed := suppressedLines(pass.Fset, file)

		// We carry an explicit stack of ancestor nodes so we can find
		// the enclosing statement for each call. Suppression comments
		// typically sit above the statement containing the call, not
		// above the call itself (which may be a nested argument).
		var stack []ast.Node
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				// End of a node's children: pop.
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				return false
			}

			if call, ok := n.(*ast.CallExpr); ok {
				checkCall(pass, call, stack, suppressed)
			}

			stack = append(stack, n)
			return true
		})
	}

	return (*struct{})(nil), nil
}

// checkCall reports the call site if it resolves to a dbauthz.As*
// helper, takes a single context argument, and no enclosing line in
// the statement or its leading comment group carries a suppression.
func checkCall(pass *analysis.Pass, call *ast.CallExpr, stack []ast.Node, suppressed map[int]bool) {
	fn, ok := dbauthzAsCallee(pass, call)
	if !ok {
		return
	}

	if !callHasSingleContextArg(pass, call) {
		return
	}

	if isSuppressed(pass.Fset, suppressed, stack, call) {
		return
	}

	pass.Report(analysis.Diagnostic{
		Pos: call.Fun.Pos(),
		Message: "using '" + fn.Name() + "' is dangerous and should be " +
			"accompanied by a //" + suppressionDirective +
			" comment explaining why it's ok",
	})
}

// isSuppressed reports whether the call is covered by a
// //dbauthzcheck:ignore directive. A directive suppresses the call if
// it sits on:
//
//   - the call's own line (trailing comment), or
//   - any line between the enclosing statement's first line and the
//     call's line (inclusive), which covers the common pattern of a
//     leading comment above a multi-line statement whose argument
//     list contains the dbauthz.As* call.
func isSuppressed(fset *token.FileSet, suppressed map[int]bool, stack []ast.Node, call *ast.CallExpr) bool {
	callLine := fset.Position(call.Fun.Pos()).Line
	if suppressed[callLine] {
		return true
	}

	stmtLine := enclosingStatementLine(fset, stack, callLine)
	for line := stmtLine; line <= callLine; line++ {
		if suppressed[line] {
			return true
		}
	}
	return false
}

// enclosingStatementLine returns the line of the innermost enclosing
// ast.Stmt in the parent stack. If no statement ancestor is found
// (e.g. the call is in a top-level var/const initializer) the call's
// own line is returned.
func enclosingStatementLine(fset *token.FileSet, stack []ast.Node, fallback int) int {
	for i := len(stack) - 1; i >= 0; i-- {
		if stmt, ok := stack[i].(ast.Stmt); ok {
			return fset.Position(stmt.Pos()).Line
		}
	}
	return fallback
}

// dbauthzAsCallee returns the called function object when the call
// resolves to a function in the dbauthz package whose name starts with
// "As". Method calls and non-dbauthz helpers are ignored.
func dbauthzAsCallee(pass *analysis.Pass, call *ast.CallExpr) (*types.Func, bool) {
	selector, ok := unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}

	if pass.TypesInfo.Selections[selector] != nil {
		// Method expressions and method values are never package-level
		// functions, so they can't be dbauthz.As*.
		return nil, false
	}

	use, ok := pass.TypesInfo.Uses[selector.Sel].(*types.Func)
	if !ok {
		return nil, false
	}

	pkg := use.Pkg()
	if pkg == nil || pkg.Path() != dbauthzPkgPath {
		return nil, false
	}

	if !strings.HasPrefix(use.Name(), "As") {
		return nil, false
	}

	return use, true
}

// callHasSingleContextArg reports whether the call has exactly one
// argument and that argument implements context.Context. This mirrors
// the original ruleguard rule, whose pattern `dbauthz.$f($c)` matched
// only single-node argument lists.
//
// Multi-arg As helpers (notably dbauthz.As(ctx, actor) and
// dbauthz.AsSubAgentAPI(ctx, orgID, userID)) are arguably just as
// dangerous, but widening the match would turn a pure port into a
// behavior change, so we keep the analyzer faithful to the original
// rule.
func callHasSingleContextArg(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 1 {
		return false
	}

	ctxIface := contextInterface(pass)
	if ctxIface == nil {
		return false
	}

	tv, ok := pass.TypesInfo.Types[call.Args[0]]
	if !ok || tv.Type == nil {
		return false
	}
	return types.Implements(tv.Type, ctxIface)
}

// contextInterface returns the *types.Interface for context.Context in
// the context package used by the file under analysis. It returns nil
// if the package is not part of the build, which happens for Go files
// that don't transitively import context.
func contextInterface(pass *analysis.Pass) *types.Interface {
	for _, imp := range pass.Pkg.Imports() {
		if imp.Path() != "context" {
			continue
		}
		obj := imp.Scope().Lookup("Context")
		if obj == nil {
			return nil
		}
		named, ok := obj.Type().(*types.Named)
		if !ok {
			return nil
		}
		iface, ok := named.Underlying().(*types.Interface)
		if !ok {
			return nil
		}
		return iface
	}
	return nil
}

// isTestFile reports whether the file is a Go test file. The original
// ruleguard rule skipped _test.go by filename, which we mirror here.
func isTestFile(fset *token.FileSet, file *ast.File) bool {
	name := fset.Position(file.Pos()).Filename
	return strings.HasSuffix(name, "_test.go")
}

// suppressedLines collects the set of source lines that are covered
// by a //dbauthzcheck:ignore directive. A single directive covers:
//
//   - the line it sits on (for trailing comments), and
//   - every line from the comment group's start through one line past
//     its end, which is the line directly adjacent to the comment.
//
// isSuppressed extends this further by walking from the enclosing
// statement's first line down to the flagged call, so multi-line
// statements also benefit from a single leading suppression.
func suppressedLines(fset *token.FileSet, file *ast.File) map[int]bool {
	lines := make(map[int]bool)
	for _, group := range file.Comments {
		if !groupHasSuppression(group) {
			continue
		}

		start := fset.Position(group.Pos()).Line
		end := fset.Position(group.End()).Line
		for line := start; line <= end+1; line++ {
			lines[line] = true
		}
	}
	return lines
}

// groupHasSuppression reports whether any comment in the group begins
// with the suppression directive.
func groupHasSuppression(group *ast.CommentGroup) bool {
	for _, comment := range group.List {
		if isSuppressionComment(comment.Text) {
			return true
		}
	}
	return false
}

// isSuppressionComment reports whether the given comment text begins
// with the suppression directive. We require the directive at the
// start (after whitespace and the comment markers) so that quotes of
// the directive in longer text — e.g. analysistest `// want` patterns
// that reproduce the diagnostic message — don't accidentally suppress
// real diagnostics.
func isSuppressionComment(text string) bool {
	text = strings.TrimPrefix(text, "//")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, suppressionDirective)
}

// unparen strips parentheses. We only ever care about the underlying
// expression when deciding whether a call is dbauthz.As*.
func unparen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}
