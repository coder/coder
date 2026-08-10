package aiagentidentity

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

const keyLifetime = 24 * time.Hour

// Profile defines the scopes and resource allow list for an AI agent API key.
type Profile struct {
	Scopes    database.APIKeyScopes
	AllowList database.AllowList
	TokenName string
}

// ChatAgentProfile returns the platform and chat permissions needed by a chat
// AI agent identity. Workspace-related resources use typed wildcards because a
// newly created workspace does not have an ID that can be pinned in advance.
func ChatAgentProfile(chatID uuid.UUID) Profile {
	if chatID == uuid.Nil {
		panic("chat ID must be non-nil, this is a developer error")
	}

	return Profile{
		Scopes: database.APIKeyScopes{
			database.ApiKeyScopeCoderWorkspacescreate,
			database.ApiKeyScopeCoderWorkspacesoperate,
			database.ApiKeyScopeCoderWorkspacesaccess,
			database.ApiKeyScopeChatRead,
			database.ApiKeyScopeChatUpdate,
			// The chat workspace tools read the owner's user record to
			// create workspaces on their behalf. This is a read, and the
			// owner's own roles still bound what is reachable.
			database.ApiKeyScopeUserRead,
		},
		AllowList: database.AllowList{
			{Type: rbac.ResourceChat.Type, ID: chatID.String()},
			{Type: rbac.ResourceWorkspace.Type, ID: policy.WildcardSymbol},
			{Type: rbac.ResourceTemplate.Type, ID: policy.WildcardSymbol},
			{Type: rbac.ResourceOrganizationMember.Type, ID: policy.WildcardSymbol},
			{Type: rbac.ResourceUser.Type, ID: policy.WildcardSymbol},
		},
		TokenName: "ai-chat-" + chatID.String(),
	}
}

// WorkspaceAgentIdentityProfile returns workspace-only permissions pinned to a
// single workspace. It intentionally contains no user-data scopes.
func WorkspaceAgentIdentityProfile(workspaceID uuid.UUID) Profile {
	if workspaceID == uuid.Nil {
		panic("workspace ID must be non-nil, this is a developer error")
	}

	return Profile{
		Scopes: database.APIKeyScopes{
			database.ApiKeyScopeWorkspaceRead,
			database.ApiKeyScopeWorkspaceUpdate,
			database.ApiKeyScopeWorkspaceStart,
			database.ApiKeyScopeWorkspaceStop,
			database.ApiKeyScopeWorkspaceSsh,
			database.ApiKeyScopeWorkspaceApplicationConnect,
		},
		AllowList: database.AllowList{
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID.String()},
		},
		TokenName: "ai-ws-" + workspaceID.String(),
	}
}

func validateProfile(profile Profile) (Profile, error) {
	if len(profile.Scopes) == 0 {
		return Profile{}, xerrors.New("AI agent profile must include at least one scope")
	}
	if len(profile.AllowList) == 0 {
		return Profile{}, xerrors.New("AI agent profile must include at least one allow-list entry")
	}

	for _, scope := range profile.Scopes {
		if !scope.Valid() {
			return Profile{}, xerrors.Errorf("invalid AI agent scope %q", scope)
		}
		switch scope {
		case database.ApiKeyScopeCoderAll,
			database.ApiKeyScopeCoderApplicationConnect,
			database.ApiKeyScopeCoderApikeysmanageSelf,
			database.ApiKeyScopeCoderTemplatesauthor:
			return Profile{}, xerrors.Errorf("AI agent scope %q is forbidden", scope)
		}

		resource, action, ok := rbac.ParseResourceAction(string(scope))
		if !ok {
			continue
		}
		switch resource {
		case rbac.ResourceApiKey.Type, rbac.ResourceUserSecret.Type, rbac.ResourceUserSkill.Type:
			// AI agents must never manage credentials, read user secrets,
			// or read user skills (no self-escalation, no secret exfil).
			return Profile{}, xerrors.Errorf("AI agent scope %q is forbidden", scope)
		case rbac.ResourceUser.Type:
			// Agents may read non-personal user fields (needed to resolve
			// their owner when acting on the owner's behalf) but never
			// personal/PII data (read_personal) or user mutations.
			if action != string(policy.ActionRead) {
				return Profile{}, xerrors.Errorf("AI agent scope %q is forbidden", scope)
			}
		case rbac.ResourceTemplate.Type:
			if action != string(policy.ActionRead) && action != string(policy.ActionUse) {
				return Profile{}, xerrors.Errorf("AI agent scope %q is forbidden", scope)
			}
		}
	}

	normalized := make(database.AllowList, 0, len(profile.AllowList))
	for _, entry := range profile.AllowList {
		validated, err := rbac.NewAllowListElement(entry.Type, entry.ID)
		if err != nil {
			return Profile{}, xerrors.Errorf("validate AI agent allow-list entry: %w", err)
		}
		if validated.Type == policy.WildcardSymbol && validated.ID == policy.WildcardSymbol {
			return Profile{}, xerrors.New("AI agent allow-list entries cannot grant every resource")
		}
		normalized = append(normalized, validated)
	}
	normalized, err := rbac.NormalizeAllowList(normalized)
	if err != nil {
		return Profile{}, xerrors.Errorf("normalize AI agent allow list: %w", err)
	}
	if len(normalized) == 0 {
		return Profile{}, xerrors.New("AI agent profile allow list cannot normalize to empty")
	}

	profile.AllowList = normalized
	return profile, nil
}
