package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// writeGoFile writes src to dir/name under root and returns the file path.
func writeGoFile(t *testing.T, root, dir, name, src string) string {
	t.Helper()

	full := filepath.Join(root, dir)
	if err := os.MkdirAll(full, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	path := filepath.Join(full, name)
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// inRoot runs fn with the working directory set to root, because the scanner
// resolves package directories relative to the repository root.
func inRoot(t *testing.T, root string, fn func()) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir %s: %v", root, err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	}()
	fn()
}

//nolint:paralleltest // Changes the working directory, so it cannot run in parallel.
func TestBuildPrefixIndex(t *testing.T) {
	root := t.TempDir()

	// A command package that wraps the registry and hands it to two
	// constructors, one through a literal prefix and one through an imported
	// constant.
	writeGoFile(t, root, "cli", "server.go", `package cli

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/server/costcontrol"
	gwmetrics "github.com/coder/coder/v2/gateway/metrics"
	"github.com/coder/coder/v2/gateway/keypool"
)

func run(registry *prometheus.Registry) {
	costReg := prometheus.WrapRegistererWithPrefix("coder_ai_gateway_", registry)
	costcontrol.NewMetrics(costReg)

	gatewayReg := prometheus.WrapRegistererWithPrefix(gwmetrics.PrometheusMetricPrefix, registry)
	gwmetrics.NewMetrics(gatewayReg)
	gatewayReg.MustRegister(keypool.NewStateCollector(nil))

	aliasReg := prometheusmetrics.NewMetricAliasRegisterer(registry, "coder_ai_gateway_proxy_", "coder_legacy_")
	proxy.NewMetrics(aliasReg)
}
`)
	writeGoFile(t, root, "gateway/metrics", "metrics.go", `package metrics

const PrometheusMetricPrefix = "coder_ai_gateway_"
`)
	writeGoFile(t, root, "server/costcontrol", "metrics.go", "package costcontrol\n")
	writeGoFile(t, root, "gateway/keypool", "collector.go", "package keypool\n")

	var index prefixIndex
	inRoot(t, root, func() {
		var err error
		index, err = buildPrefixIndex([]string{"cli", "gateway", "server"})
		if err != nil {
			t.Fatalf("buildPrefixIndex: %v", err)
		}
	})

	cases := []struct {
		dir  string
		want []string
	}{
		// Literal prefix passed to a constructor.
		{"server/costcontrol", []string{"coder_ai_gateway_"}},
		// Prefix resolved through an imported constant.
		{"gateway/metrics", []string{"coder_ai_gateway_"}},
		// Collector registered against the wrapped registerer.
		{"gateway/keypool", []string{"coder_ai_gateway_"}},
	}
	for _, tc := range cases {
		if got := index[tc.dir]; !slices.Equal(got, tc.want) {
			t.Errorf("index[%q] = %v, want %v", tc.dir, got, tc.want)
		}
	}
}

// TestBuildPrefixIndexFollowsForwarders covers a constructor that passes its
// registerer to another package, which is how the gateway packages are
// wired: the prefix must reach the package that declares the metrics.
//
//nolint:paralleltest // Changes the working directory, so it cannot run in parallel.
func TestBuildPrefixIndexFollowsForwarders(t *testing.T) {
	root := t.TempDir()

	writeGoFile(t, root, "cli", "server.go", `package cli

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/bridge"
)

func run(registry *prometheus.Registry) {
	reg := prometheus.WrapRegistererWithPrefix("coder_ai_gateway_", registry)
	bridge.NewMetrics(reg)
}
`)
	writeGoFile(t, root, "bridge", "api.go", `package bridge

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/bridge/metrics"
	"github.com/coder/coder/v2/bridge/other"
	"github.com/coder/coder/v2/db"
)

func NewMetrics(reg prometheus.Registerer, store db.Store) *metrics.Metrics {
	other.NewMetrics(store)
	return metrics.NewMetrics(reg)
}
`)
	writeGoFile(t, root, "bridge/metrics", "metrics.go", "package metrics\n")
	writeGoFile(t, root, "bridge/other", "metrics.go", "package other\n")

	var index prefixIndex
	inRoot(t, root, func() {
		var err error
		index, err = buildPrefixIndex([]string{"cli", "bridge"})
		if err != nil {
			t.Fatalf("buildPrefixIndex: %v", err)
		}
	})

	want := []string{"coder_ai_gateway_"}
	if got := index["bridge/metrics"]; !slices.Equal(got, want) {
		t.Errorf("index[\"bridge/metrics\"] = %v, want %v (prefix did not follow the forwarder)", got, want)
	}
	if got := index["bridge/other"]; len(got) != 0 {
		t.Errorf("index[\"bridge/other\"] = %v, want no prefix for a forwarded non-registerer", got)
	}
}

func TestApplyPrefixes(t *testing.T) {
	t.Parallel()

	metric := Metric{Name: "interceptions_total", Help: "help"}

	if got := applyPrefixes(metric, nil); len(got) != 1 || got[0].Name != "interceptions_total" {
		t.Errorf("applyPrefixes with no prefix = %v, want the metric unchanged", got)
	}

	got := applyPrefixes(metric, []string{"coder_ai_gateway_", "coder_legacy_"})
	want := []string{"coder_ai_gateway_interceptions_total", "coder_legacy_interceptions_total"}
	if len(got) != len(want) {
		t.Fatalf("applyPrefixes returned %d metrics, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Errorf("applyPrefixes()[%d].Name = %q, want %q", i, got[i].Name, want[i])
		}
	}
}

func TestExtractAppendedLabels(t *testing.T) {
	t.Parallel()

	src := `package metrics

var baseLabels = []string{"provider", "model"}
const extra = "route"
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "metrics.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	decls := collectDecls(file)

	cases := []struct {
		name string
		expr string
		want []string
	}{
		{"literal extras", `append(baseLabels, "status", "route")`, []string{"provider", "model", "status", "route"}},
		{"constant extra", `append(baseLabels, extra)`, []string{"provider", "model", "route"}},
		{"no extras", `append(baseLabels)`, []string{"provider", "model"}},
		{"unknown base", `append(otherLabels, "status")`, nil},
		{"unresolvable extra", `append(baseLabels, someVar)`, nil},
		{"not append", `join(baseLabels, "status")`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("parse expr: %v", err)
			}
			call, ok := expr.(*ast.CallExpr)
			if !ok {
				t.Fatalf("%q is not a call expression", tc.expr)
			}
			if got := extractAppendedLabels(call, decls); !slices.Equal(got, tc.want) {
				t.Errorf("extractAppendedLabels(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestExcluded(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"enterprise/scaletest/agentfake/metrics.go": true,
		"enterprise/scaletest":                      true,
		"enterprise/scaletestextra/metrics.go":      false,
		"coderd/prometheusmetrics/metrics.go":       false,
	}
	for path, want := range cases {
		if got := excluded(path); got != want {
			t.Errorf("excluded(%q) = %v, want %v", path, got, want)
		}
	}
}
