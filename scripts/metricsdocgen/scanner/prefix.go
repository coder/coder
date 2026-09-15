package main

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// This file resolves the registry prefix that applies to a metric
// constructor call. Metrics registered through a prefixed registerer
// (for example [prometheus.WrapRegistererWithPrefix]) declare bare names
// in source, so the exported name is only known from the registration
// site. The resolver answers, for a given constructor call, which
// prefixes the exported names carry.
//
// The analysis is purely syntactic: it uses go/ast only, no go/types and
// no SSA. It is deliberately bounded. Everything it cannot prove is
// reported as unresolved rather than guessed, and unprefixed
// registerers never propagate across function boundaries, so a throwaway
// registry in a test helper cannot make a prefixed metric look bare.
//
// Supported flows:
//   - promauto.With(reg).NewX(...) and factory := promauto.With(reg).
//   - prometheus.WrapRegistererWithPrefix(prefix, base), nested.
//   - prometheus.WrapRegistererWith(labels, base) (prefix passes through).
//   - NewMetricAliasRegisterer, using only its canonical prefix.
//   - Registerer values passed as arguments into other functions, to a
//     fixpoint, so constructor forwarding chains resolve.
//   - Collectors that build their own prometheus.NewDesc, when the
//     caller registers the constructor result on a prefixed registerer
//     via MustRegister or Register.
//   - Roots: prometheus.NewRegistry() and struct fields declared as
//     *prometheus.Registry, reached through a receiver whose type is
//     known from a parameter, a composite literal or a single call to a
//     function or func-typed parameter. Promoted fields resolve through
//     embedded structs.
//
// Unsupported (reported unresolved or unmanaged, never guessed):
//   - Receivers whose type cannot be inferred from the bounded sources
//     above, including registerers read back from maps, slices or
//     multi-step field chains.
//   - Registerers produced by interface method calls, or by helpers other
//     than the two prometheus wrappers and NewMetricAliasRegisterer.
//   - Package-level variable initialisation of registerers.
//   - Prefixes built from non-constant expressions.
//   - Flow-sensitivity: a local name assigned two different registerer
//     values in one function is treated as unresolved.

const (
	prometheusPkgPath = "github.com/prometheus/client_golang/prometheus"
	promautoPkgPath   = prometheusPkgPath + "/promauto"

	// defaultModulePath is used when go.mod cannot be read. It only
	// affects the import paths used as index keys, so a wrong value
	// degrades to "nothing resolves" rather than to a wrong prefix.
	defaultModulePath = "github.com/coder/coder/v2"

	// maxPropagationRounds bounds the interprocedural fixpoint. Prefix
	// sets only grow, and real forwarding chains are two or three calls
	// deep, so this is a safety net and not a tuning knob.
	maxPropagationRounds = 10

	// maxLocalRounds bounds the per-function local evaluation. Two passes
	// resolve chains such as factory := promauto.With(reg) that appear
	// after the assignment they depend on.
	maxLocalRounds = 2

	// maxEmbedDepth bounds promoted field lookup through embedded structs.
	maxEmbedDepth = 4
)

// regValue is the result of evaluating an expression that may hold a
// Prometheus registerer or a promauto factory.
type regValue struct {
	// isRegisterer reports that the expression was recognized as a
	// registerer or a factory derived from one.
	isRegisterer bool
	// resolved reports that prefixes is exact. An unresolved registerer
	// means "prefixed by something we could not compute".
	resolved bool
	// prefixed reports that the value passed through a prefixing wrapper.
	// Combined with resolved == false it means "this metric is renamed at
	// registration but we cannot say to what", which must be skipped
	// rather than emitted under its bare name.
	prefixed bool
	// prefixes holds the canonical exported prefixes. An empty string
	// element means "no prefix" (a bare root registry).
	prefixes []string
}

func (v regValue) hasPrefix() bool {
	return slices.ContainsFunc(v.prefixes, func(p string) bool { return p != "" })
}

// paramInfo describes one function parameter name.
type paramInfo struct {
	name string
	// registerer is true when the declared type mentions Registerer,
	// Registry or Gatherer. It is a syntactic hint used to seed the
	// candidate set, not a type check.
	registerer bool
	// variadic is true for the final ...T parameter.
	variadic bool
}

// assignFact is a local assignment whose right-hand side may be a
// registerer.
type assignFact struct {
	name string
	expr ast.Expr
}

