package aiagentidentity

import (
	"slices"
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

var (
	chatAgentProfileScopes = database.APIKeyScopes{
		database.ApiKeyScopeCoderWorkspacescreate,
		database.ApiKeyScopeCoderWorkspacesoperate,
		database.ApiKeyScopeCoderWorkspacesaccess,
		database.ApiKeyScopeChatRead,
		database.ApiKeyScopeChatUpdate,
		database.ApiKeyScopeUserRead,
	}
	workspaceAgentProfileScopes = database.APIKeyScopes{
		database.ApiKeyScopeWorkspaceRead,
		database.ApiKeyScopeWorkspaceUpdate,
		database.ApiKeyScopeWorkspaceStart,
		database.ApiKeyScopeWorkspaceStop,
		database.ApiKeyScopeWorkspaceSsh,
		database.ApiKeyScopeWorkspaceApplicationConnect,
	}
	permittedProfileScopes      = newProfileScopeSet(chatAgentProfileScopes, workspaceAgentProfileScopes)
	permittedProfilePermissions = mustExpandProfilePermissions(permittedProfileScopes)
)

type profilePermission struct {
	resourceType string
	action       policy.Action
}

// ChatAgentProfile returns the platform and chat permissions needed by a chat
// AI agent identity. Workspace-related resources use typed wildcards because a
// newly created workspace does not have an ID that can be pinned in advance.
func ChatAgentProfile(chatID uuid.UUID) Profile {
	if chatID == uuid.Nil {
		panic("chat ID must be non-nil, this is a developer error")
	}

	return Profile{
		// The user read scope lets chat workspace tools read the owner's user
		// record when creating workspaces on their behalf. The owner's roles
		// still bound what is reachable.
		Scopes: slices.Clone(chatAgentProfileScopes),
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
		Scopes: slices.Clone(workspaceAgentProfileScopes),
		AllowList: database.AllowList{
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID.String()},
		},
		TokenName: "ai-ws-" + workspaceID.String(),
	}
}

// SandboxIdentityProfile is the scoped session key an AI sandbox's child
// agent uses for in-workspace CLI actions. It carries the same
// workspace-pinned ceiling as the workspace profile, but is named per
// sandbox so it can be rotated and revoked with that sandbox's lifecycle
// without disturbing the enclosing workspace's key.
func SandboxIdentityProfile(workspaceID, sandboxID uuid.UUID) Profile {
	if workspaceID == uuid.Nil || sandboxID == uuid.Nil {
		panic("workspace and sandbox IDs must be non-nil, this is a developer error")
	}

	profile := WorkspaceAgentIdentityProfile(workspaceID)
	profile.TokenName = "ai-sb-" + sandboxID.String()
	return profile
}

