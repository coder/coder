// Package main provides a tool to scan Go source files and extract Prometheus
// metric definitions. It outputs metrics in Prometheus text exposition format
// to stdout for use by the documentation generator.
//
// Usage:
//
//	go run ./scripts/metricsdocgen/scanner > scripts/metricsdocgen/generated_metrics
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/term"
	"golang.org/x/xerrors"
)

// Directories to scan for metric definitions, relative to the repository root.
// Add or remove directories here to control the scanner's scope.
var scanDirs = []string{
	"agent",
	"aibridge",
	"coderd",
	"enterprise",
	"provisionerd",
	"tailnet",
}

// skipPaths lists files that should be excluded from scanning. Their metrics
// must be maintained in the static metrics file instead.
var skipPaths = []string{
	"enterprise/scaletest/agentfake/metrics.go",
}

// canonicalPrefixConst is the name of the constant a package declares to state
// the Prometheus prefix that its registration wiring applies at runtime.
const canonicalPrefixConst = "PrometheusMetricPrefix"

// knownMetricNamespaces lists the namespaces that scanned metric names are
// expected to carry. A name outside this set means the metric is registered
// through a prefixing registerer whose prefix the scanner does not know about,
// so validateMetricNamespaces rejects it rather than emitting a name that no
// scrape produces.
var knownMetricNamespaces = []string{
	"agent_",
	"coder_",
	"coderd_",
}

// metricPrefixSources maps a file that defines metrics to the file declaring
// the canonical prefix constant applied to them. Keying by the defining file,
// rather than by metric name, lets the scanner bypass the registration wiring,
// and reading the constant rather than repeating its value keeps the generated
// names in step with a prefix rename. Deprecated alias prefixes stay out of the
// generated reference because only the canonical constant is read.
//
// A new file whose metrics are prefixed at registration needs an entry here.
// Two checks catch most missing entries: validateMetricNamespaces rejects the
// resulting name when it carries no known namespace, and
// TestScanAIGatewayFilesAreMapped in scanner_test.go walks the AI Gateway trees
// for unmapped metric files. Neither catches a file outside those trees whose
// options already set a known Namespace, because the scanned name then looks
// valid while the registration wiring adds a further prefix.
var metricPrefixSources = map[string]string{
	"aibridge/keypool/state_collector.go":  "aibridge/metrics/metrics.go",
	"aibridge/metrics/metrics.go":          "aibridge/metrics/metrics.go",
	"coderd/aibridged/metrics.go":          "aibridge/metrics/metrics.go",
	"coderd/aibridgedserver/metrics.go":    "aibridge/metrics/metrics.go",
	"enterprise/aibridgeproxyd/metrics.go": "enterprise/aibridgeproxyd/metrics.go",
}

// metricPrefixPaths holds the keys of metricPrefixSources ordered by descending
// length, which is defensive against a future key that could match the same
// file as another key. No current pair of keys is suffix-comparable.
var metricPrefixPaths = sortedMetricPrefixPaths()

func sortedMetricPrefixPaths() []string {
	return slices.SortedFunc(maps.Keys(metricPrefixSources), func(a, b string) int {
		if diff := len(b) - len(a); diff != 0 {
			return diff
		}
		return strings.Compare(a, b)
	})
}

// canonicalPrefixes caches the constant value read from each prefix source
// file, keyed by that file's path. The scanner runs in a single goroutine.
var canonicalPrefixes = map[string]string{}

// MetricType represents the type of Prometheus metric.
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// Metric represents a single Prometheus metric definition extracted from source code.
type Metric struct {
	Name   string     // Full metric name (namespace_subsystem_name)
	Type   MetricType // counter, gauge, histogram, or summary
	Help   string     // Description of the metric
	Labels []string   // Label names for this metric
}

// metricOpts holds the fields extracted from a prometheus.*Opts struct.
type metricOpts struct {
	Namespace string
	Subsystem string
	Name      string
	Help      string
}

// declarations holds const/var values collected from a file for resolving references.
type declarations struct {
	strings      map[string]string   // string constants/variables
	stringSlices map[string][]string // []string variables
}

// packageDeclarations holds exported string constants collected from all scanned files,
// keyed by package name. This allows resolving cross-file references.
// Note: resolution depends on directory scan order in scanDirs, i.e.,
// constants from later directories won't be available when scanning earlier ones.
var packageDeclarations = make(map[string]map[string]string)

