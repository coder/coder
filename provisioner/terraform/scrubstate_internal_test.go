package terraform

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScrubState(t *testing.T) {
	t.Parallel()

	// A trimmed Terraform state format v4 file: credential-bearing data
	// sources (top-level and nested in a module), a harmless data source,
	// and a managed resource whose attributes must never be touched.
	const stateJSON = `{
		"version": 4,
		"terraform_version": "1.17.2",
		"serial": 7,
		"lineage": "2f2a34f5-72c2-4587-a4f4-9e10a27e3496",
		"outputs": {},
		"resources": [
			{
				"mode": "data",
				"type": "coder_workspace_owner",
				"name": "me",
				"provider": "provider[\"registry.terraform.io/coder/coder\"]",
				"instances": [{"attributes": {"session_token": "owner-session-token", "ssh_private_key": "-----BEGIN PRIVATE KEY-----", "oidc_access_token": "okta-token"}}]
			},
			{
				"mode": "data",
				"type": "coder_external_auth",
				"name": "github",
				"provider": "provider[\"registry.terraform.io/coder/coder\"]",
				"instances": [{"attributes": {"access_token": "gho_secret"}}]
			},
			{
				"module": "module.jetbrains",
				"mode": "data",
				"type": "coder_external_auth",
				"name": "gitlab",
				"provider": "provider[\"registry.terraform.io/coder/coder\"]",
				"instances": [{"attributes": {"access_token": "glpat_secret"}}]
			},
			{
				"mode": "data",
				"type": "coder_workspace",
				"name": "me",
				"provider": "provider[\"registry.terraform.io/coder/coder\"]",
				"instances": [{"attributes": {"name": "dev"}}]
			},
			{
				"mode": "managed",
				"type": "coder_agent",
				"name": "main",
				"provider": "provider[\"registry.terraform.io/coder/coder\"]",
				"instances": [{"attributes": {"token": "agent-token", "env": {"SECRET": "value"}}}]
			}
		]
	}`

	t.Run("RemovesCredentialDataSources", func(t *testing.T) {
		t.Parallel()
		scrubbed, err := scrubState([]byte(stateJSON))
		require.NoError(t, err)

		require.NotContains(t, string(scrubbed), "owner-session-token")
		require.NotContains(t, string(scrubbed), "BEGIN PRIVATE KEY")
		require.NotContains(t, string(scrubbed), "okta-token")
		require.NotContains(t, string(scrubbed), "gho_secret")
		require.NotContains(t, string(scrubbed), "glpat_secret", "module-nested data sources must be scrubbed too")

		var state struct {
			Version          int    `json:"version"`
			TerraformVersion string `json:"terraform_version"`
			Serial           int    `json:"serial"`
			Lineage          string `json:"lineage"`
			Resources        []struct {
				Mode string `json:"mode"`
				Type string `json:"type"`
			} `json:"resources"`
		}
		require.NoError(t, json.Unmarshal(scrubbed, &state))
		require.Equal(t, 4, state.Version, "state metadata must be preserved")
		require.Equal(t, "1.17.2", state.TerraformVersion)
		require.Equal(t, 7, state.Serial)
		require.Equal(t, "2f2a34f5-72c2-4587-a4f4-9e10a27e3496", state.Lineage)

		require.Len(t, state.Resources, 2)
		require.Equal(t, "coder_workspace", state.Resources[0].Type, "harmless data sources must be kept")
		require.Equal(t, "coder_agent", state.Resources[1].Type, "managed resources must never be touched")
		require.Contains(t, string(scrubbed), "agent-token", "managed resource attributes must be preserved")
	})

	t.Run("NoChangeFastPath", func(t *testing.T) {
		t.Parallel()
		const clean = `{"version": 4, "serial": 1, "resources": [{"mode": "managed", "type": "coder_agent", "name": "main", "instances": []}]}`
		scrubbed, err := scrubState([]byte(clean))
		require.NoError(t, err)
		require.JSONEq(t, clean, string(scrubbed))
	})

	t.Run("EmptyState", func(t *testing.T) {
		t.Parallel()
		scrubbed, err := scrubState(nil)
		require.NoError(t, err)
		require.Empty(t, scrubbed)
	})

	t.Run("MalformedState", func(t *testing.T) {
		t.Parallel()
		_, err := scrubState([]byte("not-json"))
		require.Error(t, err)
	})
}
