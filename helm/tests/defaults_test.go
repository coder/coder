package tests // nolint: testpackage

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chartutil"
)

// These tests render the chart with the given test case and compares the output
// to the corresponding golden file.
// To update golden files, run `go test . -update-golden-files`.

// UpdateGoldenFiles is a flag that can be set to update golden files.
var UpdateGoldenFiles = flag.Bool("update-golden-files", false, "Update golden files")

var TestCases = []TestCase{
	{
		name: "default_values",
		fn: func(v *Values) {
			v.Coder.Image.Tag = "latest"
		},
	},
	{
		name:      "missing_values",
		fn:        func(v *Values) {},
		renderErr: `You must specify the coder.image.tag value if you're installing the Helm chart directly from Git.`,
	},
	{
		name: "tls",
		fn: func(v *Values) {
			v.Coder.Image.Tag = "latest"
			v.Coder.TLS.SecretNames = []string{"coder-tls"}
		},
	},
}

type TestCase struct {
	name      string                    // Name of the test case. This corresponds to the golden file name.
	fn        func(*Values)             // Function that mutates the values.
	opts      *chartutil.ReleaseOptions // Release options to pass to the renderer.
	caps      *chartutil.Capabilities   // Capabilities to pass to the renderer.
	renderErr string                    // Expected error from rendering the chart.
}

func TestRenderChart(t *testing.T) {
	t.Parallel()
	if *UpdateGoldenFiles {
		t.Skip("Golden files are being updated. Skipping test.")
	}

	for _, tc := range TestCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w, err := LoadChart()
			require.NoError(t, err)
			require.NoError(t, w.Chart.Validate())
			manifests, err := renderManifests(w.Chart, w.OriginalValues, tc.fn, tc.opts, tc.caps)
			if tc.renderErr != "" {
				require.Error(t, err, "render should have failed")
				require.Contains(t, err.Error(), tc.renderErr, "render error should match")
				require.Empty(t, manifests, "manifests should be empty")
			} else {
				require.NoError(t, err, "render should not have failed")
				require.NotEmpty(t, manifests, "manifests should not be empty")
				expected, err := readGoldenFile(tc.name)
				require.NoError(t, err, "failed to load golden file")
				actual := dumpManifests(manifests)
				require.Equal(t, expected, actual)
			}
		})
	}
}

func TestUpdateGoldenFiles(t *testing.T) {
	t.Parallel()
	if !*UpdateGoldenFiles {
		t.Skip("Run with -update-golden-files to update golden files")
	}
	for _, tc := range TestCases {
		w, err := LoadChart()
		require.NoError(t, err, "failed to load chart")
		if tc.renderErr != "" {
			t.Logf("skipping test case %q with render error", tc.name)
			continue
		}
		manifests, err := renderManifests(w.Chart, w.OriginalValues, tc.fn, tc.opts, tc.caps)
		require.NoError(t, err, "failed to render manifests")
		require.NoError(t, writeGoldenFile(tc.name, manifests), "failed to write golden file")
	}
	t.Log("Golden files updated. Please review the changes and commit them.")
	t.Log("This test fails intentionally to prevent accidental updates.")
	t.FailNow()
}

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}
