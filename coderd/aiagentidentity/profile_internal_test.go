package aiagentidentity

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func TestValidateProfileBuiltIns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		profile       Profile
		wantAllowList int
	}{
		{
			name:          "chat",
			profile:       ChatAgentProfile(uuid.New()),
			wantAllowList: 6,
		},
		{
			name:          "workspace",
			profile:       WorkspaceAgentIdentityProfile(uuid.New()),
			wantAllowList: 2,
		},
		{
			name:          "sandbox",
			profile:       SandboxIdentityProfile(uuid.New(), uuid.New()),
			wantAllowList: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			validated, err := validateProfile(tt.profile)
			require.NoError(t, err)
			require.Len(t, tt.profile.Scopes, 7)
			require.Contains(t, tt.profile.Scopes, database.ApiKeyScopeMcpGatewayUse)
			require.Len(t, tt.profile.AllowList, tt.wantAllowList)
			require.Contains(t, tt.profile.AllowList, rbac.AllowListElement{
				Type: rbac.ResourceMcpGateway.Type,
				ID:   policy.WildcardSymbol,
			})
			require.Equal(t, tt.profile.Scopes, validated.Scopes)
			require.ElementsMatch(t, tt.profile.AllowList, validated.AllowList)
		})
	}
}

func TestValidateProfileRejectsUnpermittedScopes(t *testing.T) {
	t.Parallel()

	allowList := database.AllowList{{
		Type: rbac.ResourceWorkspace.Type,
		ID:   uuid.New().String(),
	}}
	tests := []struct {
		name      string
		scope     database.APIKeyScope
		wantError string
	}{
		{
			name:      "API key wildcard",
			scope:     database.ApiKeyScopeApiKey,
			wantError: "api_key:*",
		},
		{
			name:      "user secret wildcard",
			scope:     database.ApiKeyScopeUserSecret,
			wantError: "user_secret:*",
		},
		{
			name:      "coder all",
			scope:     database.ApiKeyScopeCoderAll,
			wantError: "coder:all",
		},
		{
			name:      "manage own API keys",
			scope:     database.ApiKeyScopeCoderApikeysmanageSelf,
			wantError: "coder:apikeys.manage_self",
		},
		{
			name:      "author templates",
			scope:     database.ApiKeyScopeCoderTemplatesauthor,
			wantError: "coder:templates.author",
		},
		{
			name:      "composite expansion",
			scope:     database.ApiKeyScopeCoderTemplatesbuild,
			wantError: "coder:templates.build",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := validateProfile(Profile{
				Scopes:    database.APIKeyScopes{tt.scope},
				AllowList: allowList,
			})
			require.ErrorContains(t, err, tt.wantError)
			if tt.scope == database.ApiKeyScopeCoderTemplatesbuild {
				require.ErrorContains(t, err, "expands to permission")
			}
		})
	}
}

func TestValidateProfileRejectsGlobalAllowList(t *testing.T) {
	t.Parallel()

	_, err := validateProfile(Profile{
		Scopes:    database.APIKeyScopes{database.ApiKeyScopeWorkspaceRead},
		AllowList: database.AllowList{rbac.AllowListAll()},
	})
	require.ErrorContains(t, err, "*:*", "error must name the offending allow-list entry")
}

func TestValidateProfileRejectsEmptyBounds(t *testing.T) {
	t.Parallel()

	_, err := validateProfile(Profile{
		AllowList: database.AllowList{{
			Type: rbac.ResourceWorkspace.Type,
			ID:   uuid.New().String(),
		}},
	})
	require.ErrorContains(t, err, "at least one scope")

	_, err = validateProfile(Profile{
		Scopes: database.APIKeyScopes{database.ApiKeyScopeWorkspaceRead},
	})
	require.ErrorContains(t, err, "at least one allow-list entry")
}

func TestValidateProfileAcceptedScopeCatalog(t *testing.T) {
	t.Parallel()

	expected := database.APIKeyScopes{
		database.ApiKeyScopeCoderWorkspacescreate,
		database.ApiKeyScopeCoderWorkspacesoperate,
		database.ApiKeyScopeCoderWorkspacesaccess,
		database.ApiKeyScopeChatRead,
		database.ApiKeyScopeChatUpdate,
		database.ApiKeyScopeUserRead,
		database.ApiKeyScopeWorkspaceRead,
		database.ApiKeyScopeWorkspaceUpdate,
		database.ApiKeyScopeWorkspaceStart,
		database.ApiKeyScopeWorkspaceStop,
		database.ApiKeyScopeWorkspaceSsh,
		database.ApiKeyScopeWorkspaceApplicationConnect,
		database.ApiKeyScopeMcpGatewayUse,
	}
	allowList := database.AllowList{{
		Type: rbac.ResourceWorkspace.Type,
		ID:   uuid.New().String(),
	}}
	accepted := make(database.APIKeyScopes, 0, len(expected))
	for _, scope := range database.AllAPIKeyScopeValues() {
		_, err := validateProfile(Profile{
			Scopes:    database.APIKeyScopes{scope},
			AllowList: allowList,
		})
		if err == nil {
			accepted = append(accepted, scope)
		}
	}

	require.Len(t, accepted, 13)
	require.ElementsMatch(t, expected, accepted)
}