// callFact is a call to an indexed function with at least one argument
// that may be a registerer.
type callFact struct {
	callee funcKey
	args   []ast.Expr
}

// registerFact is a MustRegister or Register call on a possible
// registerer receiver.
type registerFact struct {
	recv ast.Expr
	args []ast.Expr
}

// typeAssign is a local assignment whose right-hand side may reveal the
// static type of the assigned name: a composite literal, or result index
// idx of a single call.
type typeAssign struct {
	name string
	expr ast.Expr
	idx  int
}

// funcKey identifies a package-level function as "importpath.Name".
// Methods are not addressable this way and are indexed with an empty key:
// their bodies still get local resolution, but nothing propagates into
// them.
type funcKey string

// funcInfo is the compact summary of one function body. Facts reference
// the original AST nodes so the fixpoint can re-evaluate them without
// walking the tree again.
type funcInfo struct {
	key    funcKey
	file   *sourceFile
	params []paramInfo

	assigns   []assignFact
	calls     []callFact
	registers []registerFact

	// typeAssigns are resolved once, after indexing, into varTypes.
	typeAssigns []typeAssign
	// varTypes maps a local name to the key of its static named type.
	varTypes map[string]string
	// funcTypes holds func-typed parameters, so that calling one yields the
	// declared result type.
	funcTypes map[string]*ast.FuncType

	// env maps local names to their registerer value. It is rebuilt on
	// every propagation round and left populated for resolve.
	env map[string]regValue
}

// prefixResolver answers which registry prefixes apply to a metric
// constructor call.
type prefixResolver struct {
	modulePath string

	pkgPaths  map[*sourceFile]string
	imports   map[*sourceFile]map[string]string // local package name to import path
	pkgConsts map[string]map[string]string      // import path to string constant values

	// registryFields holds "importpath.Type.Field" keys for fields declared
	// as *prometheus.Registry. The concrete registry type has no prefix, so
	// reading such a field is a sound root. Registerer-typed fields are
	// deliberately excluded: their prefix is unknown.
	registryFields map[string]bool
	// registryFieldNames is the unqualified form of the same fields. It is
	// only a cheap pre-filter and never decides a prefix.
	registryFieldNames map[string]bool
	// embeds maps a type key to the type keys it embeds, for promoted
	// field lookup.
	embeds map[string][]string
	// funcResultTypes maps a package-level function to the type keys of its
	// results.
	funcResultTypes map[funcKey][]string

	funcInfos []*funcInfo
	// callFunc maps every call expression in an indexed body to that body.
	callFunc map[*ast.CallExpr]*funcInfo

	// paramPrefixes holds the non-empty prefixes observed flowing into a
	// function parameter, keyed by argument position.
	paramPrefixes map[funcKey]map[int][]string
	// paramUnknownPrefix marks parameters that receive a registerer known
	// to be prefixed by an expression we could not evaluate. Such
	// parameters poison resolution for their function so that no bare name
	// is emitted for a metric that is renamed at registration.
	paramUnknownPrefix map[funcKey]map[int]bool
	// collectorPrefixes holds the prefixes under which the result of a
	// collector constructor is registered by its callers.
	collectorPrefixes map[funcKey][]string
	collectorUnknown  map[funcKey]bool
}

// wrapperSummary describes a helper that returns a prefixed registerer
// built from its own parameters.
type wrapperSummary struct {
	ok        bool
	baseIdx   int
	prefixIdx int
}

// newPrefixResolver indexes files and runs the interprocedural fixpoint.
// The files should include registration sites (for example the cli
// wiring) even when no metric is extracted from them.
func newPrefixResolver(files []*sourceFile) *prefixResolver {
	r := &prefixResolver{
		modulePath:         readModulePath(),
		pkgPaths:           make(map[*sourceFile]string, len(files)),
		imports:            make(map[*sourceFile]map[string]string, len(files)),
		pkgConsts:          make(map[string]map[string]string),
		registryFields:     make(map[string]bool),
		registryFieldNames: make(map[string]bool),
		embeds:             make(map[string][]string),
		funcResultTypes:    make(map[funcKey][]string),
		callFunc:           make(map[*ast.CallExpr]*funcInfo),
		paramPrefixes:      make(map[funcKey]map[int][]string),
		paramUnknownPrefix: make(map[funcKey]map[int]bool),
		collectorPrefixes:  make(map[funcKey][]string),
		collectorUnknown:   make(map[funcKey]bool),
	}
	// Types and signatures are indexed before bodies so that field and
	// result lookups do not depend on directory scan order.
	for _, f := range files {
		r.indexFileHeader(f)
	}
	for _, f := range files {
		r.indexTypes(f)
		r.indexSignatures(f)
	}
	for _, f := range files {
		r.indexBodies(f)
	}
	r.resolveVarTypes()
	r.propagate()
	for _, fi := range r.funcInfos {
		fi.env = r.buildEnv(fi)
	}
	return r
}

