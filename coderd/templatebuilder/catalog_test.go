package templatebuilder_test

import (
	"encoding/json"
	"testing"

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

func TestToSDK(t *testing.T) {
	t.Parallel()

	defaultVal := json.RawMessage(`8080`)
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
				Name:        "agent_id",
				Type:        "string",
				Description: "The Coder agent ID.",
				Required:    true,
				Sensitive:   false,
				Computed:    true,
			},
			{
				Name:        "port",
				Type:        "number",
				Description: "Port to listen on.",
				Default:     defaultVal,
				Required:    false,
				Sensitive:   false,
				Computed:    false,
			},
			{
				Name:        "secret_key",
				Type:        "string",
				Description: "A sensitive value.",
				Required:    true,
				Sensitive:   true,
				Computed:    false,
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

		// Computed variables (agent_id) are excluded from SDK output.
		require.Len(t, sdk.Variables, 2)

		port := sdk.Variables[0]
		require.Equal(t, "port", port.Name)
		require.Equal(t, codersdk.TemplateBuilderVariableTypeNumber, port.Type)
		require.Equal(t, "Port to listen on.", port.Description)
		require.NotNil(t, port.Default)
		require.JSONEq(t, "8080", string(port.Default))
		require.False(t, port.Required)
		require.False(t, port.Sensitive)

		secret := sdk.Variables[1]
		require.Equal(t, "secret_key", secret.Name)
		require.Equal(t, codersdk.TemplateBuilderVariableTypeString, secret.Type)
		require.Equal(t, "A sensitive value.", secret.Description)
		require.Nil(t, secret.Default)
		require.True(t, secret.Required)
		require.True(t, secret.Sensitive)
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

func TestCompatibleWithOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		compatibleOS []string
		os           string
		want         bool
	}{
		{"EmptyListMatchesAll", nil, "linux", true},
		{"ExactMatch", []string{"linux"}, "linux", true},
		{"MultipleMatch", []string{"linux", "windows"}, "windows", true},
		{"NoMatch", []string{"linux"}, "windows", false},
		{"CaseSensitive", []string{"linux"}, "Linux", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := templatebuilder.ModuleManifest{
				ID:            "test",
				PinnedVersion: "1.0.0",
				CompatibleOS:  tc.compatibleOS,
			}
			require.Equal(t, tc.want, m.CompatibleWithOS(tc.os))
		})
	}
}
