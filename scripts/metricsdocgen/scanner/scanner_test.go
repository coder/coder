package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

//nolint:paralleltest // Prefix constants are resolved relative to the process working directory.
func TestMetricPrefixForPath(t *testing.T) {
	t.Chdir("../../..")

	testCases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "common metric",
			path: "aibridge/metrics/metrics.go",
			want: "coder_ai_gateway_",
		},
		{
			name: "metric prefixed by another package",
			path: "coderd/aibridgedserver/metrics.go",
			want: "coder_ai_gateway_",
		},
		{
			name: "absolute proxy metric",
			path: "/repo/enterprise/aibridgeproxyd/metrics.go",
			want: "coder_ai_gateway_proxy_",
		},
		{
			name: "ordinary metric",
			path: "coderd/metrics.go",
			want: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			prefix, err := metricPrefixForPath(testCase.path)
			require.NoError(t, err)
			require.Equal(t, testCase.want, prefix)
		})
	}
}

//nolint:paralleltest // Prefix constants are resolved relative to the process working directory.
func TestScanFileAppliesCanonicalPrefixAndAppendLabels(t *testing.T) {
	// The metric options come from the fixture below, but the prefix comes from
	// the real `aibridge/metrics/metrics.go`, because metricPrefixForPath
	// suffix-matches the fixture path and reads the constant from the repository.
	source := `package metrics
import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)
var baseLabels = []string{"provider", "model"}
func newMetrics(reg prometheus.Registerer) {
	promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Subsystem: "interceptions",
		Name: "total",
		Help: "Requests.",
	}, append(baseLabels, "status", "route"))
}`

	path := writeSourceFile(t, "aibridge/metrics/metrics.go", source)
	t.Chdir("../../..")

	metrics, err := scanFile(path)
	require.NoError(t, err)
	require.Equal(t, []Metric{{
		Name:   "coder_ai_gateway_interceptions_total",
		Type:   MetricTypeCounter,
		Help:   "Requests.",
		Labels: []string{"provider", "model", "status", "route"},
	}}, metrics)
}

func TestExtractLabelsAppendDoesNotMutateBaseLabels(t *testing.T) {
	t.Parallel()

	// The base slice has three elements so that extractStringSlice leaves spare
	// capacity. Without the defensive copy in the ident branch, the second
	// append writes over the first result's backing array.
	file, err := parser.ParseFile(token.NewFileSet(), "metrics.go", `package metrics
var baseLabels = []string{"provider", "model", "route"}
var first = append(baseLabels, "status")
var second = append(baseLabels, "outcome")
`, parser.SkipObjectResolution)
	require.NoError(t, err)
	decls := collectDecls(file)
	require.Greater(t, cap(decls.stringSlices["baseLabels"]), len(decls.stringSlices["baseLabels"]))

	firstLabels := extractLabels(findValueExpr(t, file, "first"), decls)
	secondLabels := extractLabels(findValueExpr(t, file, "second"), decls)
	require.Equal(t, []string{"provider", "model", "route", "status"}, firstLabels)
	require.Equal(t, []string{"provider", "model", "route", "outcome"}, secondLabels)
	require.Equal(t, []string{"provider", "model", "route"}, decls.stringSlices["baseLabels"])
}

func TestValidateMetricNamespaces(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateMetricNamespaces([]Metric{
		{Name: "coder_ai_gateway_interceptions_total"},
		{Name: "coderd_agents_up"},
		{Name: "agent_boundary_log_proxy_logs_dropped_total"},
	}))

	// A metric defined in a package whose registerer applies the prefix, but with
	// no metricPrefixSources entry, reaches this check as a bare local name.
	err := validateMetricNamespaces([]Metric{
		{Name: "coder_ai_gateway_interceptions_total"},
		{Name: "outside_counter_total"},
	})
	require.ErrorContains(t, err, "outside_counter_total")
	require.ErrorContains(t, err, "metricPrefixSources")
}