// resolve reports the prefixes that apply to a metric constructor call.
//
// The boolean reports whether the metric is registry managed, meaning its
// exported name depends on the registerer it is registered against:
//
//   - (prefixes, true): managed and resolved. The caller must emit one
//     metric per prefix, prepending the prefix to the extracted name.
//   - (nil, true): managed but unresolvable. The caller must skip the
//     metric and warn. Emitting the bare name
//     would publish a name that does not exist at runtime.
//   - (nil, false): not managed. The extracted name is already complete,
//     so the caller emits it unchanged.
func (r *prefixResolver) resolve(file *sourceFile, call *ast.CallExpr) ([]string, bool) {
	fi := r.callFunc[call]
	if fi == nil {
		// The call is outside any indexed function body, for example a
		// package-level variable initialiser. Such metrics cannot be
		// registered through a prefixed registerer at declaration time.
		return nil, false
	}

	// A prometheus.NewDesc descriptor is not registered by itself; the
	// collector that owns it is registered by the caller.
	if r.isPkgCall(file, call, prometheusPkgPath, "NewDesc") {
		if r.collectorUnknown[fi.key] {
			return nil, true
		}
		if prefixes := r.collectorPrefixes[fi.key]; len(prefixes) > 0 {
			return prefixes, true
		}
		return nil, false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	if metricType, _ := parseMetricFuncName(sel.Sel.Name); metricType == "" {
		return nil, false
	}

	v := r.eval(fi, sel.X)
	if !v.isRegisterer {
		// Plain prometheus.NewGaugeVec, or a factory we do not track. The
		// registration site is unknown, so the extracted name stands.
		return nil, false
	}
	if v.resolved {
		prefixes := nonEmptyPrefixes(v.prefixes)
		if len(prefixes) == 0 {
			// Registered on a bare root registry.
			return nil, false
		}
		return prefixes, true
	}

	// The receiver is a registerer whose prefix we could not compute.
	// Treat it as managed when the value is known to pass through a
	// prefixing wrapper, or when this function is otherwise known to run
	// in a prefixed context. Everything else keeps its current output.
	if v.prefixed || r.hasPrefixedContext(fi) {
		return nil, true
	}
	return nil, false
}

// hasPrefixedContext reports whether anything in this function is known to
// carry a non-empty prefix.
func (r *prefixResolver) hasPrefixedContext(fi *funcInfo) bool {
	for _, prefixes := range r.paramPrefixes[fi.key] {
		if len(nonEmptyPrefixes(prefixes)) > 0 {
			return true
		}
	}
	if len(r.paramUnknownPrefix[fi.key]) > 0 {
		return true
	}
	if len(r.collectorPrefixes[fi.key]) > 0 {
		return true
	}
	for _, v := range fi.env {
		if v.resolved && v.hasPrefix() {
			return true
		}
	}
	return false
}

// propagate runs the interprocedural fixpoint. Only non-empty prefixes
// propagate: an unprefixed registerer carries no information, and letting
// it flow would make an unprefixed test helper contradict the real
// registration site.
func (r *prefixResolver) propagate() {
	for range maxPropagationRounds {
		changed := false
		for _, fi := range r.funcInfos {
			fi.env = r.buildEnv(fi)

			for _, c := range fi.calls {
				for i, arg := range c.args {
					v := r.eval(fi, arg)
					if v.resolved && v.hasPrefix() {
						changed = r.addParamPrefixes(c.callee, i, v.prefixes) || changed
						continue
					}
					// A prefixed registerer we cannot evaluate still has to
					// reach the callee, otherwise its metrics look bare.
					if v.prefixed && !v.resolved {
						changed = r.markUnknownPrefix(c.callee, i) || changed
					}
				}
			}

			for _, rf := range fi.registers {
				v := r.eval(fi, rf.recv)
				if !v.prefixed && !v.hasPrefix() {
					continue
				}
				for _, arg := range rf.args {
					call, ok := arg.(*ast.CallExpr)
					if !ok {
						continue
					}
					key := r.calleeKey(fi.file, call)
					if key == "" {
						continue
					}
					if !v.resolved {
						if !r.collectorUnknown[key] {
							r.collectorUnknown[key] = true
							changed = true
						}
						continue
					}
					changed = r.addCollectorPrefixes(key, v.prefixes) || changed
				}
			}
		}
		if !changed {
			return
		}
	}
	warnf("prefix: registry prefix propagation did not converge in %d rounds", maxPropagationRounds)
}

func (r *prefixResolver) addParamPrefixes(key funcKey, idx int, prefixes []string) bool {
	byIdx := r.paramPrefixes[key]
	if byIdx == nil {
		byIdx = make(map[int][]string)
		r.paramPrefixes[key] = byIdx
	}
	merged, changed := mergePrefixes(byIdx[idx], nonEmptyPrefixes(prefixes))
	byIdx[idx] = merged
	return changed
}

func (r *prefixResolver) markUnknownPrefix(key funcKey, idx int) bool {
	byIdx := r.paramUnknownPrefix[key]
	if byIdx == nil {
		byIdx = make(map[int]bool)
		r.paramUnknownPrefix[key] = byIdx
	}
	if byIdx[idx] {
		return false
	}
	byIdx[idx] = true
	return true
}

func (r *prefixResolver) addCollectorPrefixes(key funcKey, prefixes []string) bool {
	merged, changed := mergePrefixes(r.collectorPrefixes[key], nonEmptyPrefixes(prefixes))
	r.collectorPrefixes[key] = merged
	return changed
}

// buildEnv computes the local name to registerer mapping for a function.
// The analysis is flow insensitive: a name assigned two different
// registerer values collapses to "registerer with unknown prefix".
func (r *prefixResolver) buildEnv(fi *funcInfo) map[string]regValue {
	seed := make(map[string]regValue)
	for idx, prefixes := range r.paramPrefixes[fi.key] {
		if idx >= len(fi.params) {
			continue
		}
		p := fi.params[idx]
		if p.name == "" || p.variadic {
			continue
		}
		seed[p.name] = regValue{isRegisterer: true, resolved: true, prefixed: true, prefixes: prefixes}
	}
	// A parameter that also receives an unevaluable prefixed registerer
	// cannot be trusted, even when another call site resolves it.
	for idx := range r.paramUnknownPrefix[fi.key] {
		if idx >= len(fi.params) {
			continue
		}
		p := fi.params[idx]
		if p.name == "" || p.variadic {
			continue
		}
		seed[p.name] = regValue{isRegisterer: true, prefixed: true}
	}

	prev := seed
	for range maxLocalRounds {
		cur := make(map[string]regValue, len(seed))
		for k, v := range seed {
			cur[k] = v
		}
		fi.env = prev
		for _, a := range fi.assigns {
			v := r.eval(fi, a.expr)
			if !v.isRegisterer {
				continue
			}
			if old, ok := cur[a.name]; ok && !sameRegValue(old, v) {
				// Conflicting assignments: keep the value unresolved rather
				// than picking one of them.
				cur[a.name] = regValue{isRegisterer: true, prefixed: old.prefixed || v.prefixed}
				continue
			}
			cur[a.name] = v
		}
		prev = cur
	}
	fi.env = prev
	return prev
}

// eval evaluates an expression to a registerer value.
func (r *prefixResolver) eval(fi *funcInfo, expr ast.Expr) regValue {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return r.eval(fi, e.X)
	case *ast.Ident:
		return fi.env[e.Name]
	case *ast.SelectorExpr:
		return r.evalFieldRead(fi, e)
	case *ast.CallExpr:
		return r.evalCall(fi, e)
	}
	return regValue{}
}

