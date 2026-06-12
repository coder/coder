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

	t.Run("UnknownBase", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.RenderBaseTemplate("nonexistent", "main.tf.tmpl", templatebuilder.BaseRenderContext{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown base template")
	})

	t.Run("InvalidPath", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.RenderBaseTemplate("docker", "nonexistent.tf.tmpl", templatebuilder.BaseRenderContext{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	imageOpts := []templatebuilder.ImageOption{
		{Name: "Ubuntu", Value: "codercom/enterprise-base:ubuntu"},
		{Name: "Custom", Value: "custom/image:latest"},
	}

	t.Run("DockerWithImageOptions", func(t *testing.T) {
		t.Parallel()

		renderCtx := templatebuilder.BaseRenderContext{
			ContainerImage: "custom/image:latest",
			ImageOptions:   imageOpts,
		}
		out, err := templatebuilder.RenderBaseTemplate("docker", "main.tf.tmpl", renderCtx)
		require.NoError(t, err)
		rendered := string(out)
		require.Contains(t, rendered, `data.coder_parameter.container_image.value`)
		require.Contains(t, rendered, `name  = "Ubuntu"`)
		require.Contains(t, rendered, `name  = "Custom"`)
		require.Contains(t, rendered, `coder_parameter`)
	})

	t.Run("KubernetesWithImageOptions", func(t *testing.T) {
		t.Parallel()

		renderCtx := templatebuilder.BaseRenderContext{
			ContainerImage: "custom/image:latest",
			ImageOptions:   imageOpts,
		}
		out, err := templatebuilder.RenderBaseTemplate("kubernetes", "main.tf.tmpl", renderCtx)
		require.NoError(t, err)
		rendered := string(out)
		require.Contains(t, rendered, `data.coder_parameter.container_image.value`)
		require.Contains(t, rendered, `name  = "Ubuntu"`)
		require.Contains(t, rendered, `coder_parameter`)
	})
}

func TestBaseTemplateSnapshot(t *testing.T) {
	t.Parallel()

	// This test table must cover every known base template.
	// BaseTemplateIDs() is the source of truth; this list must match.
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

			renderCtx := templatebuilder.DefaultBaseRenderContext(tc.exampleID)
			rendered, err := templatebuilder.RenderBaseTemplate(tc.exampleID, "main.tf.tmpl", renderCtx)
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
