package templatebuilder

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestParseModulesFromFS(t *testing.T) {
	t.Parallel()

	t.Run("ValidManifest", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"modules/mymod/module.json": &fstest.MapFile{
				Data: []byte(`{
					"id": "mymod",
					"display_name": "My Module",
					"description": "A test module.",
					"icon": "/icons/mymod.svg",
					"category": "IDE",
					"tags": ["ide"],
					"compatible_os": ["linux"],
					"conflicts_with": ["other"],
					"pinned_version": "1.2.3",
					"variables": [
						{
							"name": "agent_id",
							"type": "string",
							"description": "The Coder agent ID.",
							"required": true,
							"sensitive": false,
							"computed": true
						},
						{
							"name": "port",
							"type": "number",
							"description": "Port number.",
							"default": 8080,
							"required": false,
							"sensitive": false,
							"computed": false
						},
						{
							"name": "enable_debug",
							"type": "bool",
							"description": "Enable debug mode.",
							"required": false,
							"sensitive": false,
							"computed": false
						},
						{
							"name": "api_key",
							"type": "string",
							"description": "Secret API key.",
							"required": true,
							"sensitive": true,
							"computed": false
						}
					]
				}`),
			},
		}

		modules, err := parseModulesFromFS(fsys)
		require.NoError(t, err)
		require.Len(t, modules, 1)

		m := modules[0]
		require.Equal(t, "mymod", m.ID)
		require.Equal(t, "My Module", m.DisplayName)
		require.Equal(t, "A test module.", m.Description)
		require.Equal(t, "/icons/mymod.svg", m.Icon)
		require.Equal(t, "IDE", m.Category)
		require.Equal(t, []string{"ide"}, m.Tags)
		require.Equal(t, []string{"linux"}, m.CompatibleOS)
		require.Equal(t, []string{"other"}, m.ConflictsWith)
		require.Equal(t, "1.2.3", m.PinnedVersion)
		require.Len(t, m.Variables, 4)

		// Verify variable types parsed correctly.
		require.Equal(t, "string", m.Variables[0].Type)
		require.Equal(t, "number", m.Variables[1].Type)
		require.Equal(t, "bool", m.Variables[2].Type)

		// Verify computed and sensitive fields.
		require.True(t, m.Variables[0].Computed)
		require.True(t, m.Variables[3].Sensitive)

		// Verify default pointer.
		require.Nil(t, m.Variables[0].Default)
		require.NotNil(t, m.Variables[1].Default)
		require.JSONEq(t, "8080", string(m.Variables[1].Default))
	})

	t.Run("MultipleModules", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"modules/alpha/module.json": &fstest.MapFile{
				Data: []byte(`{"id": "alpha", "pinned_version": "1.0.0"}`),
			},
			"modules/beta/module.json": &fstest.MapFile{
				Data: []byte(`{"id": "beta", "pinned_version": "2.0.0"}`),
			},
		}

		modules, err := parseModulesFromFS(fsys)
		require.NoError(t, err)
		require.Len(t, modules, 2)

		ids := []string{modules[0].ID, modules[1].ID}
		require.Contains(t, ids, "alpha")
		require.Contains(t, ids, "beta")
	})

	t.Run("EmptyCatalog", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"modules/.keep": &fstest.MapFile{Data: []byte{}},
		}

		modules, err := parseModulesFromFS(fsys)
		require.NoError(t, err)
		require.Empty(t, modules)
	})

	rejects := []struct {
		name    string
		fsys    fstest.MapFS
		wantErr string
	}{
		{
			name: "RejectsDirWithoutManifest",
			fsys: fstest.MapFS{
				"modules/nomod/readme.txt": &fstest.MapFile{Data: []byte("hi")},
			},
			wantErr: "read nomod/module.json",
		},
		{
			name: "RejectsEmptyID",
			fsys: fstest.MapFS{
				"modules/bad/module.json": &fstest.MapFile{
					Data: []byte(`{"id": "", "pinned_version": "1.0.0"}`),
				},
			},
			wantErr: "empty id",
		},
		{
			name: "RejectsEmptyPinnedVersion",
			fsys: fstest.MapFS{
				"modules/bad/module.json": &fstest.MapFile{
					Data: []byte(`{"id": "bad", "pinned_version": ""}`),
				},
			},
			wantErr: "empty pinned_version",
		},
		{
			name: "RejectsDuplicateID",
			fsys: fstest.MapFS{
				"modules/a/module.json": &fstest.MapFile{
					Data: []byte(`{"id": "dupe", "pinned_version": "1.0.0"}`),
				},
				"modules/b/module.json": &fstest.MapFile{
					Data: []byte(`{"id": "dupe", "pinned_version": "2.0.0"}`),
				},
			},
			wantErr: "duplicate module id",
		},
		{
			name: "RejectsUnknownVariableType",
			fsys: fstest.MapFS{
				"modules/bad/module.json": &fstest.MapFile{
					Data: []byte(`{
						"id": "bad",
						"pinned_version": "1.0.0",
						"variables": [{"name": "x", "type": "list"}]
					}`),
				},
			},
			wantErr: `unknown type "list"`,
		},
		{
			name: "RejectsUnknownField",
			fsys: fstest.MapFS{
				"modules/bad/module.json": &fstest.MapFile{
					Data: []byte(`{"id": "bad", "pinned_version": "1.0.0", "dispaly_name": "typo"}`),
				},
			},
			wantErr: "decode",
		},
		{
			name: "RejectsEmptyVariableName",
			fsys: fstest.MapFS{
				"modules/bad/module.json": &fstest.MapFile{
					Data: []byte(`{
						"id": "bad",
						"pinned_version": "1.0.0",
						"variables": [{"name": "", "type": "string"}]
					}`),
				},
			},
			wantErr: "empty name",
		},
		{
			name: "RejectsDuplicateVariableName",
			fsys: fstest.MapFS{
				"modules/bad/module.json": &fstest.MapFile{
					Data: []byte(`{
						"id": "bad",
						"pinned_version": "1.0.0",
						"variables": [
							{"name": "x", "type": "string"},
							{"name": "x", "type": "number"}
						]
					}`),
				},
			},
			wantErr: "duplicate variable name",
		},
		{
			name: "RejectsInvalidJSON",
			fsys: fstest.MapFS{
				"modules/bad/module.json": &fstest.MapFile{
					Data: []byte(`{not json`),
				},
			},
			wantErr: "decode",
		},
	}
	for _, tc := range rejects {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseModulesFromFS(tc.fsys)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
