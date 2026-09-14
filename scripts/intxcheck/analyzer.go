package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports outer store usage inside database.Store.InTx closures.
var Analyzer = &analysis.Analyzer{
	Name: "intxcheck",
	Doc:  "report unsafe outer-store usage inside database.Store.InTx closures",
	Run:  run,
	// ResultType must be set so run can return a typed nil instead
	// of nil, nil — which the nilnil linter forbids. No downstream
	// analyzer depends on this result.
	ResultType: reflect.TypeOf((*struct{})(nil)),
}

type txContext struct {
	outerStore outerStoreMatcher
	txName     string
}

type outerStoreMatcher struct {
	display     string
	fieldSuffix string
	ownerForms  []exprForm
	storeForms  []exprForm
}

type exprForm struct {
	text   string
	root   types.Object
	suffix string
}

func run(pass *analysis.Pass) (any, error) {
	decls := make(map[types.Object]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			obj := pass.TypesInfo.Defs[funcDecl.Name]
			if obj == nil {
				continue
			}
			decls[obj] = funcDecl
		}
	}

	for _, file := range pass.Files {
		suppressed := suppressedLines(pass.Fset, file)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			inTxSelector, ok := unparen(call.Fun).(*ast.SelectorExpr)
			if !ok || inTxSelector.Sel.Name != "InTx" {
				return true
			}
			if len(call.Args) == 0 {
				return true
			}

			funcLit, ok := unparen(call.Args[0]).(*ast.FuncLit)
			if !ok {
				return true
			}

			outerStore, ok := newOuterStoreMatcher(pass, inTxSelector.X)
			if !ok {
				return true
			}

			ctx := txContext{
				outerStore: outerStore,
				txName:     firstParamName(funcLit.Type),
			}

			inspectInTxBody(pass, funcLit.Body, ctx, decls, suppressed)
			return true
		})
	}

	return (*struct{})(nil), nil
}

func inspectInTxBody(pass *analysis.Pass, body *ast.BlockStmt, ctx txContext, decls map[types.Object]*ast.FuncDecl, suppressed map[int]bool) {
	ctx = ctx.withAliases(pass, body)

	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.GoStmt:
			if funcLit, ok := funcLitCall(n.Call); ok {
				reportCallMisuse(pass, n.Call, ctx, suppressed)
				inspectInTxBody(pass, funcLit.Body, ctx, decls, suppressed)
				return false
			}
			return true
		case *ast.DeferStmt:
			if funcLit, ok := funcLitCall(n.Call); ok {
				reportCallMisuse(pass, n.Call, ctx, suppressed)
				inspectInTxBody(pass, funcLit.Body, ctx, decls, suppressed)
				return false
			}
			return true
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		reported := reportCallMisuse(pass, call, ctx, suppressed)
		if funcLit, ok := funcLitCall(call); ok {
			inspectInTxBody(pass, funcLit.Body, ctx, decls, suppressed)
			return true
		}
		if reported {
			return true
		}

		callee, calleeOuterStore, ok := resolveSamePackageCallee(pass, call, ctx, decls)
		if !ok || callee == nil || callee.Body == nil {
			return true
		}
		if !bodyUsesOuterStore(pass, callee.Body, calleeOuterStore) {
			return true
		}

		reportIfNotSuppressed(pass, suppressed, call.Pos(), fmt.Sprintf(
			"call to '%s' inside InTx uses outer store '%s'; pass '%s' through the helper or hoist the call",
			exprString(call.Fun),
			ctx.outerStore.display,
			ctx.txName,
		))
		return true
	})
}

func reportCallMisuse(pass *analysis.Pass, call *ast.CallExpr, ctx txContext, suppressed map[int]bool) bool {
	kind, pos := classifyCall(pass, call, ctx.outerStore)
	switch kind {
	case misuseDirect:
		reportIfNotSuppressed(pass, suppressed, pos, fmt.Sprintf(
			"outer store '%s' used inside InTx; use transaction store '%s' instead",
			ctx.outerStore.display,
			ctx.txName,
		))
		return true
	case misusePassThrough:
		reportIfNotSuppressed(pass, suppressed, pos, fmt.Sprintf(
			"outer store '%s' passed as argument inside InTx; use transaction store '%s' instead",
			ctx.outerStore.display,
			ctx.txName,
		))
		return true
	default:
		return false
	}
}

func funcLitCall(call *ast.CallExpr) (*ast.FuncLit, bool) {
	funcLit, ok := unparen(call.Fun).(*ast.FuncLit)
	if !ok {
		return nil, false
	}
	return funcLit, true
}

func reportIfNotSuppressed(pass *analysis.Pass, suppressed map[int]bool, pos token.Pos, message string) {
	if suppressedLine(pass.Fset, suppressed, pos) {
		return
	}

	pass.Report(analysis.Diagnostic{
		Pos:     pos,
		Message: message,
	})
}

type misuseKind int

const (
	misuseNone misuseKind = iota
	misuseDirect
	misusePassThrough
)