// verbose controls whether informational messages are printed to
// stderr. It is true when stdout is a terminal (interactive use)
// and false when stdout is piped (e.g. via atomic_write in make).
var verbose = term.IsTerminal(int(os.Stdout.Fd()))

// logf prints an informational message to stderr only when running
// interactively. Use this for progress and debug output that is
// not actionable.
func logf(format string, args ...any) {
	if verbose {
		log.Printf(format, args...)
	}
}

// warnf prints a warning to stderr unconditionally. Use this for
// messages about real problems that a developer should investigate.
func warnf(format string, args ...any) {
	log.Printf("WARNING: "+format, args...)
}

func main() {
	metrics, err := scanAllDirs()
	if err != nil {
		log.Fatalf("Failed to scan directories: %v", err)
	}
	metrics = prepareMetrics(metrics)
	if err := validateMetricNamespaces(metrics); err != nil {
		log.Fatalf("Failed to validate metric names: %v", err)
	}

	writeMetrics(metrics, os.Stdout)
	logf("Successfully parsed %d metrics", len(metrics))
}

// validateMetricNamespaces rejects scanned metrics whose names carry no known
// namespace. Such a name means the metrics are prefixed by their registerer and
// the defining file has no metricPrefixSources entry, which would document a
// name that no scrape produces.
func validateMetricNamespaces(metrics []Metric) error {
	var unknown []string
	for _, metric := range metrics {
		if !slices.ContainsFunc(knownMetricNamespaces, func(namespace string) bool {
			return strings.HasPrefix(metric.Name, namespace)
		}) {
			unknown = append(unknown, metric.Name)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	return xerrors.Errorf("metrics carry no known namespace (%v): set Namespace in the metric options, "+
		"add a metricPrefixSources entry when a registerer applies the prefix, "+
		"or extend knownMetricNamespaces: %s",
		knownMetricNamespaces, strings.Join(unknown, ", "))
}

// prepareMetrics deduplicates and sorts the scanned metrics. Duplicates are not
// expected since Prometheus enforces unique metric names at registration, so the
// deduplication is a safety net rather than a functional requirement. Sorting
// keeps the generated output stable across runs.
func prepareMetrics(metrics []Metric) []Metric {
	uniqueMetrics := make(map[string]Metric)
	for _, metric := range metrics {
		uniqueMetrics[metric.Name] = metric
	}
	return slices.SortedFunc(maps.Values(uniqueMetrics), func(a, b Metric) int {
		return strings.Compare(a.Name, b.Name)
	})
}

// scanAllDirs scans all configured directories for metric definitions.
func scanAllDirs() ([]Metric, error) {
	var allMetrics []Metric

	for _, dir := range scanDirs {
		metrics, err := scanDirectory(dir)
		if err != nil {
			return nil, xerrors.Errorf("scanning %s: %w", dir, err)
		}

		logf("scanning %s: found %d metrics", dir, len(metrics))
		allMetrics = append(allMetrics, metrics...)
	}

	return allMetrics, nil
}

// scanDirectory recursively walks a directory and extracts metrics from all Go files.
func scanDirectory(root string) ([]Metric, error) {
	var metrics []Metric

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip non-Go files.
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files.
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Skip files listed in skipPaths.
		for _, sp := range skipPaths {
			if path == sp {
				return nil
			}
		}

		fileMetrics, err := scanFile(path)
		if err != nil {
			return xerrors.Errorf("scanning %s: %w", path, err)
		}

		if len(fileMetrics) > 0 {
			logf("scanning %s: found %d metrics", path, len(fileMetrics))
		}
		metrics = append(metrics, fileMetrics...)

		return nil
	})

	return metrics, err
}

// scanFile parses a single Go file and extracts all Prometheus metric definitions.
func scanFile(path string) ([]Metric, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, xerrors.Errorf("parsing file: %w", err)
	}

	// Collect exported constants into the global package declarations map.
	collectPackageConsts(file)

	// Collect file-local const and var declarations for resolving references.
	decls := collectDecls(file)

	prefix, err := metricPrefixForPath(path)
	if err != nil {
		return nil, err
	}

	var metrics []Metric

	// Walk the AST looking for metric registration calls.
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		metric, ok := extractMetricFromCall(call, decls)
		if ok {
			metric.Name = prefix + metric.Name
			if metric.Help == "" {
				warnf("metric %q has no HELP description, skipping", metric.Name)
				// Skip metrics without descriptions, they should be fixed in the source code
				// or added to the static metrics file with a manual description.
				return true
			}
			metrics = append(metrics, metric)
		}

		return true
	})

	return metrics, nil
}

