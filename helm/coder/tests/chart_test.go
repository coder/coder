package tests // nolint: testpackage
import (
	"fmt"
	"errors"
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"github.com/stretchr/testify/require"
	"github.com/coder/coder/v2/testutil"
)
// These tests run `helm template` with the values file specified in each test
// and compare the output to the contents of the corresponding golden file.
// All values and golden files are located in the `testdata` directory.
// To update golden files, run `go test . -update`.
// updateGoldenFiles is a flag that can be set to update golden files.
var updateGoldenFiles = flag.Bool("update", false, "Update golden files")
var namespaces = []string{
	"default",
	"coder",
}
var testCases = []testCase{
	{
		name:          "default_values",
		expectedError: "",
	},
	{
		name:          "missing_values",
		expectedError: `You must specify the coder.image.tag value if you're installing the Helm chart directly from Git.`,
	},
	{
		name:          "tls",
		expectedError: "",
	},
	{
		name:          "sa",
		expectedError: "",
	},
	{
		name:          "labels_annotations",
		expectedError: "",
	},
	{
		name:          "workspace_proxy",
		expectedError: "",
	},
	{
		name:          "command",
		expectedError: "",
	},
	{
		name:          "command_args",
		expectedError: "",
	},
	{
		name:          "provisionerd_psk",
		expectedError: "",
	},
	{
		name:          "auto_access_url_1",
		expectedError: "",
	},
	{
		name:          "auto_access_url_2",
		expectedError: "",
	},
	{
		name:          "auto_access_url_3",
		expectedError: "",
	},
	{
		name:          "env_from",
		expectedError: "",
	},
	{
		name:          "extra_templates",
		expectedError: "",
	},
	{
		name:          "prometheus",
		expectedError: "",
	},
	{
		name:          "sa_extra_rules",
		expectedError: "",
	},
	{
		name:          "sa_disabled",
		expectedError: "",
	},
	{
		name:          "topology",
		expectedError: "",
	},
	{
		name:          "svc_loadbalancer_class",
		expectedError: "",
	},
	{
		name:          "svc_nodeport",
		expectedError: "",
	},
	{
		name:          "svc_loadbalancer",
		expectedError: "",
	},
	{
		name:          "securitycontext",
		expectedError: "",
	},
}
type testCase struct {
	name          string // Name of the test case. This is used to control which values and golden file are used.
	namespace     string // Namespace is the name of the namespace the resources should be generated within
	expectedError string // Expected error from running `helm template`.
}
func (tc testCase) valuesFilePath() string {
	return filepath.Join("./testdata", tc.name+".yaml")
}
func (tc testCase) goldenFilePath() string {
	if tc.namespace == "default" {
		return filepath.Join("./testdata", tc.name+".golden")
	}
	return filepath.Join("./testdata", tc.name+"_"+tc.namespace+".golden")
}
func TestRenderChart(t *testing.T) {
	t.Parallel()
	if *updateGoldenFiles {
		t.Skip("Golden files are being updated. Skipping test.")
	}
	if testutil.InCI() {
		switch runtime.GOOS {
		case "windows", "darwin":
			t.Skip("Skipping tests on Windows and macOS in CI")
		}
	}
	// Ensure that Helm is available in $PATH
	helmPath := lookupHelm(t)
	err := updateHelmDependencies(t, helmPath, "..")
	require.NoError(t, err, "failed to build Helm dependencies")
	for _, tc := range testCases {
		tc := tc
		for _, ns := range namespaces {
			tc := tc
			tc.namespace = ns
			t.Run(tc.namespace+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				// Ensure that the values file exists.
				valuesFilePath := tc.valuesFilePath()
				if _, err := os.Stat(valuesFilePath); os.IsNotExist(err) {
					t.Fatalf("values file %q does not exist", valuesFilePath)
				}
				// Run helm template with the values file.
				templateOutput, err := runHelmTemplate(t, helmPath, "..", valuesFilePath, tc.namespace)
				if tc.expectedError != "" {
					require.Error(t, err, "helm template should have failed")
					require.Contains(t, templateOutput, tc.expectedError, "helm template output should contain expected error")
				} else {
					require.NoError(t, err, "helm template should not have failed")
					require.NotEmpty(t, templateOutput, "helm template output should not be empty")
					goldenFilePath := tc.goldenFilePath()
					goldenBytes, err := os.ReadFile(goldenFilePath)
					require.NoError(t, err, "failed to read golden file %q", goldenFilePath)
					// Remove carriage returns to make tests pass on Windows.
					goldenBytes = bytes.ReplaceAll(goldenBytes, []byte("\r"), []byte(""))
					expected := string(goldenBytes)
					require.NoError(t, err, "failed to load golden file %q")
					require.Equal(t, expected, templateOutput)
				}
			})
		}
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
		tc := tc
		if tc.expectedError != "" {
			t.Logf("skipping test case %q with render error", tc.name)
			continue
		}
		for _, ns := range namespaces {
			tc := tc
			tc.namespace = ns
			valuesPath := tc.valuesFilePath()
			templateOutput, err := runHelmTemplate(t, helmPath, "..", valuesPath, tc.namespace)
			if err != nil {
				t.Logf("error running `helm template -f %q`: %v", valuesPath, err)
				t.Logf("output: %s", templateOutput)
			}
			require.NoError(t, err, "failed to run `helm template -f %q`", valuesPath)
			goldenFilePath := tc.goldenFilePath()
			err = os.WriteFile(goldenFilePath, []byte(templateOutput), 0o644) // nolint:gosec
			require.NoError(t, err, "failed to write golden file %q", goldenFilePath)
		}
	}
	t.Log("Golden files updated. Please review the changes and commit them.")
}
// updateHelmDependencies runs `helm dependency update .` on the given chartDir.
func updateHelmDependencies(t testing.TB, helmPath, chartDir string) error {
	// Remove charts/ from chartDir if it exists.
	err := os.RemoveAll(filepath.Join(chartDir, "charts"))
	if err != nil {
		return fmt.Errorf("failed to remove charts/ directory: %w", err)
	}
	// Regenerate the chart dependencies.
	cmd := exec.Command(helmPath, "dependency", "update", "--skip-refresh", ".")
	cmd.Dir = chartDir
	t.Logf("exec command: %v", cmd.Args)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run `helm dependency build`: %w\noutput: %s", err, out)
	}
	return nil
}
// runHelmTemplate runs helm template on the given chart with the given values and
// returns the raw output.
func runHelmTemplate(t testing.TB, helmPath, chartDir, valuesFilePath, namespace string) (string, error) {
	// Ensure that valuesFilePath exists
	if _, err := os.Stat(valuesFilePath); err != nil {
		return "", fmt.Errorf("values file %q does not exist: %w", valuesFilePath, err)
	}
	cmd := exec.Command(helmPath, "template", chartDir, "-f", valuesFilePath, "--namespace", namespace)
	t.Logf("exec command: %v", cmd.Args)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
// lookupHelm ensures that Helm is available in $PATH and returns the path to the
// Helm executable.
func lookupHelm(t testing.TB) string {
	helmPath, err := exec.LookPath("helm")
	if err != nil {
		t.Fatalf("helm not found in $PATH: %v", err)
		return ""
	}
	t.Logf("Using helm at %q", helmPath)
	return helmPath
}
func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}
