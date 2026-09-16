package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
	"github.com/coder/serpent"
)

// TestCLICommandRoute pins the per-command metadata mapping the CLI generator
// mirrors into page front matter: title from the command's full name and
// description from its Short, with no curated fields invented. Path, IconPath,
// and State stay zero here (Path is layered on by the manifest rebuild;
// IconPath/State are index/manifest-authored only), so the shared emitter
// cannot silently gain a CLI-only value.
func TestCLICommandRoute(t *testing.T) {
	t.Parallel()

	got := cliCommandRoute(&serpent.Command{Use: "ping", Short: "Ping a workspace"})
	require.Equal(t, "ping", got.Title)
	require.Equal(t, "Ping a workspace", got.Description)
	require.Empty(t, got.Path)
	require.Empty(t, got.IconPath)
	require.Nil(t, got.State)
}

// TestCLIIndexRouteMirrorsManifest pins the CLI index page's route: it copies
// the whole "Command Line" manifest route (minus nav children) so curated,
// index-only fields (icon_path, state) still reach the shared front-matter
// emitter. No Command Line route carries icon_path or state today, so neither
// the golden regen nor the per-command test above exercises this arm.
func TestCLIIndexRouteMirrorsManifest(t *testing.T) {
	t.Parallel()

	src := docgenenv.Route{
		Title:       "Command Line",
		Description: "Learn how to use Coder CLI",
		IconPath:    "./images/icons/terminal.svg",
		State:       []string{"beta"},
		Children:    []docgenenv.Route{{Title: "ping", Path: "reference/cli/ping.md"}},
	}
	got := cliIndexRouteFrom(src)
	require.Equal(t, src.Title, got.Title)
	require.Equal(t, src.Description, got.Description)
	require.Equal(t, src.IconPath, got.IconPath)
	require.Equal(t, src.State, got.State)
	require.Nil(t, got.Children, "nav children must not leak into index front matter")

	fm := docgenenv.FrontMatter(got)
	require.Contains(t, fm, `icon_path: "./images/icons/terminal.svg"`)
	require.Contains(t, fm, "state:\n  - beta")
}