// metricPrefixForPath returns the canonical prefix that the registration wiring
// applies to the metrics defined in path, or an empty string when the file
// defines unprefixed metrics.
func metricPrefixForPath(path string) (string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	for _, definingPath := range metricPrefixPaths {
		if cleaned != definingPath && !strings.HasSuffix(cleaned, "/"+definingPath) {
			continue
		}
		prefix, err := canonicalPrefix(metricPrefixSources[definingPath])
		if err != nil {
			return "", xerrors.Errorf("resolving metric prefix for %s: %w", path, err)
		}
		return prefix, nil
	}
	return "", nil
}

// canonicalPrefix reads canonicalPrefixConst from the given file. The path is
// relative to the repository root, which is the scanner's working directory.
func canonicalPrefix(sourcePath string) (string, error) {
	if prefix, ok := canonicalPrefixes[sourcePath]; ok {
		return prefix, nil
	}

	file, err := parser.ParseFile(token.NewFileSet(), sourcePath, nil, parser.SkipObjectResolution)
	if err != nil {
		return "", xerrors.Errorf("parsing %s: %w", sourcePath, err)
	}
	prefix, ok := collectDecls(file).strings[canonicalPrefixConst]
	if !ok {
		return "", xerrors.Errorf("%s does not declare %s", sourcePath, canonicalPrefixConst)
	}
	canonicalPrefixes[sourcePath] = prefix
	return prefix, nil
}

// collectPackageConsts collects exported string constants from a file into
// the global packageDeclarations map, keyed by package name.
func collectPackageConsts(file *ast.File) {
	pkgName := file.Name.Name

	if packageDeclarations[pkgName] == nil {
		packageDeclarations[pkgName] = make(map[string]string)
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for i, name := range valueSpec.Names {
				if !ast.IsExported(name.Name) {
					continue
				}

				if i >= len(valueSpec.Values) {
					continue
				}

				if lit, ok := valueSpec.Values[i].(*ast.BasicLit); ok {
					if lit.Kind == token.STRING {
						packageDeclarations[pkgName][name.Name] = strings.Trim(lit.Value, `"`)
					}
				}
			}
		}
	}
}

// resolveStringExpr attempts to resolve an expression to a string value.
// Examples:
//   - "my_metric": "my_metric" (string literal)
//   - metricName: resolved value of metricName constant (identifier)
//   - agentmetrics.LabelUsername: resolved from package constants (selector)
func resolveStringExpr(expr ast.Expr, decls declarations) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return strings.Trim(e.Value, `"`)
	case *ast.Ident:
		return decls.strings[e.Name]
	case *ast.BinaryExpr:
		return resolveBinaryExpr(e, decls)
	case *ast.SelectorExpr:
		// Handle pkg.Const syntax.
		if ident, ok := e.X.(*ast.Ident); ok {
			if pkgConsts, ok := packageDeclarations[ident.Name]; ok {
				return pkgConsts[e.Sel.Name]
			}
		}
	}

	return ""
}

// resolveBinaryExpr resolves a binary expression (string concatenation) to a string.
// It recursively resolves the left and right operands.
// Example:
//   - "coderd_" + "api_" + "requests": "coderd_api_requests"
//   - namespace + "_" + metricName: resolved concatenation
func resolveBinaryExpr(expr *ast.BinaryExpr, decls declarations) string {
	left := resolveStringExpr(expr.X, decls)
	right := resolveStringExpr(expr.Y, decls)
	if left != "" && right != "" {
		return left + right
	}
	return ""
}

