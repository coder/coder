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

func TestBaseTemplatesMapConsistency(t *testing.T) {
	t.Parallel()

	// Verify every entry in the map has a matching ExampleID key.
	for key, bt := range templatebuilder.BaseTemplates {
		require.Equal(t, key, bt.ExampleID,
			"map key %q must match ExampleID %q", key, bt.ExampleID)
		require.NotEmpty(t, bt.OS, "base template %q must have a non-empty OS", key)
	}
}

func TestDefaultBaseRenderContext(t *testing.T) {
	t.Parallel()

	t.Run("Docker", func(t *testing.T) {
		t.Parallel()
		ctx := templatebuilder.DefaultBaseRenderContext("docker")
		require.Equal(t, "codercom/enterprise-base:ubuntu", ctx.ContainerImage)
		require.Nil(t, ctx.ImageOptions)
	})

	t.Run("Kubernetes", func(t *testing.T) {
		t.Parallel()
		ctx := templatebuilder.DefaultBaseRenderContext("kubernetes")
		require.Equal(t, "codercom/enterprise-base:ubuntu", ctx.ContainerImage)
		require.Nil(t, ctx.ImageOptions)
	})

	t.Run("AWSLinux", func(t *testing.T) {
		t.Parallel()
		ctx := templatebuilder.DefaultBaseRenderContext("aws-linux")
		require.Empty(t, ctx.ContainerImage)
		require.Nil(t, ctx.ImageOptions)
	})

	t.Run("Unknown", func(t *testing.T) {
		t.Parallel()
		ctx := templatebuilder.DefaultBaseRenderContext("unknown")
		require.Empty(t, ctx.ContainerImage)
	})
}