// evalFieldRead resolves x.Field. A field is only a root when the static
// type of x is known and that type, or one of its embedded types, declares
// the field as *prometheus.Registry. When the field name matches a known
// registry field but the receiver type is unknown, the value is reported
// as an unresolved registerer rather than as an unprefixed root.
func (r *prefixResolver) evalFieldRead(fi *funcInfo, sel *ast.SelectorExpr) regValue {
	if typeKey := r.exprTypeKey(fi, sel.X); typeKey != "" {
		if r.hasRegistryField(typeKey, sel.Sel.Name, 0) {
			return regValue{isRegisterer: true, resolved: true, prefixes: []string{""}}
		}
	}
	if r.registryFieldNames[sel.Sel.Name] {
		return regValue{isRegisterer: true}
	}
	return regValue{}
}

// hasRegistryField reports whether typeKey declares field as
// *prometheus.Registry, following embedded types.
func (r *prefixResolver) hasRegistryField(typeKey, field string, depth int) bool {
	if depth > maxEmbedDepth {
		return false
	}
	if r.registryFields[typeKey+"."+field] {
		return true
	}
	for _, embedded := range r.embeds[typeKey] {
		if r.hasRegistryField(embedded, field, depth+1) {
			return true
		}
	}
	return false
}

// exprTypeKey returns the key of the static named type of an expression,
// or "" when it is not one of the bounded sources.
func (r *prefixResolver) exprTypeKey(fi *funcInfo, expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return r.exprTypeKey(fi, e.X)
	case *ast.Ident:
		return fi.varTypes[e.Name]
	}
	return ""
}