// extractStringSlice extracts a []string from a composite literal.
// Example:
//   - []string{"a", "b", myConst}: ["a", "b", <resolved value of myConst>]
func extractStringSlice(lit *ast.CompositeLit, decls declarations) []string {
	var labels []string
	for _, elt := range lit.Elts {
		if label := resolveStringExpr(elt, decls); label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}

// collectDecls collects const and var declarations from a file.
// This is used to resolve constant and variable references in metric definitions.
func collectDecls(file *ast.File) declarations {
	decls := declarations{
		strings:      make(map[string]string),
		stringSlices: make(map[string][]string),
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for i, name := range valueSpec.Names {
				if i >= len(valueSpec.Values) {
					continue
				}

				switch v := valueSpec.Values[i].(type) {
				case *ast.BasicLit:
					// String literal: const name = "value"
					decls.strings[name.Name] = strings.Trim(v.Value, `"`)
				case *ast.BinaryExpr:
					// Concatenation: const name = prefix + "suffix"
					if resolved := resolveBinaryExpr(v, decls); resolved != "" {
						decls.strings[name.Name] = resolved
					}
				case *ast.CompositeLit:
					// Slice literal: var labels = []string{"a", "b"}
					if resolved := extractStringSlice(v, decls); resolved != nil {
						decls.stringSlices[name.Name] = resolved
					}
				}
			}
		}
	}

	return decls
}

// extractLabels extracts label names from an expression passed as an argument
// to a metric constructor. It handles inline literals, variable references,
// and append calls that extend a shared label slice.
func extractLabels(expr ast.Expr, decls declarations) []string {
	switch e := expr.(type) {
	case *ast.CompositeLit:
		return extractStringSlice(e, decls)
	case *ast.Ident:
		return slices.Clone(decls.stringSlices[e.Name])
	case *ast.CallExpr:
		ident, ok := e.Fun.(*ast.Ident)
		if !ok || ident.Name != "append" || len(e.Args) < 2 {
			return nil
		}

		labels := extractLabels(e.Args[0], decls)
		for _, arg := range e.Args[1:] {
			if label := resolveStringExpr(arg, decls); label != "" {
				labels = append(labels, label)
				continue
			}
			labels = append(labels, extractLabels(arg, decls)...)
		}
		return labels
	}
	return nil
}

// extractNewDescMetric extracts a metric from a prometheus.NewDesc() call.
// Pattern: prometheus.NewDesc(name, help, variableLabels, constLabels)
// Currently, coder only uses MustNewConstMetric with NewDesc.
// TODO(ssncferreira): Add support for other MustNewConst* functions if needed.
func extractNewDescMetric(call *ast.CallExpr, decls declarations) (Metric, bool) {
	// Check if this is a prometheus.NewDesc call.
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return Metric{}, false
	}

	// Match calls that are exactly "prometheus.NewDesc()". This checks the local
	// package identifier, not the resolved import path. If the prometheus package
	// is imported with an alias, this will not match.
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != "prometheus" || sel.Sel.Name != "NewDesc" {
		return Metric{}, false
	}

	// NewDesc requires at least 4 arguments: name, help, variableLabels, constLabels
	if len(call.Args) < 4 {
		return Metric{}, false
	}

	// Extract name (first argument).
	name := resolveStringExpr(call.Args[0], decls)
	if name == "" {
		warnf("extractNewDescMetric: skipping prometheus.NewDesc() call: could not resolve metric name")
		return Metric{}, false
	}

	// Extract help (second argument).
	help := resolveStringExpr(call.Args[1], decls)

	// Extract labels (third argument).
	labels := extractLabels(call.Args[2], decls)

	// Infer metric type from name suffix.
	// TODO(ssncferreira): The actual type is determined by the MustNewConst* function
	// 	that uses this descriptor (e.g., MustNewConstMetric with prometheus.CounterValue or
	// 	prometheus.GaugeValue). Currently, coder only uses MustNewConstMetric, so we
	// 	infer the type from naming conventions.
	metricType := MetricTypeGauge
	if strings.HasSuffix(name, "_total") || strings.HasSuffix(name, "_count") {
		metricType = MetricTypeCounter
	}

	return Metric{
		Name:   name,
		Type:   metricType,
		Help:   help,
		Labels: labels,
	}, true
}

// parseMetricFuncName parses a prometheus function name and returns the metric type
// and whether it's a Vec type. Returns empty string if not a recognized metric function.
func parseMetricFuncName(funcName string) (MetricType, bool) {
	isVec := strings.HasSuffix(funcName, "Vec")
	baseName := strings.TrimSuffix(funcName, "Vec")

	switch baseName {
	case "NewGauge":
		return MetricTypeGauge, isVec
	case "NewCounter":
		return MetricTypeCounter, isVec
	case "NewHistogram":
		return MetricTypeHistogram, isVec
	case "NewSummary":
		return MetricTypeSummary, isVec
	}
	return "", false
}