//nolint:paralleltest // The scanner resolves paths relative to the process working directory.
func TestScanAIGatewayFilesAreMapped(t *testing.T) {
	t.Chdir("../../..")

	// Metrics defined in these trees are registered through a prefixed
	// registerer, so every file that defines metrics must appear in
	// metricPrefixSources. A missing entry emits an unprefixed name that no
	// documentation section matches.
	aiGatewayDirs := []string{
		"aibridge",
		"coderd/aibridged",
		"coderd/aibridgedserver",
		"enterprise/aibridgeproxyd",
	}
	for _, dir := range aiGatewayDirs {
		require.NoError(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			metrics, err := scanFile(path)
			require.NoError(t, err)
			if len(metrics) > 0 {
				prefix, err := metricPrefixForPath(path)
				require.NoError(t, err)
				require.NotEmpty(t, prefix,
					"%s defines metrics but has no metricPrefixSources entry", path)
			}
			return nil
		}))
	}
}

//nolint:paralleltest // The scanner resolves paths relative to the process working directory.
func TestScanAllDirsCanonicalAIGatewaySets(t *testing.T) {
	t.Chdir("../../..")

	metrics, err := scanAllDirs()
	require.NoError(t, err)
	prepared := prepareMetrics(metrics)
	require.NoError(t, validateMetricNamespaces(prepared))
	require.True(t, slices.IsSortedFunc(prepared, func(a, b Metric) int {
		return strings.Compare(a.Name, b.Name)
	}))

	var common, costControl, proxy []string
	for _, metric := range prepared {
		switch {
		case strings.HasPrefix(metric.Name, "coder_ai_gateway_proxy_"):
			proxy = append(proxy, metric.Name)
		case strings.HasPrefix(metric.Name, "coder_ai_gateway_cost_control_"):
			costControl = append(costControl, metric.Name)
		case strings.HasPrefix(metric.Name, "coder_ai_gateway_"):
			common = append(common, metric.Name)
		}
	}

	require.Equal(t, []string{
		"coder_ai_gateway_circuit_breaker_rejects_total",
		"coder_ai_gateway_circuit_breaker_state",
		"coder_ai_gateway_circuit_breaker_trips_total",
		"coder_ai_gateway_injected_tool_invocations_total",
		"coder_ai_gateway_interceptions_duration_seconds",
		"coder_ai_gateway_interceptions_inflight",
		"coder_ai_gateway_interceptions_total",
		"coder_ai_gateway_key_pool_exhaustions_total",
		"coder_ai_gateway_key_pool_failover_attempts",
		"coder_ai_gateway_key_pool_state",
		"coder_ai_gateway_key_pool_state_transitions_total",
		"coder_ai_gateway_non_injected_tool_selections_total",
		"coder_ai_gateway_passthrough_total",
		"coder_ai_gateway_prompts_total",
		"coder_ai_gateway_provider_info",
		"coder_ai_gateway_providers_last_reload_success_timestamp_seconds",
		"coder_ai_gateway_providers_last_reload_timestamp_seconds",
		"coder_ai_gateway_tokens_total",
	}, common)
	require.Equal(t, []string{
		"coder_ai_gateway_cost_control_blocked_requests_total",
		"coder_ai_gateway_cost_control_blocked_users",
		"coder_ai_gateway_cost_control_enforcement_duration_seconds",
		"coder_ai_gateway_cost_control_unpriced_token_usage_records_total",
	}, costControl)
	require.Equal(t, []string{
		"coder_ai_gateway_proxy_connect_sessions_total",
		"coder_ai_gateway_proxy_inflight_mitm_requests",
		"coder_ai_gateway_proxy_mitm_requests_total",
		"coder_ai_gateway_proxy_mitm_responses_total",
		"coder_ai_gateway_proxy_provider_info",
		"coder_ai_gateway_proxy_providers_last_reload_success_timestamp_seconds",
		"coder_ai_gateway_proxy_providers_last_reload_timestamp_seconds",
	}, proxy)
}

func writeSourceFile(t *testing.T, relativePath, source string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), relativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
	return path
}

func findValueExpr(t *testing.T, file *ast.File, name string) ast.Expr {
	t.Helper()

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
			for i, ident := range valueSpec.Names {
				if ident.Name == name && i < len(valueSpec.Values) {
					return valueSpec.Values[i]
				}
			}
		}
	}
	t.Fatalf("value %q not found", name)
	return nil
}
