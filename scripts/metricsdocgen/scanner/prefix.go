package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// modulePath is the Go module path of this repository. Import paths are
// rewritten into repository-relative directories through it, so a metric
// constructor's package can be matched to the files the scanner walks.
const modulePath = "github.com/coder/coder/v2/"

// prefixIndex maps a package directory, relative to the repository root, to
// the metric name prefixes applied to the registerer handed to that package's
// metric constructors.
//
// Some packages declare their metrics without a namespace and rely on the
// caller to wrap the registerer, so the declaration alone does not spell the
// published metric name. Scanning those files without the prefix would emit
// names that no deployment ever exposes, which is worse than omitting them.
type prefixIndex map[string][]string

// prefixFuncs are the registerer wrappers that prepend to every metric name
// registered through them. Each maps to the index of the argument holding the
// canonical prefix.
//
// NewMetricAliasRegisterer also registers a second, legacy prefix so existing
// dashboards keep working, and takes it as a later argument. Only the
// canonical prefix is indexed: the aliases are deprecated and the reference
// should not teach them.
var prefixFuncs = map[string]int{
	"WrapRegistererWithPrefix": 0,
	"NewMetricAliasRegisterer": 1,
}

// buildPrefixIndex scans roots for registerer wrapping and returns the
// prefixes that reach each metric constructor's package.
func buildPrefixIndex(roots []string) (prefixIndex, error) {
	consts, files, err := collectPrefixInputs(roots)
	if err != nil {
		return nil, err
	}

	index := prefixIndex{}
	for _, pf := range files {
		for _, decl := range pf.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			indexFunc(index, pf, fn, consts)
		}
	}

	propagateThroughForwarders(index, files)

	for dir := range index {
		slices.Sort(index[dir])
		index[dir] = slices.Compact(index[dir])
	}
	return index, nil
}

// propagateThroughForwarders follows constructors that hand their registerer
// to another package, so a prefix applied at the call site reaches the package
// that actually declares the metrics. aibridge.NewMetrics, for example,
// forwards straight to aibridge/metrics.NewMetrics, where the metrics live.
//
// It iterates to a fixed point, bounded so a cycle cannot hang generation.
func propagateThroughForwarders(index prefixIndex, files []parsedFile) {
	const maxRounds = 8

	for round := range maxRounds {
		changed := false
		for _, pf := range files {
			dir := filepath.Dir(pf.path)
			prefixes, ok := index[dir]
			if !ok {
				continue
			}
			for _, target := range forwardedPackages(pf) {
				if target == dir {
					continue
				}
				for _, prefix := range prefixes {
					if !slices.Contains(index[target], prefix) {
						index[target] = append(index[target], prefix)
						changed = true
					}
				}
			}
		}
		if !changed {
			return
		}
		if round == maxRounds-1 {
			warnf("metric prefix propagation did not converge after %d rounds", maxRounds)
		}
	}
}

// forwardedPackages returns the package directories that this file's
// constructors pass their prometheus.Registerer parameter to.
func forwardedPackages(pf parsedFile) []string {
	var targets []string

	for _, decl := range pf.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		registerers := map[string]bool{}
		for _, field := range fn.Type.Params.List {
			if !isPrometheusRegisterer(field.Type) {
				continue
			}
			for _, name := range field.Names {
				registerers[name.Name] = true
			}
		}
		if len(registerers) == 0 {
			continue
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			target, ok := pf.imports[pkgIdent.Name]
			if !ok {
				return true
			}
			for _, arg := range call.Args {
				if argIdent, ok := arg.(*ast.Ident); ok && registerers[argIdent.Name] {
					targets = append(targets, target)
				}
			}
			return true
		})
	}

	return targets
}

// isPrometheusRegisterer reports whether expr names prometheus.Registerer.
// The prefix scanner intentionally stays syntax-only, so it recognizes the
// package name used throughout this repository rather than loading type data.
func isPrometheusRegisterer(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Registerer" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "prometheus"
}

// parsedFile is a parsed Go file plus the import information needed to resolve
// a selector back to a package directory.
type parsedFile struct {
	path    string
	file    *ast.File
	imports map[string]string // local name -> package directory
}

// collectPrefixInputs parses every non-test Go file under roots, returning the
// package-level string constants (keyed "<dir>.<Name>") and the parsed files.
func collectPrefixInputs(roots []string) (map[string]string, []parsedFile, error) {
	consts := map[string]string{}
	var files []parsedFile

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return xerrors.Errorf("parsing %s: %w", path, err)
			}

			dir := filepath.Dir(path)
			for name, value := range stringConsts(file, func(string) bool { return true }, stringLiteral) {
				consts[dir+"."+name] = value
			}
			files = append(files, parsedFile{path: path, file: file, imports: fileImports(file)})
			return nil
		})
		if err != nil {
			return nil, nil, xerrors.Errorf("collecting prefix inputs from %s: %w", root, err)
		}
	}

	return consts, files, nil
}

