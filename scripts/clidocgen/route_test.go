package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
	"github.com/coder/serpent"
)

// commandTree is a small command tree shaped like the real one: "coder" with
// a leaf ("ping"), a nested group ("users" > "roles" > "list"), and a hidden
// command that must not get a page or nav entry.
type commandTree struct {
	root, ping, users, roles, list *serpent.Command
}

func newCommandTree() commandTree {
	tr := commandTree{
		root:  &serpent.Command{Use: "coder"},
		ping:  &serpent.Command{Use: "ping <workspace>", Short: "Ping a workspace"},
		users: &serpent.Command{Use: "users", Short: "Manage users"},
		roles: &serpent.Command{Use: "roles", Short: "Manage user roles"},
		list:  &serpent.Command{Use: "list", Short: "List user roles"},
	}
	tr.roles.AddSubcommands(tr.list)
	tr.users.AddSubcommands(tr.roles, &serpent.Command{Use: "secret", Hidden: true})
	tr.root.AddSubcommands(tr.users, tr.ping)
	return tr
}

// TestCLICommandRoute pins the per-command metadata mapping the CLI generator
// mirrors into page front matter: the title is the full invocation as inline
// code and the description is the command's Short, with no curated fields
// invented. Path, IconPath, and State stay zero so the shared emitter cannot
// silently gain a CLI-only value.
func TestCLICommandRoute(t *testing.T) {
	t.Parallel()

	got := cliCommandRoute(newCommandTree().list)
	require.Equal(t, "`coder users roles list`", got.Title)
	require.Equal(t, "List user roles", got.Description)
	require.Empty(t, got.Path)
	require.Empty(t, got.IconPath)
	require.Nil(t, got.State)
	require.Contains(t, docgenenv.FrontMatter(got), "title: \"`coder users roles list`\"\n")
}

// TestFmtDocFilename pins the page layout: pages nest by command path, and a
// command with visible subcommands is the index.md of its own directory.
func TestFmtDocFilename(t *testing.T) {
	t.Parallel()

	tr := newCommandTree()
	require.Equal(t, "index.md", fmtDocFilename(tr.root))
	require.Equal(t, "ping.md", fmtDocFilename(tr.ping))
	require.Equal(t, "users/index.md", fmtDocFilename(tr.users))
	require.Equal(t, "users/roles/index.md", fmtDocFilename(tr.roles))
	require.Equal(t, "users/roles/list.md", fmtDocFilename(tr.list))
}

// TestSubcommandLink pins the Subcommands table links, which are relative to
// the parent page's directory.
func TestSubcommandLink(t *testing.T) {
	t.Parallel()

	tr := newCommandTree()
	require.Equal(t, "./ping.md", subcommandLink(tr.ping))
	require.Equal(t, "./users/index.md", subcommandLink(tr.users))
	require.Equal(t, "./roles/index.md", subcommandLink(tr.roles))
	require.Equal(t, "./list.md", subcommandLink(tr.list))
}

// TestCLIManifestChildren pins the nested nav: entries are sorted by name,
// titled with only their own verb as inline code, point at the nested page
// path, and skip hidden commands.
func TestCLIManifestChildren(t *testing.T) {
	t.Parallel()

	got := cliManifestChildren(newCommandTree().root)
	want := []docgenenv.Route{
		{Title: "`ping`", Description: "Ping a workspace", Path: "reference/cli/ping.md"},
		{
			Title:       "`users`",
			Description: "Manage users",
			Path:        "reference/cli/users/index.md",
			Children: []docgenenv.Route{{
				Title:       "`roles`",
				Description: "Manage user roles",
				Path:        "reference/cli/users/roles/index.md",
				Children: []docgenenv.Route{{
					Title:       "`list`",
					Description: "List user roles",
					Path:        "reference/cli/users/roles/list.md",
				}},
			}},
		},
	}
	require.Equal(t, want, got)
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
