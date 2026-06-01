package templatebuilder_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/templatebuilder"
	"github.com/coder/coder/v2/codersdk"
)

func TestLoadModules(t *testing.T) {
	t.Parallel()

	modules, err := templatebuilder.LoadModules()
	require.NoError(t, err)
	require.NotEmpty(t, modules, "catalog should contain at least the stub module")

	// Find the stub module used for testing.
	var stub *templatebuilder.ModuleManifest
	for i := range modules {
		if modules[i].ID == "stub" {
			stub = &modules[i]
			break
		}
	}
	require.NotNil(t, stub, "stub module must be present in the catalog")

	t.Run("ManifestFields", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "stub", stub.ID)
		require.Equal(t, "Stub Module", stub.DisplayName)
		require.Equal(t, "Utility", stub.Category)
		require.Equal(t, "0.0.0", stub.PinnedVersion)
		require.Equal(t, []string{"linux"}, stub.CompatibleOS)
		require.Empty(t, stub.ConflictsWith)
		require.Len(t, stub.Variables, 2)
	})

	t.Run("BuilderManagedVariable", func(t *testing.T) {
		t.Parallel()

		agentVar := findVariable(t, stub.Variables, "agent_id")
		require.True(t, agentVar.BuilderManaged, "agent_id should be builder_managed")
		require.True(t, agentVar.Required)
		require.False(t, agentVar.Sensitive)
		require.Equal(t, "string", agentVar.Type)
	})

	t.Run("SensitiveVariable", func(t *testing.T) {
		t.Parallel()

		secretVar := findVariable(t, stub.Variables, "example_secret")
		require.True(t, secretVar.Sensitive, "example_secret should be sensitive")
		require.False(t, secretVar.BuilderManaged)
		require.False(t, secretVar.Required)
		require.Equal(t, "string", secretVar.Type)
	})
}

func TestToSDK(t *testing.T) {
	t.Parallel()

	modules, err := templatebuilder.LoadModules()
	require.NoError(t, err)

	var stub templatebuilder.ModuleManifest
	for _, m := range modules {
		if m.ID == "stub" {
			stub = m
			break
		}
	}
	require.NotEmpty(t, stub.ID, "stub module must be present")

	sdk := stub.ToSDK()

	t.Run("PinnedVersionMapsToVersion", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, stub.PinnedVersion, sdk.Version)
		require.Equal(t, "0.0.0", sdk.Version)
	})

	t.Run("FieldsPreserved", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, stub.ID, sdk.ID)
		require.Equal(t, stub.DisplayName, sdk.DisplayName)
		require.Equal(t, stub.Description, sdk.Description)
		require.Equal(t, stub.Category, sdk.Category)
		require.Equal(t, stub.CompatibleOS, sdk.CompatibleOS)
		require.Equal(t, stub.ConflictsWith, sdk.ConflictsWith)
	})

	t.Run("VariablesConverted", func(t *testing.T) {
		t.Parallel()
		require.Len(t, sdk.Variables, 2)

		agentVar := findSDKVariable(t, sdk.Variables, "agent_id")
		require.Equal(t, codersdk.TemplateBuilderVariableTypeString, agentVar.Type)
		require.True(t, agentVar.BuilderManaged)

		secretVar := findSDKVariable(t, sdk.Variables, "example_secret")
		require.True(t, secretVar.Sensitive)
		require.False(t, secretVar.BuilderManaged)
	})
}

func findVariable(t *testing.T, vars []templatebuilder.ModuleVariable, name string) templatebuilder.ModuleVariable {
	t.Helper()
	for _, v := range vars {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("variable %q not found", name)
	return templatebuilder.ModuleVariable{}
}

func findSDKVariable(t *testing.T, vars []codersdk.TemplateBuilderModuleVariable, name string) codersdk.TemplateBuilderModuleVariable {
	t.Helper()
	for _, v := range vars {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("variable %q not found", name)
	return codersdk.TemplateBuilderModuleVariable{}
}
