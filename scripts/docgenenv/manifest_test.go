package docgenenv_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

func TestManifestFindRoute(t *testing.T) {
	t.Parallel()

	m := &docgenenv.Manifest{
		Routes: []docgenenv.Route{
			{Title: "Reference", Children: []docgenenv.Route{
				{Title: "Command Line", Description: "Learn how to use Coder CLI"},
				{Title: "REST API", Description: "Learn how to use Coderd API", Children: []docgenenv.Route{
					{Title: "General"},
				}},
			}},
		},
	}

	rest := m.FindRoute("Reference", "REST API")
	require.NotNil(t, rest)
	require.Equal(t, "Learn how to use Coderd API", rest.Description)

	// The returned pointer aliases the manifest, so mutations persist.
	rest.Children = nil
	require.Nil(t, m.FindRoute("Reference", "REST API").Children)

	require.NotNil(t, m.FindRoute("Reference", "Command Line"))

	// Misses return nil rather than panicking.
	require.Nil(t, m.FindRoute("Reference", "Nope"))
	require.Nil(t, m.FindRoute())
	require.Nil(t, m.FindRoute("REST API")) // Not a top-level route.
}

func TestMarshalJSON(t *testing.T) {
	t.Parallel()

	m := &docgenenv.Manifest{
		Routes: []docgenenv.Route{{Title: "Groups & Roles <beta>", Path: "./a.md"}},
	}

	b, err := docgenenv.MarshalJSON(m)
	require.NoError(t, err)
	require.Contains(t, string(b), `"title": "Groups & Roles <beta>"`)
	require.NotContains(t, string(b), `\u00`)
	require.Equal(t, byte('\n'), b[len(b)-1])

	// Decodes back to the same manifest.
	var got docgenenv.Manifest
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, m, &got)
}