func validateProfile(profile Profile) (Profile, error) {
	if len(profile.Scopes) == 0 {
		return Profile{}, xerrors.New("AI agent profile must include at least one scope")
	}
	if len(profile.AllowList) == 0 {
		return Profile{}, xerrors.New("AI agent profile must include at least one allow-list entry")
	}

	for _, scope := range profile.Scopes {
		if err := validateProfileScope(scope); err != nil {
			return Profile{}, err
		}
	}

	normalized := make(database.AllowList, 0, len(profile.AllowList))
	for _, entry := range profile.AllowList {
		validated, err := rbac.NewAllowListElement(entry.Type, entry.ID)
		if err != nil {
			return Profile{}, xerrors.Errorf("validate AI agent allow-list entry %q: %w", entry.String(), err)
		}
		if validated.Type == policy.WildcardSymbol && validated.ID == policy.WildcardSymbol {
			return Profile{}, xerrors.Errorf("AI agent allow-list entry %q cannot grant every resource", entry.String())
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

func validateProfileScope(scope database.APIKeyScope) error {
	if !scope.Valid() {
		return xerrors.Errorf("invalid AI agent scope %q", scope)
	}

	// These high-level scopes are never valid for an AI agent, even when their
	// expanded permissions overlap with the permitted profile permissions.
	switch scope {
	case database.ApiKeyScopeCoderAll,
		database.ApiKeyScopeCoderApplicationConnect,
		database.ApiKeyScopeCoderApikeysmanageSelf,
		database.ApiKeyScopeCoderTemplatesauthor:
		return xerrors.Errorf("AI agent scope %q is forbidden", scope)
	}

	permissions, err := expandProfileScopePermissions(scope)
	if err != nil {
		return xerrors.Errorf("expand AI agent scope %q: %w", scope, err)
	}
	for _, permission := range permissions {
		if forbiddenProfilePermission(permission) {
			return xerrors.Errorf(
				"AI agent scope %q expands to forbidden permission %q",
				scope,
				permission.resourceType+":"+string(permission.action),
			)
		}
		if _, ok := permittedProfilePermissions[permission]; !ok {
			return xerrors.Errorf(
				"AI agent scope %q expands to permission %q, which is not permitted",
				scope,
				permission.resourceType+":"+string(permission.action),
			)
		}
	}

	if _, ok := permittedProfileScopes[scope]; !ok {
		return xerrors.Errorf("AI agent scope %q is not permitted", scope)
	}
	return nil
}

func forbiddenProfilePermission(permission profilePermission) bool {
	switch permission.resourceType {
	case rbac.ResourceApiKey.Type, rbac.ResourceUserSecret.Type, rbac.ResourceUserSkill.Type:
		// AI agents must never manage credentials, read user secrets, or read
		// user skills.
		return true
	case rbac.ResourceUser.Type:
		// Agents may read non-personal user fields, but never personal data or
		// user mutations.
		return permission.action != policy.ActionRead
	case rbac.ResourceTemplate.Type:
		return permission.action != policy.ActionRead && permission.action != policy.ActionUse
	default:
		return false
	}
}

func newProfileScopeSet(scopeGroups ...database.APIKeyScopes) map[database.APIKeyScope]struct{} {
	scopes := make(map[database.APIKeyScope]struct{})
	for _, group := range scopeGroups {
		for _, scope := range group {
			scopes[scope] = struct{}{}
		}
	}
	return scopes
}

func mustExpandProfilePermissions(scopes map[database.APIKeyScope]struct{}) map[profilePermission]struct{} {
	permissions := make(map[profilePermission]struct{})
	for scope := range scopes {
		expanded, err := expandProfileScopePermissions(scope)
		if err != nil {
			panic(xerrors.Errorf("expand permitted AI agent scope %q: %w", scope, err))
		}
		for _, permission := range expanded {
			permissions[permission] = struct{}{}
		}
	}
	return permissions
}

func expandProfileScopePermissions(scope database.APIKeyScope) ([]profilePermission, error) {
	expanded, err := scope.ToRBAC().Expand()
	if err != nil {
		return nil, err
	}

	permissionCount := len(expanded.Site) + len(expanded.User)
	for _, organization := range expanded.ByOrgID {
		permissionCount += len(organization.Org) + len(organization.Member)
	}
	permissions := make([]profilePermission, 0, permissionCount)
	appendPermissions := func(entries []rbac.Permission) {
		for _, entry := range entries {
			permissions = append(permissions, profilePermission{
				resourceType: entry.ResourceType,
				action:       entry.Action,
			})
		}
	}
	appendPermissions(expanded.Site)
	appendPermissions(expanded.User)
	for _, organization := range expanded.ByOrgID {
		appendPermissions(organization.Org)
		appendPermissions(organization.Member)
	}
	return permissions, nil
}

// APIKeyMatchesBuiltInProfile reports whether an API key's scope set and
// allow-list shape match a built-in AI agent profile. Authentication middleware
// should apply this check to keys owned by AI agent users as defense in depth.
func APIKeyMatchesBuiltInProfile(key database.APIKey) bool {
	switch {
	case sameProfileScopes(key.Scopes, chatAgentProfileScopes):
		return matchesChatProfileAllowList(key.AllowList)
	case sameProfileScopes(key.Scopes, workspaceAgentProfileScopes):
		return matchesWorkspaceProfileAllowList(key.AllowList)
	default:
		return false
	}
}

func sameProfileScopes(actual, expected database.APIKeyScopes) bool {
	if len(actual) != len(expected) {
		return false
	}

	expectedSet := make(map[database.APIKeyScope]struct{}, len(expected))
	for _, scope := range expected {
		expectedSet[scope] = struct{}{}
	}
	seen := make(map[database.APIKeyScope]struct{}, len(actual))
	for _, scope := range actual {
		if _, ok := expectedSet[scope]; !ok {
			return false
		}
		if _, duplicate := seen[scope]; duplicate {
			return false
		}
		seen[scope] = struct{}{}
	}
	return true
}

func matchesChatProfileAllowList(allowList database.AllowList) bool {
	if len(allowList) != 5 {
		return false
	}

	wildcardTypes := map[string]struct{}{
		rbac.ResourceWorkspace.Type:          {},
		rbac.ResourceTemplate.Type:           {},
		rbac.ResourceOrganizationMember.Type: {},
		rbac.ResourceUser.Type:               {},
	}
	seen := make(map[string]struct{}, len(allowList))
	for _, entry := range allowList {
		if _, duplicate := seen[entry.Type]; duplicate {
			return false
		}
		seen[entry.Type] = struct{}{}

		if entry.Type == rbac.ResourceChat.Type {
			if !validProfileResourceID(entry.ID) {
				return false
			}
			continue
		}
		if _, ok := wildcardTypes[entry.Type]; !ok || entry.ID != policy.WildcardSymbol {
			return false
		}
	}
	return len(seen) == len(wildcardTypes)+1
}

func matchesWorkspaceProfileAllowList(allowList database.AllowList) bool {
	return len(allowList) == 1 &&
		allowList[0].Type == rbac.ResourceWorkspace.Type &&
		validProfileResourceID(allowList[0].ID)
}

func validProfileResourceID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil
}
