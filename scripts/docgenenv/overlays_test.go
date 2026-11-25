package docgenenv_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

func writeOverlays(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "overlays.yml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func TestLoadOverlays(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		o, err := docgenenv.LoadOverlays(writeOverlays(t, `index:
  description: Reference for the API.
  icon_path: ./images/icons/api.svg
sections:
  - title: Agents
    description: REST endpoints for the agent.
    state: [beta]
`))
		require.NoError(t, err)
		require.Equal(t, docgenenv.PageMetadata{Description: "Reference for the API.", IconPath: "./images/icons/api.svg"}, o.Index)
		require.Equal(t, []docgenenv.SectionMetadata{{
			Title:        "Agents",
			PageMetadata: docgenenv.PageMetadata{Description: "REST endpoints for the agent.", State: []string{"beta"}},
		}}, o.Sections)
	})

	for _, tc := range []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "IndexDescriptionWithoutPeriod",
			content: "index:\n  description: Reference\n",
			wantErr: `index: description must end with a period: "Reference"`,
		},
		{
			name:    "SectionDescriptionWithoutPeriod",
			content: "sections:\n  - title: Agents\n    description: REST endpoints\n",
			wantErr: `Agents: description must end with a period`,
		},
		{
			name:    "SectionMissingTitle",
			content: "sections:\n  - description: Orphan.\n",
			wantErr: "section overlay is missing a title",
		},
		{
			name:    "DuplicateSection",
			content: "sections:\n  - title: Agents\n  - title: Agents\n",
			wantErr: `section "Agents" is listed more than once`,
		},
		{
			name:    "UnknownKey",
			content: "index:\n  descripton: Typo.\n",
			wantErr: "field descripton not found",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := docgenenv.LoadOverlays(writeOverlays(t, tc.content))
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestLoadOverlaysCRLF(t *testing.T) {
	t.Parallel()

	o, err := docgenenv.LoadOverlays(writeOverlays(t, "index:\r\n  description: Reference.\r\n"))
	require.NoError(t, err)
	require.Equal(t, "Reference.", o.Index.Description)
}

func TestPageMetadataRoute(t *testing.T) {
	t.Parallel()

	p := docgenenv.PageMetadata{Description: "Desc.", IconPath: "./i.svg", State: []string{"beta"}}
	r := p.Route("Title")
	require.Equal(t, docgenenv.Route{Title: "Title", Description: "Desc.", IconPath: "./i.svg", State: []string{"beta"}}, r)

	// The route must not share the overlay's state slice.
	r.State[0] = "changed"
	require.Equal(t, []string{"beta"}, p.State)
}