func classifyCall(pass *analysis.Pass, call *ast.CallExpr, outerStore outerStoreMatcher) (misuseKind, token.Pos) {
	if receiver := callReceiver(call); receiver != nil && outerStore.matches(pass, receiver) {
		return misuseDirect, receiver.Pos()
	}

	for _, arg := range call.Args {
		if outerStore.matches(pass, arg) {
			return misusePassThrough, arg.Pos()
		}
	}

	return misuseNone, token.NoPos
}

func bodyUsesOuterStore(pass *analysis.Pass, body *ast.BlockStmt, outerStore outerStoreMatcher) bool {
	outerStore = outerStore.withAliases(pass, body)

	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}

		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.GoStmt:
			if kind, _ := classifyCall(pass, n.Call, outerStore); kind != misuseNone {
				found = true
				return false
			}
			if funcLit, ok := funcLitCall(n.Call); ok {
				found = bodyUsesOuterStore(pass, funcLit.Body, outerStore)
				return false
			}
			return true
		case *ast.DeferStmt:
			if kind, _ := classifyCall(pass, n.Call, outerStore); kind != misuseNone {
				found = true
				return false
			}
			if funcLit, ok := funcLitCall(n.Call); ok {
				found = bodyUsesOuterStore(pass, funcLit.Body, outerStore)
				return false
			}
			return true
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		kind, _ := classifyCall(pass, call, outerStore)
		if kind != misuseNone {
			found = true
			return false
		}
		if funcLit, ok := funcLitCall(call); ok {
			found = bodyUsesOuterStore(pass, funcLit.Body, outerStore)
			if found {
				return false
			}
		}
		return true
	})
	return found
}

func resolveSamePackageCallee(pass *analysis.Pass, call *ast.CallExpr, ctx txContext, decls map[types.Object]*ast.FuncDecl) (*ast.FuncDecl, outerStoreMatcher, bool) {
	switch fun := unparen(call.Fun).(type) {
	case *ast.Ident:
		// Package-level helpers have their own parameter scope. The
		// pass-through check already catches explicit outer-store
		// arguments, so skip indirect analysis here.
		return nil, outerStoreMatcher{}, false
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[fun]
		if selection == nil {
			return nil, outerStoreMatcher{}, false
		}
		decl, ok := decls[selection.Obj()]
		if !ok || decl == nil || decl.Recv == nil {
			return nil, outerStoreMatcher{}, false
		}
		if !ctx.outerStore.matchesOwner(pass, fun.X) {
			return nil, outerStoreMatcher{}, false
		}
		calleeOuterStore, ok := ctx.outerStore.withReceiver(pass, decl)
		if !ok {
			return nil, outerStoreMatcher{}, false
		}
		return decl, calleeOuterStore, true
	default:
		return nil, outerStoreMatcher{}, false
	}
}

func (ctx txContext) withAliases(pass *analysis.Pass, body *ast.BlockStmt) txContext {
	ctx.outerStore = ctx.outerStore.withAliases(pass, body)
	return ctx
}

func newOuterStoreMatcher(pass *analysis.Pass, expr ast.Expr) (outerStoreMatcher, bool) {
	display := exprString(expr)
	if display == "" {
		return outerStoreMatcher{}, false
	}

	matcher := outerStoreMatcher{display: display}
	matcher.addStoreForm(exprFormFor(pass, expr))

	selector, ok := unparen(expr).(*ast.SelectorExpr)
	if !ok {
		return matcher, true
	}

	matcher.fieldSuffix = "." + selector.Sel.Name
	matcher.addOwnerForm(exprFormFor(pass, selector.X))
	return matcher, true
}

func (m outerStoreMatcher) withAliases(pass *analysis.Pass, body *ast.BlockStmt) outerStoreMatcher {
	base := m
	derived := m

	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			if n.Tok != token.DEFINE {
				return true
			}
			for i, lhs := range n.Lhs {
				if i >= len(n.Rhs) {
					break
				}
				derived.collectAlias(pass, base, lhs, n.Rhs[i])
			}
		case *ast.DeclStmt:
			genDecl, ok := n.Decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.VAR {
				return true
			}
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range valueSpec.Names {
					if i >= len(valueSpec.Values) {
						break
					}
					derived.collectAlias(pass, base, name, valueSpec.Values[i])
				}
			}
		}
		return true
	})

	return derived
}

func (m *outerStoreMatcher) collectAlias(pass *analysis.Pass, base outerStoreMatcher, lhs ast.Expr, rhs ast.Expr) {
	lhsForm, ok := declaredIdentForm(pass, lhs)
	if !ok {
		return
	}

	switch {
	case base.matches(pass, rhs):
		m.addStoreForm(lhsForm)
	case base.matchesOwner(pass, rhs):
		m.addOwnerForm(lhsForm)
	}
}

func (m outerStoreMatcher) withReceiver(pass *analysis.Pass, decl *ast.FuncDecl) (outerStoreMatcher, bool) {
	recvForm, ok := receiverForm(pass, decl)
	if !ok {
		return outerStoreMatcher{}, false
	}

	rebound := outerStoreMatcher{
		display:     m.display,
		fieldSuffix: m.fieldSuffix,
		ownerForms:  []exprForm{recvForm},
	}
	if m.fieldSuffix == "" {
		rebound.storeForms = []exprForm{recvForm}
	}
	return rebound, true
}

