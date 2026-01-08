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
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/xerrors"
)

// Directories to scan for metric definitions, relative to the repository root.
// Add or remove directories here to control the scanner's scope.
var scanDirs = []string{
	"agent",
	"coderd",
	"enterprise",
	"provisionerd",
}

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

// declarations holds const/var values collected from a file for resolving references.
type declarations struct {
	strings      map[string]string   // string constants/variables
	stringSlices map[string][]string // []string variables
}

func main() {
	metrics, err := scanAllDirs()
	if err != nil {
		log.Fatalf("Failed to scan directories: %v", err)
	}

	// Duplicates are not expected since Prometheus enforces unique metric names at registration.
	uniqueMetrics := make(map[string]Metric)
	for _, m := range metrics {
		uniqueMetrics[m.Name] = m
	}
	metrics = make([]Metric, 0, len(uniqueMetrics))
	for _, m := range uniqueMetrics {
		metrics = append(metrics, m)
	}

	// Sort metrics by name for consistent output across runs.
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].Name < metrics[j].Name
	})

	writeMetrics(metrics, os.Stdout)

	log.Printf("Successfully parsed %d metrics", len(metrics))
}

// scanAllDirs scans all configured directories for metric definitions.
func scanAllDirs() ([]Metric, error) {
	var allMetrics []Metric

	for _, dir := range scanDirs {
		metrics, err := scanDirectory(dir)
		if err != nil {
			return nil, xerrors.Errorf("scanning %s: %w", dir, err)
		}

		log.Printf("scanning %s: found %d metrics", dir, len(metrics))
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

		fileMetrics, err := scanFile(path)
		if err != nil {
			return xerrors.Errorf("scanning %s: %w", path, err)
		}

		if len(fileMetrics) > 0 {
			log.Printf("scanning %s: found %d metrics", path, len(fileMetrics))
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

	// First pass: collect const and var declarations for resolving references.
	decls := collectDecls(file)

	var metrics []Metric

	// Walk the AST looking for metric registration calls.
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		metric, ok := extractMetricFromCall(call, decls)
		if ok {
			metrics = append(metrics, metric)
		}

		return true
	})

	return metrics, nil
}

// resolveStringExpr attempts to resolve an expression to a string value.
// Examples:
//   - "my_metric": "my_metric" (string literal)
//   - metricName: resolved value of metricName constant (identifier)
func resolveStringExpr(expr ast.Expr, decls declarations) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return strings.Trim(e.Value, `"`)
	case *ast.Ident:
		return decls.strings[e.Name]
	case *ast.BinaryExpr:
		return resolveBinaryExpr(e, decls)
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
					if labels := extractStringSlice(v, decls); labels != nil {
						decls.stringSlices[name.Name] = labels
					}
				}
			}
		}
	}

	return decls
}

// extractLabels extracts label names from an expression.
// Handles []string{...} literals and variable references.
func extractLabels(expr ast.Expr, decls declarations) []string {
	switch e := expr.(type) {
	case *ast.CompositeLit:
		// []string{"label1", "label2"}
		return extractStringSlice(e, decls)
	case *ast.Ident:
		// Variable reference like 'labels'.
		if labels, ok := decls.stringSlices[e.Name]; ok {
			return labels
		}
		return nil
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

	// TODO(ssncferreira): Implement upstack.
	// 	Handle prometheus.New*Vec() and prometheus.New*() with *Opts{}
	// 	Handle promauto.With(reg).New*() pattern

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
