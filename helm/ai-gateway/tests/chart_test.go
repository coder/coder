package tests // nolint: testpackage

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
)

var updateGoldenFiles = flag.Bool("update", false, "Update golden files")

var testCases = []testCase{
	{
		name:    "default_values",
		fixture: "default_values",
	},
	{
		name:        "networking",
		fixture:     "networking",
		namespace:   "ai-gateway-test",
		apiVersions: []string{"gateway.networking.k8s.io/v1/HTTPRoute"},
	},
	{
		name:      "custom",
		fixture:   "custom",
		namespace: "ai-gateway-test",
	},
	{
		name:    "nodeport",
		fixture: "nodeport",
	},
	{
		name:          "missing_key_field",
		fixture:       "fails_missing_key_field",
		expectedError: "aigateway.keySecret.key is required when name is set.",
	},
	{
		name:          "partial_listener_tls",
		fixture:       "fails_partial_listener_tls",
		expectedError: "aigateway.listenerTLS.certKey and keyKey are required when name is set.",
	},
	// This verifies that listener TLS and Ingress can be rendered together.
	// Production use requires controller-specific backend TLS and certificate
	// trust configuration outside this chart.
	{
		name:    "listener_tls_with_ingress",
		fixture: "listener_tls_with_ingress",
	},
	{
		name:          "partial_client_tls",
		fixture:       "fails_partial_client_tls",
		expectedError: "aigateway.coderTLS.clientSecret.certKey and keyKey are required when name is set.",
	},
	{
		name:          "partial_ca_tls",
		fixture:       "fails_partial_ca_tls",
		expectedError: "aigateway.coderTLS.caSecret.key is required when name is set.",
	},
	{
		name:          "ingress_without_service",
		fixture:       "fails_ingress_without_service",
		expectedError: "service.enable must be true when ingress.enable is true.",
	},
	{
		name:          "ingress_without_host",
		fixture:       "fails_ingress_without_host",
		expectedError: "ingress.host is required when ingress.enable is true.",
	},
	{
		name:          "httproute_without_service",
		fixture:       "fails_httproute_without_service",
		expectedError: "service.enable must be true when httproute.enable is true.",
	},
	{
		name:          "httproute_without_parent_refs",
		fixture:       "fails_httproute_without_parent_refs",
		expectedError: "httproute.parentRefs is required when httproute.enable is true.",
		apiVersions:   []string{"gateway.networking.k8s.io/v1/HTTPRoute"},
	},
	{
		name:          "httproute_without_crd",
		fixture:       "networking",
		expectedError: "httproute.enable requires the gateway.networking.k8s.io/v1 HTTPRoute CRD.",
	},
	{
		name:          "nodeport_with_clusterip",
		fixture:       "fails_nodeport_with_clusterip",
		expectedError: "service.nodePort requires service.type to be NodePort or LoadBalancer.",
	},
}

type testCase struct {
	name          string
	fixture       string
	namespace     string
	expectedError string
	apiVersions   []string
}

func (tc testCase) valuesFilePath() string {
	return filepath.Join("testdata", tc.fixture+".yaml")
}

func (tc testCase) goldenFilePath() string {
	if tc.namespace == "default" {
		return filepath.Join("testdata", tc.name+".golden")
	}
	return filepath.Join("testdata", tc.name+"_"+tc.namespace+".golden")
}

func TestRenderChart(t *testing.T) {
	t.Parallel()
	if *updateGoldenFiles {
		t.Skip("Golden files are being updated")
	}
	if testutil.InCI() && (runtime.GOOS == "windows" || runtime.GOOS == "darwin") {
		t.Skip("Skipping Helm tests on Windows and macOS in CI")
	}

	helmPath := lookupHelm(t)
	err := updateHelmDependencies(t, helmPath, "..")
	require.NoError(t, err, "failed to build Helm dependencies")
	for _, tc := range testCases {
		tc := tc
		if tc.namespace == "" {
			tc.namespace = "default"
		}
		t.Run(tc.namespace+"/"+tc.name, func(t *testing.T) {
			t.Parallel()
			output, err := runHelmTemplate(t, helmPath, tc.valuesFilePath(), tc.namespace, tc.apiVersions)
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, output, tc.expectedError)
				return
			}
			require.NoError(t, err, output)
			golden, err := os.ReadFile(tc.goldenFilePath())
			require.NoError(t, err)
			golden = bytes.ReplaceAll(golden, []byte("\r"), nil)
			require.Equal(t, string(golden), output)
		})
	}
}

func TestUpdateGoldenFiles(t *testing.T) {
	t.Parallel()
	if !*updateGoldenFiles {
		t.Skip("Run with -update to update golden files")
	}

	helmPath := lookupHelm(t)
	err := updateHelmDependencies(t, helmPath, "..")
	require.NoError(t, err, "failed to build Helm dependencies")
	for _, tc := range testCases {
		if tc.expectedError != "" {
			continue
		}
		if tc.namespace == "" {
			tc.namespace = "default"
		}
		output, err := runHelmTemplate(t, helmPath, tc.valuesFilePath(), tc.namespace, tc.apiVersions)
		require.NoError(t, err, output)
		require.NoError(t, os.WriteFile(tc.goldenFilePath(), []byte(output), 0o644)) // nolint:gosec
	}
}

func runHelmTemplate(t testing.TB, helmPath, valuesFile, namespace string, apiVersions []string) (string, error) {
	t.Helper()
	args := []string{"template", "ai-gateway", "..", "-f", valuesFile, "--namespace", namespace}
	for _, apiVersion := range apiVersions {
		args = append(args, "--api-versions", apiVersion)
	}
	cmd := exec.Command(helmPath, args...)
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// updateDepsOnce guards updateHelmDependencies: parallel top-level tests
// share the charts/ directory, and rebuilding it while another test
// templates the chart races.
var (
	updateDepsOnce sync.Once
	errUpdateDeps  error
)

// updateHelmDependencies runs `helm dependency update .` on the given chartDir.
func updateHelmDependencies(t testing.TB, helmPath, chartDir string) error {
	t.Helper()
	updateDepsOnce.Do(func() {
		// Remove charts/ from chartDir if it exists.
		err := os.RemoveAll(filepath.Join(chartDir, "charts"))
		if err != nil {
			errUpdateDeps = xerrors.Errorf("failed to remove charts/ directory: %w", err)
			return
		}

		cmd := exec.Command(helmPath, "dependency", "update", "--skip-refresh", ".")
		cmd.Dir = chartDir
		t.Logf("exec command: %v", cmd.Args)
		out, err := cmd.CombinedOutput()
		if err != nil {
			errUpdateDeps = xerrors.Errorf("failed to run `helm dependency update`: %w\noutput: %s", err, out)
			return
		}
	})
	return errUpdateDeps
}

func lookupHelm(t testing.TB) string {
	t.Helper()
	helmPath, err := exec.LookPath("helm")
	require.NoError(t, err, "helm not found in PATH")
	return helmPath
}

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}