func (m outerStoreMatcher) matches(pass *analysis.Pass, expr ast.Expr) bool {
	form := exprFormFor(pass, expr)
	if form.text == "" {
		return false
	}

	for _, storeForm := range m.storeForms {
		if sameExprForm(form, storeForm) {
			return true
		}
	}

	if m.fieldSuffix == "" {
		return false
	}

	for _, ownerForm := range m.ownerForms {
		if sameExprFormWithSuffix(form, ownerForm, m.fieldSuffix) {
			return true
		}
	}

	return false
}

func (m outerStoreMatcher) matchesOwner(pass *analysis.Pass, expr ast.Expr) bool {
	if len(m.ownerForms) == 0 {
		return false
	}

	form := exprFormFor(pass, expr)
	if form.text == "" {
		return false
	}

	for _, ownerForm := range m.ownerForms {
		if sameExprForm(form, ownerForm) {
			return true
		}
	}
	return false
}

func (m *outerStoreMatcher) addOwnerForm(form exprForm) {
	if form.text == "" || containsExprForm(m.ownerForms, form) {
		return
	}
	m.ownerForms = append(m.ownerForms, form)
}

func (m *outerStoreMatcher) addStoreForm(form exprForm) {
	if form.text == "" || containsExprForm(m.storeForms, form) {
		return
	}
	m.storeForms = append(m.storeForms, form)
}

func containsExprForm(forms []exprForm, want exprForm) bool {
	for _, form := range forms {
		if sameExprForm(form, want) {
			return true
		}
	}
	return false
}

func sameExprForm(got, want exprForm) bool {
	if got.root != nil && want.root != nil {
		return got.root == want.root && got.suffix == want.suffix
	}
	return got.text == want.text
}

func sameExprFormWithSuffix(got, base exprForm, suffix string) bool {
	if got.root != nil && base.root != nil {
		return got.root == base.root && got.suffix == base.suffix+suffix
	}
	return got.text == base.text+suffix
}

func exprFormFor(pass *analysis.Pass, expr ast.Expr) exprForm {
	text := exprString(expr)
	if text == "" {
		return exprForm{}
	}

	ident, suffix, ok := rootIdentAndSuffix(expr)
	if !ok {
		return exprForm{text: text}
	}

	return exprForm{
		text:   text,
		root:   identObject(pass, ident),
		suffix: suffix,
	}
}

func receiverForm(pass *analysis.Pass, decl *ast.FuncDecl) (exprForm, bool) {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return exprForm{}, false
	}
	if len(decl.Recv.List[0].Names) == 0 {
		return exprForm{}, false
	}

	ident := decl.Recv.List[0].Names[0]
	obj := pass.TypesInfo.Defs[ident]
	if obj == nil {
		return exprForm{}, false
	}

	return exprForm{text: ident.Name, root: obj}, true
}

func declaredIdentForm(pass *analysis.Pass, expr ast.Expr) (exprForm, bool) {
	ident, ok := unparen(expr).(*ast.Ident)
	if !ok || ident.Name == "_" {
		return exprForm{}, false
	}

	obj := pass.TypesInfo.Defs[ident]
	if obj == nil {
		return exprForm{}, false
	}

	return exprForm{text: ident.Name, root: obj}, true
}

func identObject(pass *analysis.Pass, ident *ast.Ident) types.Object {
	if ident == nil {
		return nil
	}
	if obj := pass.TypesInfo.Uses[ident]; obj != nil {
		return obj
	}
	return pass.TypesInfo.Defs[ident]
}

func rootIdentAndSuffix(expr ast.Expr) (*ast.Ident, string, bool) {
	switch expr := unparen(expr).(type) {
	case *ast.Ident:
		return expr, "", true
	case *ast.SelectorExpr:
		ident, suffix, ok := rootIdentAndSuffix(expr.X)
		if !ok {
			return nil, "", false
		}
		return ident, suffix + "." + expr.Sel.Name, true
	default:
		return nil, "", false
	}
}

func callReceiver(call *ast.CallExpr) ast.Expr {
	selector, ok := unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	return selector.X
}

func suppressedLines(fset *token.FileSet, file *ast.File) map[int]bool {
	lines := make(map[int]bool)
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.Contains(comment.Text, "intxcheck:ignore") {
				lines[fset.Position(comment.Pos()).Line] = true
			}
		}
	}
	return lines
}

func suppressedLine(fset *token.FileSet, suppressed map[int]bool, pos token.Pos) bool {
	return suppressed[fset.Position(pos).Line]
}

func firstParamName(funcType *ast.FuncType) string {
	if funcType == nil || funcType.Params == nil || len(funcType.Params.List) == 0 {
		return "tx"
	}
	first := funcType.Params.List[0]
	if len(first.Names) == 0 {
		return "tx"
	}
	return first.Names[0].Name
}

func exprString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	return types.ExprString(unparen(expr))
}

func unparen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}