// indexFunc records, for one function body, which prefixes reach which metric
// constructor packages. It handles the shape the codebase uses:
//
//	reg := prometheus.WrapRegistererWithPrefix("coder_ai_gateway_", registry)
//	metrics := aibridgedserver.NewMetrics(reg)
func indexFunc(index prefixIndex, pf parsedFile, fn *ast.FuncDecl, consts map[string]string) {
	wrapped := map[string]string{} // variable name -> prefix

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		prefix, ok := wrapPrefix(assign.Rhs[0], pf, consts)
		if !ok {
			return true
		}
		wrapped[ident.Name] = prefix
		return true
	})

	if len(wrapped) == 0 {
		return
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		// A collector built elsewhere and registered against the wrapped
		// registerer takes the prefix too:
		//
		//	reg.MustRegister(keypool.NewStateCollector(pool.KeyPools))
		if prefix, ok := wrapped[pkgIdent.Name]; ok && registerFuncs[sel.Sel.Name] {
			for _, arg := range call.Args {
				if dir, ok := constructorPackage(arg, pf); ok {
					index[dir] = append(index[dir], prefix)
				}
			}
			return true
		}

		// A package function handed the wrapped registerer registers its metrics
		// through it:
		//
		//	metrics := aibridgedserver.NewMetrics(costControlReg)
		dir, ok := pf.imports[pkgIdent.Name]
		if !ok {
			return true
		}

		for _, arg := range call.Args {
			argIdent, ok := arg.(*ast.Ident)
			if !ok {
				continue
			}
			if prefix, ok := wrapped[argIdent.Name]; ok {
				index[dir] = append(index[dir], prefix)
			}
		}
		return true
	})
}

// registerFuncs are the prometheus.Registerer methods that take a collector.
var registerFuncs = map[string]bool{
	"Register":     true,
	"MustRegister": true,
}

// constructorPackage returns the package directory of the collector created
// by expr. Only the top-level call is relevant because nested calls provide
// constructor arguments, not collectors registered through the registerer.
func constructorPackage(expr ast.Expr, pf parsedFile) (string, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	dir, ok := pf.imports[pkgIdent.Name]
	return dir, ok
}

// wrapPrefix reports the prefix a registerer-wrapping call applies, if the
// expression is such a call and its prefix argument resolves to a constant
// string.
func wrapPrefix(expr ast.Expr, pf parsedFile, consts map[string]string) (string, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	argIdx, ok := prefixFuncs[sel.Sel.Name]
	if !ok || len(call.Args) <= argIdx {
		return "", false
	}
	return constString(call.Args[argIdx], pf, consts)
}

// constString resolves a string literal or a reference to a package-level
// string constant, whether that constant is local or imported.
func constString(expr ast.Expr, pf parsedFile, consts map[string]string) (string, bool) {
	if literal, ok := expr.(*ast.BasicLit); ok {
		return stringLiteral(literal)
	}
	return resolveStringReference(
		expr,
		func(name string) (string, bool) {
			value, ok := consts[filepath.Dir(pf.path)+"."+name]
			return value, ok
		},
		func(pkg, name string) (string, bool) {
			dir, ok := pf.imports[pkg]
			if !ok {
				return "", false
			}
			value, ok := consts[dir+"."+name]
			return value, ok
		},
	)
}

// stringLiteral resolves a string literal without changing its escapes.
func stringLiteral(lit *ast.BasicLit) (string, bool) {
	if lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// stringConsts returns the file's package-level string constants. include and
// resolve select the constants and literal representation appropriate for each
// caller's resolution scope.
func stringConsts(
	file *ast.File,
	include func(name string) bool,
	resolve func(*ast.BasicLit) (string, bool),
) map[string]string {
	consts := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valueSpec.Names {
				if !include(name.Name) || i >= len(valueSpec.Values) {
					continue
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok {
					continue
				}
				if value, ok := resolve(lit); ok {
					consts[name.Name] = value
				}
			}
		}
	}
	return consts
}

// fileImports maps each in-repository import's local name to its directory,
// relative to the repository root. Imports from other modules are skipped
// because the scanner cannot read their sources.
func fileImports(file *ast.File) map[string]string {
	imports := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !strings.HasPrefix(path, modulePath) {
			continue
		}
		dir := strings.TrimPrefix(path, modulePath)

		name := filepath.Base(dir)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = dir
	}
	return imports
}