func (r *prefixResolver) evalCall(fi *funcInfo, call *ast.CallExpr) regValue {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return regValue{}
	}
	pkg := r.importPathOf(fi.file, sel.X)

	switch pkg {
	case prometheusPkgPath:
		switch sel.Sel.Name {
		case "NewRegistry", "NewPedanticRegistry":
			return regValue{isRegisterer: true, resolved: true, prefixes: []string{""}}
		case "WrapRegistererWithPrefix":
			if len(call.Args) < 2 {
				return regValue{isRegisterer: true}
			}
			return applyPrefix(r.eval(fi, call.Args[1]), r.prefixString(fi.file, call.Args[0]))
		case "WrapRegistererWith":
			// Constant labels do not change the exported name.
			if len(call.Args) < 2 {
				return regValue{isRegisterer: true}
			}
			base := r.eval(fi, call.Args[1])
			return regValue{isRegisterer: true, resolved: base.resolved, prefixed: base.prefixed, prefixes: base.prefixes}
		}
		return regValue{}
	case promautoPkgPath:
		if sel.Sel.Name != "With" || len(call.Args) != 1 {
			return regValue{}
		}
		base := r.eval(fi, call.Args[0])
		// A factory carries the prefix of the registerer it wraps.
		return regValue{isRegisterer: true, resolved: base.resolved, prefixed: base.prefixed, prefixes: base.prefixes}
	}

	// A repository helper that wraps a base registerer with a prefix.
	key := r.calleeKey(fi.file, call)
	if key == "" {
		return regValue{}
	}
	sum := r.wrapperSummaryFor(key)
	if sum == nil || !sum.ok {
		return regValue{}
	}
	if sum.baseIdx >= len(call.Args) || sum.prefixIdx >= len(call.Args) {
		return regValue{isRegisterer: true, prefixed: true}
	}
	return applyPrefix(r.eval(fi, call.Args[sum.baseIdx]), r.prefixString(fi.file, call.Args[sum.prefixIdx]))
}

// applyPrefix appends prefix to every prefix of base. Wrapping is
// outside-in: the outer wrapper's prefix follows the base's own prefix in
// the exported name, so nested wraps concatenate.
func applyPrefix(base regValue, prefix string) regValue {
	if !base.resolved || prefix == "" {
		return regValue{isRegisterer: true, prefixed: true}
	}
	prefixes := make([]string, 0, len(base.prefixes))
	for _, p := range base.prefixes {
		prefixes = append(prefixes, p+prefix)
	}
	return regValue{isRegisterer: true, resolved: true, prefixed: true, prefixes: prefixes}
}

// wrapperSummaryFor recognizes the canonical prefix argument of the repository's
// alias registerer. Alias prefixes are intentionally omitted from documentation.
func (r *prefixResolver) wrapperSummaryFor(key funcKey) *wrapperSummary {
	if key == funcKey(r.modulePath+"/coderd/prometheusmetrics.NewMetricAliasRegisterer") {
		return &wrapperSummary{ok: true, baseIdx: 0, prefixIdx: 1}
	}
	return nil
}

// indexFileHeader records the package path, imports and string constants
// of a file.
func (r *prefixResolver) indexFileHeader(f *sourceFile) {
	pkgPath := r.importPath(f.path)
	r.pkgPaths[f] = pkgPath

	imports := make(map[string]string, len(f.file.Imports))
	for _, imp := range f.file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imports[name] = path
	}
	r.imports[f] = imports

	// Reuse the declarations the scanner already collected so string
	// constants resolve across files of the same package and across
	// packages by import path.
	consts := r.pkgConsts[pkgPath]
	if consts == nil {
		consts = make(map[string]string)
		r.pkgConsts[pkgPath] = consts
	}
	for name, value := range f.decls.strings {
		consts[name] = value
	}
}

