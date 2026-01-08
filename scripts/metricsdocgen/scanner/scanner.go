// Package main provides a tool to scan Go source files and extract Prometheus
// metric definitions. It outputs metrics in Prometheus text format for use
// by the documentation generator.
//
// Usage:
//
//	go run ./scripts/metricsdocgen/scanner [-output FILE]
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/xerrors"
)

// Directories to scan for metric definitions, relative to the repository root.
var scanDirs = []string{
	"agent",
	"coderd",
	"enterprise",
	"provisionerd",
}

// MetricType represents the type of a Prometheus metric.
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

var outputFile string

func main() {
	flag.StringVar(&outputFile, "output", "scripts/metricsdocgen/generated_metrics", "Output file path")
	flag.Parse()

	metrics, err := scanAllDirs()
	if err != nil {
		log.Fatalf("Failed to scan directories: %v", err)
	}

	// Sort metrics by name for consistent output across runs.
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].Name < metrics[j].Name
	})

	if err := writeMetrics(metrics, outputFile); err != nil {
		log.Fatalf("Failed to write metrics: %v", err)
	}

	_, _ = fmt.Fprintf(os.Stderr, "Wrote %d metrics to %s\n", len(metrics), outputFile)
}

// scanAllDirs scans all configured directories for metric definitions.
func scanAllDirs() ([]Metric, error) {
	var allMetrics []Metric

	for _, dir := range scanDirs {
		metrics, err := scanDirectory(dir)
		if err != nil {
			return nil, xerrors.Errorf("scanning %s: %w", dir, err)
		}
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
		metrics = append(metrics, fileMetrics...)

		return nil
	})

	return metrics, err
}

// scanFile parses a single Go file and extracts all Prometheus metric definitions.
func scanFile(path string) ([]Metric, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, xerrors.Errorf("parsing file: %w", err)
	}

	var metrics []Metric

	// Walk the AST looking for metric registration calls.
	ast.Inspect(file, func(n ast.Node) bool {
		_, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		metric, ok := extractMetricFromCall()
		if ok {
			metrics = append(metrics, metric)
		}

		return true
	})

	return metrics, nil
}

// extractMetricFromCall attempts to extract a Metric from a function call expression.
// It returns the metric and true if successful, or an empty metric and false if
// the call is not a metric registration.
//
// Supported patterns:
//   - prometheus.NewDesc() calls
//   - prometheus.New*() and prometheus.New*Vec() with *Opts{}
//   - promauto.With(reg).New*() and factory.New*() patterns
func extractMetricFromCall() (Metric, bool) {
	// TODO(ssncferreira): Implement upstack.
	// 	Handle prometheus.NewDesc()
	// 	Handle prometheus.New*Vec() and prometheus.New*() with *Opts{}
	// 	Handle promauto.With(reg).New*() pattern

	return Metric{}, false
}

// writeMetrics writes the metrics in Prometheus text exposition format.
// Label values are empty strings and metric values are 0 since only
// metadata (name, type, help, label names) is used for documentation generation.
func writeMetrics(metrics []Metric, path string) error {
	var buf strings.Builder

	for _, m := range metrics {
		// Write HELP line.
		_, _ = buf.WriteString(fmt.Sprintf("# HELP %s %s\n", m.Name, m.Help))

		// Write TYPE line.
		_, _ = buf.WriteString(fmt.Sprintf("# TYPE %s %s\n", m.Name, m.Type))

		// Write a sample metric line with empty label values and zero metric value.
		if len(m.Labels) > 0 {
			labelPairs := make([]string, len(m.Labels))
			for i, l := range m.Labels {
				labelPairs[i] = fmt.Sprintf("%s=\"\"", l)
			}
			_, _ = buf.WriteString(fmt.Sprintf("%s{%s} 0\n", m.Name, strings.Join(labelPairs, ",")))
		} else {
			_, _ = buf.WriteString(fmt.Sprintf("%s 0\n", m.Name))
		}
	}

	// #nosec G306 - metrics file needs to be readable
	return os.WriteFile(path, []byte(buf.String()), 0o644)
}
