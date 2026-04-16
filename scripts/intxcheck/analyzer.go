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
	Name:       "intxcheck",
	Doc:        "report unsafe outer-store usage inside database.Store.InTx closures",
	Run:        run,
	ResultType: reflect.TypeOf(result{}),
}

type result struct{}

type txContext struct {
	outerStore string
	txName     string
	owner      string
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

			outerStore := exprString(inTxSelector.X)
			if outerStore == "" {
				return true
			}

			ctx := txContext{
				outerStore: outerStore,
				txName:     firstParamName(funcLit.Type),
			}
			if owner, ok := selectorOwnerString(inTxSelector.X); ok {
				ctx.owner = owner
			}

			inspectInTxBody(pass, funcLit.Body, ctx, decls)
			return true
		})
	}

	return result{}, nil
}

func inspectInTxBody(pass *analysis.Pass, body *ast.BlockStmt, ctx txContext, decls map[types.Object]*ast.FuncDecl) {
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		kind, pos := classifyCall(call, ctx.outerStore)
		switch kind {
		case misuseDirect:
			pass.Report(analysis.Diagnostic{
				Pos:     pos,
				Message: fmt.Sprintf("outer store '%s' used inside InTx; use transaction store '%s' instead", ctx.outerStore, ctx.txName),
			})
			return true
		case misusePassThrough:
			pass.Report(analysis.Diagnostic{
				Pos:     pos,
				Message: fmt.Sprintf("outer store '%s' passed as argument inside InTx; use transaction store '%s' instead", ctx.outerStore, ctx.txName),
			})
			return true
		}

		callee, calleeOuterStore, ok := resolveSamePackageCallee(pass, call, ctx, decls)
		if !ok || callee == nil || callee.Body == nil {
			return true
		}
		if !bodyUsesOuterStore(callee.Body, calleeOuterStore) {
			return true
		}

		pass.Report(analysis.Diagnostic{
			Pos:     call.Pos(),
			Message: fmt.Sprintf("call to '%s' inside InTx uses outer store '%s'; pass '%s' through the helper or hoist the call", exprString(call.Fun), ctx.outerStore, ctx.txName),
		})
		return true
	})
}

type misuseKind int

const (
	misuseNone misuseKind = iota
	misuseDirect
	misusePassThrough
)

func classifyCall(call *ast.CallExpr, outerStore string) (misuseKind, token.Pos) {
	if receiver := callReceiver(call); receiver != nil && exprString(receiver) == outerStore {
		return misuseDirect, receiver.Pos()
	}

	for _, arg := range call.Args {
		if exprString(arg) == outerStore {
			return misusePassThrough, arg.Pos()
		}
	}

	return misuseNone, token.NoPos
}

func bodyUsesOuterStore(body *ast.BlockStmt, outerStore string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		kind, _ := classifyCall(call, outerStore)
		if kind != misuseNone {
			found = true
			return false
		}
		return true
	})
	return found
}

func resolveSamePackageCallee(pass *analysis.Pass, call *ast.CallExpr, ctx txContext, decls map[types.Object]*ast.FuncDecl) (*ast.FuncDecl, string, bool) {
	switch fun := unparen(call.Fun).(type) {
	case *ast.Ident:
		obj := pass.TypesInfo.Uses[fun]
		decl, ok := decls[obj]
		if !ok || decl == nil || decl.Recv != nil {
			return nil, "", false
		}
		return decl, ctx.outerStore, true
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[fun]
		if selection == nil {
			return nil, "", false
		}
		decl, ok := decls[selection.Obj()]
		if !ok || decl == nil || decl.Recv == nil {
			return nil, "", false
		}
		if ctx.owner == "" || exprString(fun.X) != ctx.owner {
			return nil, "", false
		}
		recvName := receiverName(decl)
		if recvName == "" {
			return nil, "", false
		}
		return decl, rewriteOuterStore(ctx.outerStore, ctx.owner, recvName), true
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

func selectorOwnerString(expr ast.Expr) (string, bool) {
	selector, ok := unparen(expr).(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	return exprString(selector.X), true
}

func rewriteOuterStore(outerStore, owner, recvName string) string {
	suffix := strings.TrimPrefix(outerStore, owner)
	if suffix == outerStore {
		return outerStore
	}
	return recvName + suffix
}

func receiverName(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return ""
	}
	recv := decl.Recv.List[0]
	if len(recv.Names) == 0 {
		return ""
	}
	return recv.Names[0].Name
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