// indexTypes records *prometheus.Registry fields and embedded types,
// keyed by "importpath.Type".
func (r *prefixResolver) indexTypes(f *sourceFile) {
	for _, decl := range f.file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}
			typeKey := r.pkgPaths[f] + "." + typeSpec.Name.Name
			for _, field := range structType.Fields.List {
				if len(field.Names) == 0 {
					if embedded := r.typeKeyOf(f, field.Type); embedded != "" {
						r.embeds[typeKey] = append(r.embeds[typeKey], embedded)
					}
					continue
				}
				star, ok := field.Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				sel, ok := star.X.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Registry" || r.importPathOf(f, sel.X) != prometheusPkgPath {
					continue
				}
				for _, name := range field.Names {
					r.registryFields[typeKey+"."+name.Name] = true
					r.registryFieldNames[name.Name] = true
				}
			}
		}
	}
}

// indexSignatures records the result types of package-level functions so
// that x := pkg.New(...) yields a type for x.
func (r *prefixResolver) indexSignatures(f *sourceFile) {
	for _, decl := range f.file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv != nil || fd.Type.Results == nil {
			continue
		}
		key := funcKey(r.pkgPaths[f] + "." + fd.Name.Name)
		if _, exists := r.funcResultTypes[key]; exists {
			continue
		}
		r.funcResultTypes[key] = resultTypeKeys(r, f, fd.Type)
	}
}

// resultTypeKeys flattens a result list into one type key per result.
func resultTypeKeys(r *prefixResolver, f *sourceFile, ft *ast.FuncType) []string {
	if ft.Results == nil {
		return nil
	}
	var keys []string
	for _, field := range ft.Results.List {
		key := r.typeKeyOf(f, field.Type)
		count := max(len(field.Names), 1)
		for range count {
			keys = append(keys, key)
		}
	}
	return keys
}

// typeKeyOf maps a type expression to "importpath.Type", following one
// pointer. Unnamed and unresolvable types return "".
func (r *prefixResolver) typeKeyOf(f *sourceFile, expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return r.typeKeyOf(f, e.X)
	case *ast.Ident:
		if !ast.IsExported(e.Name) && !isLocalTypeName(e.Name) {
			return ""
		}
		return r.pkgPaths[f] + "." + e.Name
	case *ast.SelectorExpr:
		path := r.importPathOf(f, e.X)
		if path == "" {
			return ""
		}
		return path + "." + e.Sel.Name
	}
	return ""
}

// isLocalTypeName filters out predeclared types, which never carry fields
// we care about.
func isLocalTypeName(name string) bool {
	switch name {
	case "string", "bool", "error", "any", "byte", "rune", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64":
		return false
	}
	return true
}

// indexBodies summarizes the function bodies of a file.
func (r *prefixResolver) indexBodies(f *sourceFile) {
	for _, decl := range f.file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		r.indexFunc(f, fd)
	}
}

// resolveVarTypes turns the recorded assignments into local variable
// types. A single pass is enough for the shapes we support: the type
// source is a literal or a call, never a chain through another inferred
// variable.
func (r *prefixResolver) resolveVarTypes() {
	for _, fi := range r.funcInfos {
		for _, ta := range fi.typeAssigns {
			if _, ok := fi.varTypes[ta.name]; ok {
				// A name with two type sources is left with the first; the
				// shapes we support do not reassign across types.
				continue
			}
			if key := r.typeAssignKey(fi, ta); key != "" {
				fi.varTypes[ta.name] = key
			}
		}
	}
}

func (r *prefixResolver) typeAssignKey(fi *funcInfo, ta typeAssign) string {
	switch e := ta.expr.(type) {
	case *ast.UnaryExpr:
		if lit, ok := e.X.(*ast.CompositeLit); ok {
			return r.typeKeyOf(fi.file, lit.Type)
		}
	case *ast.CompositeLit:
		return r.typeKeyOf(fi.file, e.Type)
	case *ast.CallExpr:
		// A func-typed parameter, such as the newAPI hook in the server
		// command, carries its result types in its own declaration.
		if ident, ok := e.Fun.(*ast.Ident); ok {
			if ft, ok := fi.funcTypes[ident.Name]; ok {
				return nthTypeKey(resultTypeKeys(r, fi.file, ft), ta.idx)
			}
		}
		if key := r.calleeKey(fi.file, e); key != "" {
			return nthTypeKey(r.funcResultTypes[key], ta.idx)
		}
	}
	return ""
}

