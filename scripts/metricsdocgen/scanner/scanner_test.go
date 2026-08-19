package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricPrefixForPath(t *testing.T) {
	t.Parallel()

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
			t.Parallel()
			require.Equal(t, testCase.want, metricPrefixForPath(testCase.path))
		})
	}
}

func TestScanFileAppliesCanonicalPrefixAndAppendLabels(t *testing.T) {
	t.Parallel()

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

	file, err := parser.ParseFile(token.NewFileSet(), "metrics.go", `package metrics
var baseLabels = []string{"provider", "model"}
var first = append(baseLabels, "status")
var second = append(baseLabels, "route")
`, parser.SkipObjectResolution)
	require.NoError(t, err)
	decls := collectDecls(file)

	first := findValueExpr(t, file, "first")
	second := findValueExpr(t, file, "second")
	require.Equal(t, []string{"provider", "model", "status"}, extractLabels(first, decls))
	require.Equal(t, []string{"provider", "model", "route"}, extractLabels(second, decls))
	require.Equal(t, []string{"provider", "model"}, decls.stringSlices["baseLabels"])
}

//nolint:paralleltest // The scanner resolves paths relative to the process working directory.
func TestScanAllDirsCanonicalAIGatewaySets(t *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(filepath.Join(workingDirectory, "../../..")))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(workingDirectory))
	})

	metrics, err := scanAllDirs()
	require.NoError(t, err)
	prepared := prepareMetrics(metrics)
	require.True(t, slices.IsSortedFunc(prepared, func(a, b Metric) int {
		return strings.Compare(a.Name, b.Name)
	}))

	common := make([]string, 0, 18)
	costControl := make([]string, 0, 4)
	proxy := make([]string, 0, 7)
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
