package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli"
)

func TestParseTemplateFrontmatter(t *testing.T) {
	t.Parallel()

	t.Run("AllFields", func(t *testing.T) {
		t.Parallel()
		data := []byte("---\ndisplay_name: Docker Containers\ndescription: Provision Docker containers as Coder workspaces\nicon: ../../../site/static/icon/docker.png\n---\n# Docker Containers\nSome content here.\n")
		fm, err := cli.ParseTemplateFrontmatter(data)
		require.NoError(t, err)
		require.Equal(t, "Docker Containers", fm.DisplayName)
		require.Equal(t, "Provision Docker containers as Coder workspaces", fm.Description)
		require.Equal(t, "../../../site/static/icon/docker.png", fm.Icon)
	})

	t.Run("NoFrontmatter", func(t *testing.T) {
		t.Parallel()
		data := []byte("# Just a README\nNo frontmatter here.\n")
		fm, err := cli.ParseTemplateFrontmatter(data)
		require.NoError(t, err)
		require.Empty(t, fm.DisplayName)
	})

	t.Run("InvalidYAML", func(t *testing.T) {
		t.Parallel()
		data := []byte("---\ndisplay_name: [invalid\n  yaml: {broken\n---\n# Content\n")
		_, err := cli.ParseTemplateFrontmatter(data)
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse README.md frontmatter")
	})
}
