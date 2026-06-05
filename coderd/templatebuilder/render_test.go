package templatebuilder_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/templatebuilder"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func TestRenderBaseTemplate(t *testing.T) {
	t.Parallel()

	t.Run("InvalidPath", func(t *testing.T) {
		t.Parallel()
		fsys, err := templatebuilder.BaseTemplateFS("docker")
		require.NoError(t, err)

		_, err = templatebuilder.RenderBaseTemplate(fsys, "nonexistent.tf.tmpl", templatebuilder.BaseRenderContext{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "read template")
	})

	t.Run("WithImageOptions", func(t *testing.T) {
		t.Parallel()
		fsys, err := templatebuilder.BaseTemplateFS("docker")
		require.NoError(t, err)

		ctx := templatebuilder.BaseRenderContext{
			ContainerImage: "custom/image:latest",
			ImageOptions: []templatebuilder.ImageOption{
				{Name: "Ubuntu", Value: "codercom/enterprise-base:ubuntu"},
				{Name: "Custom", Value: "custom/image:latest"},
			},
		}
		out, err := templatebuilder.RenderBaseTemplate(fsys, "main.tf.tmpl", ctx)
		require.NoError(t, err)
		require.Contains(t, string(out), `image = "custom/image:latest"`)
		require.Contains(t, string(out), `name  = "Ubuntu"`)
		require.Contains(t, string(out), `name  = "Custom"`)
		require.Contains(t, string(out), `coder_parameter`)
	})
}

func TestBaseTemplateSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		exampleID string
	}{
		{exampleID: "docker"},
		{exampleID: "kubernetes"},
		{exampleID: "aws-linux"},
	}

	for _, tc := range tests {
		t.Run(tc.exampleID, func(t *testing.T) {
			t.Parallel()

			fsys, err := templatebuilder.BaseTemplateFS(tc.exampleID)
			require.NoError(t, err)

			ctx := templatebuilder.DefaultBaseRenderContext(tc.exampleID)
			rendered, err := templatebuilder.RenderBaseTemplate(fsys, "main.tf.tmpl", ctx)
			require.NoError(t, err)
			require.NotEmpty(t, rendered)

			goldenPath := filepath.Join("testdata", tc.exampleID+".tf.golden")

			if *updateGolden {
				err := os.MkdirAll("testdata", 0o755)
				require.NoError(t, err)
				err = os.WriteFile(goldenPath, rendered, 0o600)
				require.NoError(t, err)
				return
			}

			expected, err := os.ReadFile(goldenPath)
			require.NoError(t, err, "golden file %s not found; run with -update to create", goldenPath)
			require.Equal(t, string(expected), string(rendered),
				"rendered output for %s does not match golden file; run with -update to regenerate", tc.exampleID)
		})
	}
}