func nthTypeKey(keys []string, idx int) string {
	if idx < 0 || idx >= len(keys) {
		return ""
	}
	return keys[idx]
}

// indexFunc summarizes one function body into facts. The body is walked
// once; only functions that touch Prometheus or accept a registerer are
// kept, which keeps the index small and avoids repeated whole-tree walks
// during the fixpoint.
func (r *prefixResolver) indexFunc(f *sourceFile, fd *ast.FuncDecl) {
	fi := &funcInfo{
		file:      f,
		env:       map[string]regValue{},
		varTypes:  map[string]string{},
		funcTypes: map[string]*ast.FuncType{},
	}
	if fd.Recv == nil {
		fi.key = funcKey(r.pkgPaths[f] + "." + fd.Name.Name)
	}

	candidates := make(map[string]bool)
	if fd.Type.Params != nil {
		for _, field := range fd.Type.Params.List {
			_, variadic := field.Type.(*ast.Ellipsis)
			p := paramInfo{
				registerer: mentionsRegisterer(field.Type),
				variadic:   variadic,
			}
			if len(field.Names) == 0 {
				fi.params = append(fi.params, p)
				continue
			}
			typeKey := r.typeKeyOf(f, field.Type)
			funcType, _ := field.Type.(*ast.FuncType)
			for _, name := range field.Names {
				named := p
				named.name = name.Name
				fi.params = append(fi.params, named)
				if named.registerer {
					candidates[named.name] = true
				}
				if typeKey != "" {
					fi.varTypes[name.Name] = typeKey
				}
				if funcType != nil {
					fi.funcTypes[name.Name] = funcType
				}
			}
		}
	}

	interesting := len(candidates) > 0
	var calls []*ast.CallExpr

	// Function literals are walked as part of their enclosing function, so
	// registerers created inside command handlers are visible.
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			if path := r.imports[f][node.Name]; path == prometheusPkgPath || path == promautoPkgPath {
				interesting = true
			}
		case *ast.AssignStmt:
			if len(node.Rhs) == 1 {
				// Multi-value calls reveal the type of each assigned name.
				for i, lhs := range node.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok && ident.Name != "_" {
						fi.typeAssigns = append(fi.typeAssigns, typeAssign{name: ident.Name, expr: node.Rhs[0], idx: i})
					}
				}
			}
			if len(node.Lhs) != len(node.Rhs) {
				return true
			}
			for i, lhs := range node.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name == "_" {
					continue
				}
				if !r.maybeRegisterer(f, node.Rhs[i], candidates) {
					continue
				}
				fi.assigns = append(fi.assigns, assignFact{name: ident.Name, expr: node.Rhs[i]})
				candidates[ident.Name] = true
			}
		case *ast.ValueSpec:
			for i, name := range node.Names {
				if i >= len(node.Values) || name.Name == "_" {
					continue
				}
				if !r.maybeRegisterer(f, node.Values[i], candidates) {
					continue
				}
				fi.assigns = append(fi.assigns, assignFact{name: name.Name, expr: node.Values[i]})
				candidates[name.Name] = true
			}
		case *ast.CallExpr:
			calls = append(calls, node)
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if (sel.Sel.Name == "MustRegister" || sel.Sel.Name == "Register") &&
					r.maybeRegisterer(f, sel.X, candidates) {
					fi.registers = append(fi.registers, registerFact{recv: sel.X, args: node.Args})
					interesting = true
				}
			}
			if key := r.calleeKey(f, node); key != "" {
				if slices.ContainsFunc(node.Args, func(a ast.Expr) bool { return r.maybeRegisterer(f, a, candidates) }) {
					fi.calls = append(fi.calls, callFact{callee: key, args: node.Args})
					interesting = true
				}
			}
		}
		return true
	})

	if !interesting && len(fi.assigns) == 0 {
		return
	}

	r.funcInfos = append(r.funcInfos, fi)
	for _, call := range calls {
		r.callFunc[call] = fi
	}
}

