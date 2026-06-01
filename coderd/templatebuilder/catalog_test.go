package templatebuilder_test

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/templatebuilder"
	"github.com/coder/coder/v2/codersdk"
)

func TestLoadModules(t *testing.T) {
	t.Parallel()

	modules, err := templatebuilder.LoadModules()
	require.NoError(t, err)
	require.NotEmpty(t, modules, "embedded catalog should contain at least one module")

	// Verify the code-server module is present and valid.
	var found bool
	for _, m := range modules {
		if m.ID == "code-server" {
			found = true
			require.Equal(t, "code-server", m.DisplayName)
			require.Equal(t, "IDE", m.Category)
			require.Equal(t, []string{"linux"}, m.CompatibleOS)
			require.NotEmpty(t, m.PinnedVersion)
			break
		}
	}
	require.True(t, found, "code-server module must be in the embedded catalog")
}

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
							"builder_managed": true
						},
						{
							"name": "port",
							"type": "number",
							"description": "Port number.",
							"default": "8080",
							"required": false,
							"sensitive": false,
							"builder_managed": false
						},
						{
							"name": "enable_debug",
							"type": "bool",
							"description": "Enable debug mode.",
							"required": false,
							"sensitive": false,
							"builder_managed": false
						},
						{
							"name": "api_key",
							"type": "string",
							"description": "Secret API key.",
							"required": true,
							"sensitive": true,
							"builder_managed": false
						}
					]
				}`),
			},
		}

		modules, err := templatebuilder.ParseModulesFromFS(fsys)
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

		// Verify builder_managed and sensitive fields.
		require.True(t, m.Variables[0].BuilderManaged)
		require.True(t, m.Variables[3].Sensitive)

		// Verify default pointer.
		require.Nil(t, m.Variables[0].Default)
		require.NotNil(t, m.Variables[1].Default)
		require.Equal(t, "8080", *m.Variables[1].Default)
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

		modules, err := templatebuilder.ParseModulesFromFS(fsys)
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

		modules, err := templatebuilder.ParseModulesFromFS(fsys)
		require.NoError(t, err)
		require.Empty(t, modules)
	})

	t.Run("SkipsDirWithoutManifest", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"modules/nomod/readme.txt": &fstest.MapFile{Data: []byte("hi")},
		}

		modules, err := templatebuilder.ParseModulesFromFS(fsys)
		require.NoError(t, err)
		require.Empty(t, modules)
	})

	t.Run("RejectsEmptyID", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"modules/bad/module.json": &fstest.MapFile{
				Data: []byte(`{"id": "", "pinned_version": "1.0.0"}`),
			},
		}

		_, err := templatebuilder.ParseModulesFromFS(fsys)
		require.ErrorContains(t, err, "empty id")
	})

	t.Run("RejectsEmptyPinnedVersion", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"modules/bad/module.json": &fstest.MapFile{
				Data: []byte(`{"id": "bad", "pinned_version": ""}`),
			},
		}

		_, err := templatebuilder.ParseModulesFromFS(fsys)
		require.ErrorContains(t, err, "empty pinned_version")
	})

	t.Run("RejectsDuplicateID", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"modules/a/module.json": &fstest.MapFile{
				Data: []byte(`{"id": "dupe", "pinned_version": "1.0.0"}`),
			},
			"modules/b/module.json": &fstest.MapFile{
				Data: []byte(`{"id": "dupe", "pinned_version": "2.0.0"}`),
			},
		}

		_, err := templatebuilder.ParseModulesFromFS(fsys)
		require.ErrorContains(t, err, "duplicate module id")
	})

	t.Run("RejectsUnknownVariableType", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"modules/bad/module.json": &fstest.MapFile{
				Data: []byte(`{
					"id": "bad",
					"pinned_version": "1.0.0",
					"variables": [{"name": "x", "type": "list"}]
				}`),
			},
		}

		_, err := templatebuilder.ParseModulesFromFS(fsys)
		require.ErrorContains(t, err, `unknown type "list"`)
	})

	t.Run("RejectsUnknownField", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"modules/bad/module.json": &fstest.MapFile{
				Data: []byte(`{"id": "bad", "pinned_version": "1.0.0", "dispaly_name": "typo"}`),
			},
		}

		_, err := templatebuilder.ParseModulesFromFS(fsys)
		require.ErrorContains(t, err, "decode")
	})

	t.Run("RejectsEmptyVariableName", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"modules/bad/module.json": &fstest.MapFile{
				Data: []byte(`{
					"id": "bad",
					"pinned_version": "1.0.0",
					"variables": [{"name": "", "type": "string"}]
				}`),
			},
		}

		_, err := templatebuilder.ParseModulesFromFS(fsys)
		require.ErrorContains(t, err, "empty name")
	})

	t.Run("RejectsDuplicateVariableName", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
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
		}

		_, err := templatebuilder.ParseModulesFromFS(fsys)
		require.ErrorContains(t, err, "duplicate variable name")
	})

	t.Run("RejectsInvalidJSON", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"modules/bad/module.json": &fstest.MapFile{
				Data: []byte(`{not json`),
			},
		}

		_, err := templatebuilder.ParseModulesFromFS(fsys)
		require.ErrorContains(t, err, "decode")
	})
}

func TestToSDK(t *testing.T) {
	t.Parallel()

	defaultVal := "8080"
	manifest := templatebuilder.ModuleManifest{
		ID:            "test-mod",
		DisplayName:   "Test Module",
		Description:   "A module for testing.",
		Icon:          "/icons/test.svg",
		Category:      "Utility",
		Tags:          []string{"test"},
		CompatibleOS:  []string{"linux", "darwin"},
		ConflictsWith: []string{"conflicting-mod"},
		PinnedVersion: "2.5.0",
		Variables: []templatebuilder.ModuleVariable{
			{
				Name:           "agent_id",
				Type:           "string",
				Description:    "The Coder agent ID.",
				Required:       true,
				Sensitive:      false,
				BuilderManaged: true,
			},
			{
				Name:           "port",
				Type:           "number",
				Description:    "Port to listen on.",
				Default:        &defaultVal,
				Required:       false,
				Sensitive:      false,
				BuilderManaged: false,
			},
			{
				Name:           "secret_key",
				Type:           "string",
				Description:    "A sensitive value.",
				Required:       true,
				Sensitive:      true,
				BuilderManaged: false,
			},
		},
	}

	sdk := manifest.ToSDK()

	t.Run("TopLevelFields", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "test-mod", sdk.ID)
		require.Equal(t, "Test Module", sdk.DisplayName)
		require.Equal(t, "A module for testing.", sdk.Description)
		require.Equal(t, "/icons/test.svg", sdk.Icon)
		require.Equal(t, "Utility", sdk.Category)
		require.Equal(t, "2.5.0", sdk.Version, "PinnedVersion should map to Version")
		require.Equal(t, []string{"linux", "darwin"}, sdk.CompatibleOS)
		require.Equal(t, []string{"conflicting-mod"}, sdk.ConflictsWith)
	})

	t.Run("AllVariableFields", func(t *testing.T) {
		t.Parallel()

		require.Len(t, sdk.Variables, 3)

		agent := sdk.Variables[0]
		require.Equal(t, "agent_id", agent.Name)
		require.Equal(t, codersdk.TemplateBuilderVariableTypeString, agent.Type)
		require.Equal(t, "The Coder agent ID.", agent.Description)
		require.Nil(t, agent.Default)
		require.True(t, agent.Required)
		require.False(t, agent.Sensitive)
		require.True(t, agent.BuilderManaged)

		port := sdk.Variables[1]
		require.Equal(t, "port", port.Name)
		require.Equal(t, codersdk.TemplateBuilderVariableTypeNumber, port.Type)
		require.Equal(t, "Port to listen on.", port.Description)
		require.NotNil(t, port.Default)
		require.Equal(t, "8080", *port.Default)
		require.False(t, port.Required)
		require.False(t, port.Sensitive)
		require.False(t, port.BuilderManaged)

		secret := sdk.Variables[2]
		require.Equal(t, "secret_key", secret.Name)
		require.Equal(t, codersdk.TemplateBuilderVariableTypeString, secret.Type)
		require.Equal(t, "A sensitive value.", secret.Description)
		require.Nil(t, secret.Default)
		require.True(t, secret.Required)
		require.True(t, secret.Sensitive)
		require.False(t, secret.BuilderManaged)
	})

	t.Run("NilSlicesNormalizedToEmpty", func(t *testing.T) {
		t.Parallel()

		m := templatebuilder.ModuleManifest{
			ID:            "nil-slices",
			PinnedVersion: "1.0.0",
			// CompatibleOS and ConflictsWith are nil.
		}
		s := m.ToSDK()
		require.NotNil(t, s.CompatibleOS, "nil CompatibleOS should become empty slice")
		require.NotNil(t, s.ConflictsWith, "nil ConflictsWith should become empty slice")
		require.NotNil(t, s.Variables, "nil Variables should become empty slice")
		require.Empty(t, s.CompatibleOS)
		require.Empty(t, s.ConflictsWith)
		require.Empty(t, s.Variables)
	})
}