// extractOpts extracts fields from a prometheus.*Opts composite literal.
func extractOpts(expr ast.Expr, decls declarations) (metricOpts, bool) {
	// Handle both direct composite literals and calls that return opts.
	var lit *ast.CompositeLit

	switch e := expr.(type) {
	case *ast.CompositeLit:
		lit = e
	case *ast.UnaryExpr:
		// Handle &prometheus.GaugeOpts{...}
		if l, ok := e.X.(*ast.CompositeLit); ok {
			lit = l
		}
	}

	if lit == nil {
		return metricOpts{}, false
	}

	var opts metricOpts
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}

		value := resolveStringExpr(kv.Value, decls)

		switch key.Name {
		case "Namespace":
			opts.Namespace = value
		case "Subsystem":
			opts.Subsystem = value
		case "Name":
			opts.Name = value
		case "Help":
			opts.Help = value
		}
	}

	return opts, opts.Name != ""
}

// buildMetricName constructs the full metric name from namespace, subsystem, and name.
func buildMetricName(namespace, subsystem, name string) string {
	metricNameParts := make([]string, 0, 3)
	if namespace != "" {
		metricNameParts = append(metricNameParts, namespace)
	}
	if subsystem != "" {
		metricNameParts = append(metricNameParts, subsystem)
	}
	if name != "" {
		metricNameParts = append(metricNameParts, name)
	}
	// Join non-empty parts with "_" to handle optional namespace/subsystem.
	// e.g., ("coderd", "", "agents_up"): "coderd_agents_up"
	return strings.Join(metricNameParts, "_")
}

// extractOptsMetric extracts a metric from prometheus.New*() or prometheus.New*Vec() calls.
// Supported patterns:
//   - prometheus.NewGauge(prometheus.GaugeOpts{...})
//   - prometheus.NewCounter(prometheus.CounterOpts{...})
//   - prometheus.NewHistogram(prometheus.HistogramOpts{...})
//   - prometheus.NewSummary(prometheus.SummaryOpts{...})
//   - prometheus.NewGaugeVec(prometheus.GaugeOpts{...}, labels)
//   - prometheus.NewCounterVec(prometheus.CounterOpts{...}, labels)
//   - prometheus.NewHistogramVec(prometheus.HistogramOpts{...}, labels)
//   - prometheus.NewSummaryVec(prometheus.SummaryOpts{...}, labels)
func extractOptsMetric(call *ast.CallExpr, decls declarations) (Metric, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return Metric{}, false
	}

	// Match calls that are exactly "prometheus.New*(...)". This checks the local
	// package identifier, not the resolved import path. If the prometheus package
	// is imported with an alias, this will not match.
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != "prometheus" {
		return Metric{}, false
	}

	funcName := sel.Sel.Name
	metricType, isVec := parseMetricFuncName(funcName)
	if metricType == "" {
		return Metric{}, false
	}

	// Need at least one argument (the Opts struct).
	if len(call.Args) < 1 {
		return Metric{}, false
	}

	// Extract metric info from the Opts struct.
	opts, ok := extractOpts(call.Args[0], decls)
	if !ok {
		warnf("extractOptsMetric: skipping prometheus.%s() call: could not extract opts", funcName)
		return Metric{}, false
	}

	// Extract labels for Vec types.
	var labels []string
	if isVec && len(call.Args) >= 2 {
		labels = extractLabels(call.Args[1], decls)
	}

	// Build the full metric name.
	name := buildMetricName(opts.Namespace, opts.Subsystem, opts.Name)
	if name == "" {
		warnf("extractOptsMetric: skipping prometheus.%s() call: could not build metric name", funcName)
		return Metric{}, false
	}

	return Metric{
		Name:   name,
		Type:   metricType,
		Help:   opts.Help,
		Labels: labels,
	}, true
}