// maybeRegisterer is the syntactic pre-filter that decides whether an
// expression is worth keeping as a fact. It is intentionally permissive;
// eval decides the truth.
func (r *prefixResolver) maybeRegisterer(f *sourceFile, expr ast.Expr, candidates map[string]bool) bool {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return r.maybeRegisterer(f, e.X, candidates)
	case *ast.Ident:
		return candidates[e.Name]
	case *ast.SelectorExpr:
		return r.registryFieldNames[e.Sel.Name]
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if ok {
			switch r.importPathOf(f, sel.X) {
			case prometheusPkgPath:
				if strings.HasPrefix(sel.Sel.Name, "NewRegistry") ||
					strings.HasPrefix(sel.Sel.Name, "NewPedanticRegistry") ||
					strings.HasPrefix(sel.Sel.Name, "WrapRegisterer") {
					return true
				}
			case promautoPkgPath:
				if sel.Sel.Name == "With" {
					return true
				}
			}
		}
		// A helper that takes a registerer and returns one, for example a
		// prefixing or aliasing wrapper.
		return slices.ContainsFunc(e.Args, func(a ast.Expr) bool { return r.maybeRegisterer(f, a, candidates) })
	}
	return false
}

// calleeKey resolves the called function to an index key, handling import
// aliases. Method calls and calls into unscanned packages return "".
func (r *prefixResolver) calleeKey(f *sourceFile, call *ast.CallExpr) funcKey {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return funcKey(r.pkgPaths[f] + "." + fun.Name)
	case *ast.SelectorExpr:
		path := r.importPathOf(f, fun.X)
		if path == "" {
			return ""
		}
		return funcKey(path + "." + fun.Sel.Name)
	}
	return ""
}

// importPathOf resolves a package qualifier to its import path.
func (r *prefixResolver) importPathOf(f *sourceFile, expr ast.Expr) string {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return ""
	}
	return r.imports[f][ident.Name]
}

// isPkgCall reports whether call is pkgPath.name(...).
func (r *prefixResolver) isPkgCall(f *sourceFile, call *ast.CallExpr, pkgPath, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	return r.importPathOf(f, sel.X) == pkgPath
}

// prefixString resolves a prefix expression to its constant value.
// Identifiers resolve against the file's declarations and then the
// package's constants; selectors resolve through the import path, so
// aliased imports work.
func (r *prefixResolver) prefixString(f *sourceFile, expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return r.prefixString(f, e.X)
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return ""
		}
		value, err := strconv.Unquote(e.Value)
		if err != nil {
			return ""
		}
		return value
	case *ast.Ident:
		if value, ok := f.decls.strings[e.Name]; ok {
			return value
		}
		return r.pkgConsts[r.pkgPaths[f]][e.Name]
	case *ast.SelectorExpr:
		path := r.importPathOf(f, e.X)
		if path == "" {
			return ""
		}
		return r.pkgConsts[path][e.Sel.Name]
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return ""
		}
		left, right := r.prefixString(f, e.X), r.prefixString(f, e.Y)
		if left == "" || right == "" {
			return ""
		}
		return left + right
	}
	return ""
}

// importPath maps a repository-relative file path to its import path.
func (r *prefixResolver) importPath(path string) string {
	dir := filepath.ToSlash(filepath.Dir(path))
	if dir == "." || dir == "" {
		return r.modulePath
	}
	return r.modulePath + "/" + dir
}

// readModulePath reads the module path from go.mod in the working
// directory, which is the repository root when the scanner runs.
func readModulePath() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return defaultModulePath
	}
	for line := range strings.Lines(string(data)) {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(path)
		}
	}
	return defaultModulePath
}

func mentionsRegisterer(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		switch ident.Name {
		case "Registerer", "Registry", "Gatherer":
			found = true
		}
		return true
	})
	return found
}

func nonEmptyPrefixes(prefixes []string) []string {
	out := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// mergePrefixes unions two prefix sets and reports whether dst grew.
func mergePrefixes(dst, src []string) ([]string, bool) {
	changed := false
	for _, p := range src {
		if !slices.Contains(dst, p) {
			dst = append(dst, p)
			changed = true
		}
	}
	if changed {
		slices.Sort(dst)
	}
	return dst, changed
}

func sameRegValue(a, b regValue) bool {
	return a.isRegisterer == b.isRegisterer && a.resolved == b.resolved &&
		a.prefixed == b.prefixed && slices.Equal(a.prefixes, b.prefixes)
}
