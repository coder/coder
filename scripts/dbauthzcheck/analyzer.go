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
	ResultType: reflect.TypeFor[*struct{}](),
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if isTestFile(pass.Fset, file) {
			continue
		}

		suppressed := suppressedLines(pass.Fset, file)

		// PreorderStack hands us the ancestor stack (excluding the
		// current node) on every visit, so we can find the enclosing
		// statement without maintaining our own push/pop bookkeeping.
		// Suppression comments typically sit above the statement
		// containing the call, not above the call itself (which may be
		// a nested argument).
		ast.PreorderStack(file, nil, func(n ast.Node, stack []ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				checkCall(pass, call, stack, suppressed)
			}
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

	// Every single-arg dbauthz.As* helper takes a context.Context —
	// the package signatures enforce this — so narrowing to "the call
	// has exactly one argument" is sufficient after we know the
	// callee. No types.Implements check is needed, which also means
	// packages that transitively import context without a direct
	// import are still analyzed correctly.
	if len(call.Args) != 1 {
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
// any of these lines carry the directive:
//
//   - the call's own line (trailing comment),
//   - any line between the innermost enclosing ast.Stmt's first line
//     and the call's line (for leading suppressions above multi-line
//     statements whose dbauthz.As* argument is indented below), or
//   - any line in the enclosing FuncDecl's doc comment (when the call
//     has no enclosing statement ancestor, the innermost statement's
//     first line still matches a FuncDecl doc-comment line because the
//     doc group immediately precedes the function).
//
// Statement scope intentionally stops at the innermost ast.Stmt.
// Placing a directive above `switch` to cover a `return` inside a
// nested `case` does not work — the case clause's body is a separate
// statement. Tests in testdata pin both boundaries.
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

	// Fall back to the enclosing FuncDecl's doc comment, which isn't
	// a Stmt and so isn't reachable via the statement walk above. A
	// directive above the function declaration suppresses every call
	// inside the function body.
	if doc := enclosingFuncDoc(stack); doc != nil {
		docStart := fset.Position(doc.Pos()).Line
		docEnd := fset.Position(doc.End()).Line
		for line := docStart; line <= docEnd+1; line++ {
			if suppressed[line] {
				return true
			}
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

// enclosingFuncDoc returns the doc comment of the innermost enclosing
// FuncDecl, or nil if the call has no function ancestor or that
// function carries no doc comment.
func enclosingFuncDoc(stack []ast.Node) *ast.CommentGroup {
	for i := len(stack) - 1; i >= 0; i-- {
		if fn, ok := stack[i].(*ast.FuncDecl); ok {
			return fn.Doc
		}
	}
	return nil
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
// with the suppression directive followed by a word boundary. We
// require a boundary after the directive so typos like
// "//dbauthzcheck:ignoref" or "//dbauthzcheck:ignorefoo" don't silently
// suppress a real diagnostic. Valid terminators are end-of-text,
// whitespace, and comment separators like "//" or "--".
func isSuppressionComment(text string) bool {
	text = strings.TrimPrefix(text, "//")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSpace(text)

	rest, ok := strings.CutPrefix(text, suppressionDirective)
	if !ok {
		return false
	}
	if rest == "" {
		return true
	}
	switch rest[0] {
	case ' ', '\t':
		return true
	}
	return strings.HasPrefix(rest, "//") || strings.HasPrefix(rest, "--")
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
