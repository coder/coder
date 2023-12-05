package tests // nolint: testpackage

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
)

// These tests run `helm template` with the values file specified in each test
// and compare the output to the contents of the corresponding golden file.
// All values and golden files are located in the `testdata` directory.
// To update golden files, run `go test . -update`.

// updateGoldenFiles is a flag that can be set to update golden files.
var updateGoldenFiles = flag.Bool("update", false, "Update golden files")

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
		name:          "sa",
		expectedError: "",
	},
	{
		name:          "labels_annotations",
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
		name:          "extra_templates",
		expectedError: "",
	},
}

type testCase struct {
	name          string // Name of the test case. This is used to control which values and golden file are used.
	expectedError string // Expected error from running `helm template`.
}

func (tc testCase) valuesFilePath() string {
	return filepath.Join("./testdata", tc.name+".yaml")
}

func (tc testCase) goldenFilePath() string {
	return filepath.Join("./testdata", tc.name+".golden")
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Ensure that the values file exists.
			valuesFilePath := tc.valuesFilePath()
			if _, err := os.Stat(valuesFilePath); os.IsNotExist(err) {
				t.Fatalf("values file %q does not exist", valuesFilePath)
			}

			// Run helm template with the values file.
			templateOutput, err := runHelmTemplate(t, helmPath, "..", valuesFilePath)
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
				goldenBytes = bytes.Replace(goldenBytes, []byte("\r"), []byte(""), -1)
				expected := string(goldenBytes)

				require.NoError(t, err, "failed to load golden file %q")
				require.Equal(t, expected, templateOutput)
			}
		})
	}
}

func TestUpdateGoldenFiles(t *testing.T) {
	t.Parallel()
	if !*updateGoldenFiles {
		t.Skip("Run with -update to update golden files")
	}

	helmPath := lookupHelm(t)
	for _, tc := range testCases {
		if tc.expectedError != "" {
			t.Logf("skipping test case %q with render error", tc.name)
			continue
		}

		valuesPath := tc.valuesFilePath()
		templateOutput, err := runHelmTemplate(t, helmPath, "..", valuesPath)

		require.NoError(t, err, "failed to run `helm template -f %q`", valuesPath)

		goldenFilePath := tc.goldenFilePath()
		err = os.WriteFile(goldenFilePath, []byte(templateOutput), 0o644) // nolint:gosec
		require.NoError(t, err, "failed to write golden file %q", goldenFilePath)
	}
	t.Log("Golden files updated. Please review the changes and commit them.")
}

// updateHelmDependencies runs `helm dependency update .` on the given chartDir.
func updateHelmDependencies(t testing.TB, helmPath, chartDir string) error {
	// Remove charts/ from chartDir if it exists.
	err := os.RemoveAll(filepath.Join(chartDir, "charts"))
	if err != nil {
		return xerrors.Errorf("failed to remove charts/ directory: %w", err)
	}

	// Regenerate the chart dependencies.
	cmd := exec.Command(helmPath, "dependency", "update", "--skip-refresh", ".")
	cmd.Dir = chartDir
	t.Logf("exec command: %v", cmd.Args)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return xerrors.Errorf("failed to run `helm dependency build`: %w\noutput: %s", err, out)
	}

	return nil
}

// runHelmTemplate runs helm template on the given chart with the given values and
// returns the raw output.
func runHelmTemplate(t testing.TB, helmPath, chartDir, valuesFilePath string) (string, error) {
	// Ensure that valuesFilePath exists
	if _, err := os.Stat(valuesFilePath); err != nil {
		return "", xerrors.Errorf("values file %q does not exist: %w", valuesFilePath, err)
	}

	cmd := exec.Command(helmPath, "template", chartDir, "-f", valuesFilePath, "--namespace", "default")
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