// isPromautoCall checks if an expression is a promauto factory call.
// Matches:
//   - promauto.With(reg): direct chained call
//   - factory: variable that was assigned from promauto.With()
func isPromautoCall(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.CallExpr:
		// Check for promauto.With(reg).New*()
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return false
		}
		// Match calls that are exactly "promauto.With(...)". This checks the local
		// package identifier, not the resolved import path. If the promauto package
		// is imported with an alias, this will not match.
		return ident.Name == "promauto" && sel.Sel.Name == "With"
	case *ast.Ident:
		// Heuristic: assume any identifier that isn't "prometheus" used as a
		// receiver for New*() methods is a promauto factory variable.
		// This works for the codebase patterns (e.g., factory.NewGaugeVec(...))
		// but could false-positive on other receivers. Downstream extractOpts
		// validation prevents incorrect metrics from being emitted.
		return e.Name != "prometheus"
	}
	return false
}

// extractPromautoMetric extracts a metric from promauto.With().New*() or factory.New*() calls.
// Supported patterns:
//   - promauto.With(reg).NewCounterVec(prometheus.CounterOpts{...}, labels)
//   - factory.NewGaugeVec(prometheus.GaugeOpts{...}, labels) where factory := promauto.With(reg)
func extractPromautoMetric(call *ast.CallExpr, decls declarations) (Metric, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return Metric{}, false
	}

	funcName := sel.Sel.Name
	metricType, isVec := parseMetricFuncName(funcName)
	if metricType == "" {
		return Metric{}, false
	}

	// Check if this is a promauto call by examining the receiver.
	if !isPromautoCall(sel.X) {
		return Metric{}, false
	}

	// Need at least one argument (the Opts struct).
	if len(call.Args) < 1 {
		return Metric{}, false
	}

	// Extract metric info from the Opts struct.
	opts, ok := extractOpts(call.Args[0], decls)
	if !ok {
		warnf("extractPromautoMetric: skipping promauto.%s() call: could not extract opts", funcName)
		return Metric{}, false
	}

	// Extract labels for Vec types.
	var labels []string
	if isVec && len(call.Args) >= 2 {
		labels = extractLabels(call.Args[1], decls)
	}

	// Build the full metric name.
	name := buildMetricName(opts.Namespace, opts.Subsystem, opts.Name)
	if name == "" {
		warnf("extractPromautoMetric: skipping promauto.%s() call: could not build metric name", funcName)
		return Metric{}, false
	}

	return Metric{
		Name:   name,
		Type:   metricType,
		Help:   opts.Help,
		Labels: labels,
	}, true
}

// extractMetricFromCall attempts to extract a Metric from a function call expression.
// It returns the metric and true if successful, or an empty metric and false if
// the call is not a metric registration.
//
// Supported patterns:
//   - prometheus.NewDesc() calls
//   - prometheus.New*() and prometheus.New*Vec() with *Opts{}
//   - promauto.With(reg).New*() and factory.New*() patterns
func extractMetricFromCall(call *ast.CallExpr, decls declarations) (Metric, bool) {
	// Check for prometheus.NewDesc() pattern.
	if metric, ok := extractNewDescMetric(call, decls); ok {
		return metric, true
	}

	// Check for prometheus.New*() and prometheus.New*Vec() patterns.
	if metric, ok := extractOptsMetric(call, decls); ok {
		return metric, true
	}

	// Check for promauto.With(reg).New*() pattern.
	if metric, ok := extractPromautoMetric(call, decls); ok {
		return metric, true
	}

	return Metric{}, false
}

// String returns the metric in Prometheus text exposition format.
// Label values are empty strings and metric values are 0 since only
// metadata (name, type, help, label names) is used for documentation generation.
func (m Metric) String() string {
	var buf strings.Builder

	// Write HELP line.
	_, _ = fmt.Fprintf(&buf, "# HELP %s %s\n", m.Name, m.Help)

	// Write TYPE line.
	_, _ = fmt.Fprintf(&buf, "# TYPE %s %s\n", m.Name, m.Type)

	// Write a sample metric line with empty label values and zero metric value.
	if len(m.Labels) > 0 {
		labelPairs := make([]string, len(m.Labels))
		for i, l := range m.Labels {
			labelPairs[i] = fmt.Sprintf("%s=\"\"", l)
		}
		_, _ = fmt.Fprintf(&buf, "%s{%s} 0\n", m.Name, strings.Join(labelPairs, ","))
	} else {
		_, _ = fmt.Fprintf(&buf, "%s 0\n", m.Name)
	}

	return buf.String()
}

// writeMetrics writes all metrics in Prometheus text exposition format.
func writeMetrics(metrics []Metric, w io.Writer) {
	for _, m := range metrics {
		_, _ = fmt.Fprint(w, m.String())
	}
}
