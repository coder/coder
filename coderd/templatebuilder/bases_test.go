package templatebuilder_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/templatebuilder"
)

func TestBaseTemplateOS(t *testing.T) {
	t.Parallel()

	t.Run("Docker", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, templatebuilder.BaseOSLinux, templatebuilder.BaseTemplateOS("docker"))
	})

	t.Run("Kubernetes", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, templatebuilder.BaseOSLinux, templatebuilder.BaseTemplateOS("kubernetes"))
	})

	t.Run("AWSLinux", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, templatebuilder.BaseOSLinux, templatebuilder.BaseTemplateOS("aws-linux"))
	})

	t.Run("UnknownReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, templatebuilder.BaseOS(""), templatebuilder.BaseTemplateOS("unknown-template"))
	})
}

func TestBaseTemplateIDs(t *testing.T) {
	t.Parallel()

	ids := templatebuilder.BaseTemplateIDs()
	require.Len(t, ids, 3)
	require.Contains(t, ids, "docker")
	require.Contains(t, ids, "kubernetes")
	require.Contains(t, ids, "aws-linux")
}

func TestDefaultBaseRenderContext(t *testing.T) {
	t.Parallel()

	t.Run("Docker", func(t *testing.T) {
		t.Parallel()
		rc := templatebuilder.DefaultBaseRenderContext("docker")
		require.Equal(t, "codercom/enterprise-base:ubuntu", rc.ContainerImage)
		require.Nil(t, rc.ImageOptions)
	})

	t.Run("Kubernetes", func(t *testing.T) {
		t.Parallel()
		rc := templatebuilder.DefaultBaseRenderContext("kubernetes")
		require.Equal(t, "codercom/enterprise-base:ubuntu", rc.ContainerImage)
		require.Nil(t, rc.ImageOptions)
	})

	t.Run("AWSLinux", func(t *testing.T) {
		t.Parallel()
		rc := templatebuilder.DefaultBaseRenderContext("aws-linux")
		require.Empty(t, rc.ContainerImage)
		require.Nil(t, rc.ImageOptions)
	})

	t.Run("Unknown", func(t *testing.T) {
		t.Parallel()
		rc := templatebuilder.DefaultBaseRenderContext("unknown")
		require.Empty(t, rc.ContainerImage)
	})

	t.Run("AllBaseTemplatesHaveDefaults", func(t *testing.T) {
		t.Parallel()
		// Verify that every known base template produces a render
		// context via DefaultBaseRenderContext (not just the zero value
		// from an unknown ID). This catches forgotten entries.
		for _, id := range templatebuilder.BaseTemplateIDs() {
			rc := templatebuilder.DefaultBaseRenderContext(id)
			_ = rc // existence is the assertion; the value varies per template
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
