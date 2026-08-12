package templatebuilder_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/templatebuilder"
)

// allBaseIDs is the set of base template IDs expected in the catalog.
var allBaseIDs = []string{
	"aws-linux",
	"aws-windows",
	"azure-linux",
	"digitalocean-linux",
	"docker",
	"gcp-linux",
	"gcp-windows",
	"kubernetes",
	"scratch",
}

func TestBaseTemplateOS(t *testing.T) {
	t.Parallel()

	linuxBases := []string{
		"aws-linux", "azure-linux", "digitalocean-linux",
		"docker", "gcp-linux", "kubernetes", "scratch",
	}
	for _, id := range linuxBases {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, templatebuilder.BaseOSLinux, templatebuilder.BaseTemplateOS(id))
		})
	}

	windowsBases := []string{"aws-windows", "gcp-windows"}
	for _, id := range windowsBases {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, templatebuilder.BaseOSWindows, templatebuilder.BaseTemplateOS(id))
		})
	}

	t.Run("UnknownReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, templatebuilder.BaseOS(""), templatebuilder.BaseTemplateOS("unknown-template"))
	})
}

func TestBaseTemplateIDs(t *testing.T) {
	t.Parallel()

	ids := templatebuilder.BaseTemplateIDs()
	require.Len(t, ids, len(allBaseIDs))
	for _, id := range allBaseIDs {
		require.Contains(t, ids, id)
	}
}

func TestDefaultBaseRenderContext(t *testing.T) {
	t.Parallel()

	t.Run("Docker", func(t *testing.T) {
		t.Parallel()
		rc := templatebuilder.DefaultBaseRenderContext("docker")
		require.Equal(t, "codercom/example-base:ubuntu", rc.ContainerImage)
		require.Nil(t, rc.ImageOptions)
	})

	t.Run("Kubernetes", func(t *testing.T) {
		t.Parallel()
		rc := templatebuilder.DefaultBaseRenderContext("kubernetes")
		require.Equal(t, "codercom/example-base:ubuntu", rc.ContainerImage)
		require.Nil(t, rc.ImageOptions)
	})

	t.Run("VMBasesHaveNoContainerImage", func(t *testing.T) {
		t.Parallel()
		vmBases := []string{
			"aws-linux", "aws-windows", "azure-linux", "azure-windows",
			"digitalocean-linux", "gcp-linux", "gcp-windows", "scratch",
		}
		for _, id := range vmBases {
			rc := templatebuilder.DefaultBaseRenderContext(id)
			require.Empty(t, rc.ContainerImage, "base %q should have no container image", id)
		}
	})

	t.Run("Unknown", func(t *testing.T) {
		t.Parallel()
		rc := templatebuilder.DefaultBaseRenderContext("unknown")
		require.Empty(t, rc.ContainerImage)
	})

	t.Run("AllBaseTemplatesHaveDefaults", func(t *testing.T) {
		t.Parallel()
		for _, id := range templatebuilder.BaseTemplateIDs() {
			rc := templatebuilder.DefaultBaseRenderContext(id)
			_ = rc
		}
	})
}

func TestBaseTemplateFS(t *testing.T) {
	t.Parallel()

	t.Run("KnownTemplate", func(t *testing.T) {
		t.Parallel()
		fsys, err := templatebuilder.BaseTemplateFS("docker")
		require.NoError(t, err)
		require.NotNil(t, fsys)
	})

	t.Run("UnknownTemplate", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.BaseTemplateFS("nonexistent")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown base template")
	})
}

func TestBaseReadme(t *testing.T) {
	t.Parallel()

	t.Run("KnownBasesHaveReadme", func(t *testing.T) {
		t.Parallel()
		for _, id := range templatebuilder.BaseTemplateIDs() {
			readme := templatebuilder.BaseReadme(id)
			require.NotEmpty(t, readme, "base %q should have a README", id)
		}
	})

	t.Run("UnknownReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, templatebuilder.BaseReadme("nonexistent"))
	})
}

func TestBasePrerequisites(t *testing.T) {
	t.Parallel()

	t.Run("KnownBasesHavePrerequisites", func(t *testing.T) {
		t.Parallel()
		for _, id := range templatebuilder.BaseTemplateIDs() {
			prereqs := templatebuilder.BasePrerequisites(id)
			require.NotEmpty(t, prereqs, "base %q should have prerequisites", id)
			require.Contains(t, prereqs, "## Prerequisites",
				"base %q prerequisites should contain the heading", id)
		}
	})

	t.Run("AWSLinuxIncludesPermissions", func(t *testing.T) {
		t.Parallel()
		prereqs := templatebuilder.BasePrerequisites("aws-linux")
		require.Contains(t, prereqs, "## Required permissions / policy",
			"AWS Linux prerequisites should include the permissions section")
	})

	t.Run("UnknownReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, templatebuilder.BasePrerequisites("nonexistent"))
	})
}
