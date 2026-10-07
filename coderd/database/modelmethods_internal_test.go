package database

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func TestAPIKeyScopesExpand(t *testing.T) {
	t.Parallel()
	t.Run("builtins", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name   string
			scopes APIKeyScopes
			want   func(t *testing.T, s rbac.Scope)
		}{
			{
				name:   "all",
				scopes: APIKeyScopes{ApiKeyScopeCoderAll},
				want: func(t *testing.T, s rbac.Scope) {
					requirePermission(t, s, rbac.ResourceWildcard.Type, policy.Action(policy.WildcardSymbol))
					requireAllowAll(t, s)
				},
			},
			{
				name:   "application_connect",
				scopes: APIKeyScopes{ApiKeyScopeCoderApplicationConnect},
				want: func(t *testing.T, s rbac.Scope) {
					requirePermission(t, s, rbac.ResourceWorkspace.Type, policy.ActionApplicationConnect)
					requireAllowAll(t, s)
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				s, err := tc.scopes.expandRBACScope()
				require.NoError(t, err)
				tc.want(t, s)
			})
		}
	})

	t.Run("low_level_pairs", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name   string
			scopes APIKeyScopes
			res    string
			act    policy.Action
		}{
			{name: "workspace:read", scopes: APIKeyScopes{ApiKeyScopeWorkspaceRead}, res: rbac.ResourceWorkspace.Type, act: policy.ActionRead},
			{name: "template:use", scopes: APIKeyScopes{ApiKeyScopeTemplateUse}, res: rbac.ResourceTemplate.Type, act: policy.ActionUse},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				s, err := tc.scopes.expandRBACScope()
				require.NoError(t, err)
				requirePermission(t, s, tc.res, tc.act)
				requireAllowAll(t, s)
			})
		}
	})

	t.Run("merge", func(t *testing.T) {
		t.Parallel()
		scopes := APIKeyScopes{ApiKeyScopeCoderApplicationConnect, ApiKeyScopeCoderAll, ApiKeyScopeWorkspaceRead}
		s, err := scopes.expandRBACScope()
		require.NoError(t, err)
		requirePermission(t, s, rbac.ResourceWildcard.Type, policy.Action(policy.WildcardSymbol))
		requirePermission(t, s, rbac.ResourceWorkspace.Type, policy.ActionApplicationConnect)
		requirePermission(t, s, rbac.ResourceWorkspace.Type, policy.ActionRead)
		requireAllowAll(t, s)
	})

	t.Run("effective_scope_keep_types", func(t *testing.T) {
		t.Parallel()
		workspaceID := uuid.New()

		effective := APIKeyScopeSet{
			Scopes: APIKeyScopes{ApiKeyScopeWorkspaceRead},
			AllowList: AllowList{
				{Type: rbac.ResourceWorkspace.Type, ID: workspaceID.String()},
			},
		}

		expanded, err := effective.Expand()
		require.NoError(t, err)
		require.Len(t, expanded.AllowIDList, 1)
		require.Equal(t, "workspace", expanded.AllowIDList[0].Type)
		require.Equal(t, workspaceID.String(), expanded.AllowIDList[0].ID)
	})

	t.Run("empty_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := (APIKeyScopes{}).expandRBACScope()
		require.Error(t, err)
		require.ErrorContains(t, err, "no scopes provided")
	})

	t.Run("allow_list_overrides", func(t *testing.T) {
		t.Parallel()
		allowID := uuid.NewString()
		set := APIKeyScopes{ApiKeyScopeWorkspaceRead}.WithAllowList(AllowList{
			{Type: rbac.ResourceWorkspace.Type, ID: allowID},
		})
		s, err := set.Expand()
		require.NoError(t, err)
		require.Len(t, s.AllowIDList, 1)
		require.Equal(t, rbac.AllowListElement{Type: rbac.ResourceWorkspace.Type, ID: allowID}, s.AllowIDList[0])
	})

	t.Run("allow_list_wildcard_keeps_merged", func(t *testing.T) {
		t.Parallel()
		set := APIKeyScopes{ApiKeyScopeWorkspaceRead}.WithAllowList(AllowList{
			{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol},
		})
		s, err := set.Expand()
		require.NoError(t, err)
		requirePermission(t, s, rbac.ResourceWorkspace.Type, policy.ActionRead)
		requireAllowAll(t, s)
	})

	t.Run("scope_set_helper", func(t *testing.T) {
		t.Parallel()
		allowID := uuid.NewString()
		key := APIKey{
			Scopes: APIKeyScopes{ApiKeyScopeWorkspaceRead},
			AllowList: AllowList{
				{Type: rbac.ResourceWorkspace.Type, ID: allowID},
			},
		}
		s, err := key.ScopeSet().Expand()
		require.NoError(t, err)
		require.Len(t, s.AllowIDList, 1)
		require.Equal(t, rbac.AllowListElement{Type: rbac.ResourceWorkspace.Type, ID: allowID}, s.AllowIDList[0])
	})
}

//nolint:tparallel,paralleltest
func TestChatACLDisabled(t *testing.T) {
	uid := uuid.NewString()
	gid := uuid.NewString()

	chat := Chat{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		OwnerID:        uuid.New(),
		UserACL: ChatACL{
			uid: ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}},
		},
		GroupACL: ChatACL{
			gid: ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}},
		},
	}

	t.Run("ACLsOmittedWhenDisabled", func(t *testing.T) {
		rbac.SetChatACLDisabled(true)
		t.Cleanup(func() { rbac.SetChatACLDisabled(false) })

		obj := chat.RBACObject()

		require.Empty(t, obj.ACLUserList, "user ACLs should be empty when disabled")
		require.Empty(t, obj.ACLGroupList, "group ACLs should be empty when disabled")
	})

	t.Run("ACLsIncludedWhenEnabled", func(t *testing.T) {
		rbac.SetChatACLDisabled(false)

		obj := chat.RBACObject()

		require.NotEmpty(t, obj.ACLUserList, "user ACLs should be present when enabled")
		require.NotEmpty(t, obj.ACLGroupList, "group ACLs should be present when enabled")
		require.Contains(t, obj.ACLUserList, uid)
		require.Contains(t, obj.ACLGroupList, gid)
	})
}

//nolint:tparallel,paralleltest // It toggles the global chat ACL flag.
func TestChatProjectRBACObjects(t *testing.T) {
	readUser, updateUser, wildcardUser, noReadUser, updateOnlyUser := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	readGroup, updateGroup := uuid.NewString(), uuid.NewString()
	project := ChatProject{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		OwnerID:        uuid.New(),
		UserACL: ChatACL{
			readUser:     {Permissions: []policy.Action{policy.ActionRead}},
			updateUser:   {Permissions: []policy.Action{policy.ActionRead, policy.ActionUpdate, policy.ActionShare}},
			wildcardUser: {Permissions: []policy.Action{policy.WildcardSymbol}},
			noReadUser:   {Permissions: []policy.Action{policy.ActionShare}},
			// RBAC denies reading the project without read, so its memories
			// get nothing either.
			updateOnlyUser: {Permissions: []policy.Action{policy.ActionUpdate}},
		},
		GroupACL: ChatACL{
			readGroup:   {Permissions: []policy.Action{policy.ActionRead}},
			updateGroup: {Permissions: []policy.Action{policy.ActionRead, policy.ActionUpdate}},
		},
	}
	readWrite := []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionDelete}

	t.Run("Enabled", func(t *testing.T) {
		rbac.SetChatACLDisabled(false)

		obj := project.RBACObject()
		require.Equal(t, project.UserACL.RBACACL(), obj.ACLUserList)
		require.Equal(t, project.GroupACL.RBACACL(), obj.ACLGroupList)

		// Read sharees only read memories directly; updaters also write them.
		memory := ChatProjectMemoryRBACObject(project)
		require.Equal(t, map[string][]policy.Action{
			readUser:     {policy.ActionRead},
			updateUser:   readWrite,
			wildcardUser: readWrite,
		}, memory.ACLUserList)
		require.Equal(t, map[string][]policy.Action{
			readGroup:   {policy.ActionRead},
			updateGroup: readWrite,
		}, memory.ACLGroupList)

		memoryID := uuid.New()
		withID := ChatProjectMemory{ID: memoryID}.RBACObject(project)
		require.Equal(t, memoryID.String(), withID.ID)
		require.Equal(t, memory.ACLUserList, withID.ACLUserList)

		auth := rbac.NewStrictAuthorizer(prometheus.NewRegistry())
		orgRole := rbac.Role{
			Identifier: rbac.ScopedRoleOrgMember(project.OrganizationID),
			ByOrgID:    map[string]rbac.OrgPermissions{project.OrganizationID.String(): {}},
		}
		subject := rbac.Subject{ID: readUser, Roles: rbac.Roles{orgRole}, Scope: rbac.ScopeAll}
		ctx := context.Background()
		require.NoError(t, auth.Authorize(ctx, subject, policy.ActionRead, obj))
		require.NoError(t, auth.Authorize(ctx, subject, policy.ActionRead, memory))
		require.Error(t, auth.Authorize(ctx, subject, policy.ActionCreate, memory))
		require.Error(t, auth.Authorize(ctx, subject, policy.ActionDelete, withID))
		require.Error(t, auth.Authorize(ctx, subject, policy.ActionUpdate, obj))
	})

	t.Run("Disabled", func(t *testing.T) {
		rbac.SetChatACLDisabled(true)
		t.Cleanup(func() { rbac.SetChatACLDisabled(false) })

		obj := project.RBACObject()
		require.Empty(t, obj.ACLUserList)
		require.Empty(t, obj.ACLGroupList)
		memory := ChatProjectMemoryRBACObject(project)
		require.Empty(t, memory.ACLUserList)
		require.Empty(t, memory.ACLGroupList)
	})
}

//nolint:tparallel,paralleltest
func TestWorkspaceACLDisabled(t *testing.T) {
	uid := uuid.NewString()
	gid := uuid.NewString()

	ws := WorkspaceTable{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		OwnerID:        uuid.New(),
		UserACL: WorkspaceACL{
			uid: WorkspaceACLEntry{Permissions: []policy.Action{policy.ActionSSH}},
		},
		GroupACL: WorkspaceACL{
			gid: WorkspaceACLEntry{Permissions: []policy.Action{policy.ActionSSH}},
		},
	}

	t.Run("ACLsOmittedWhenDisabled", func(t *testing.T) {
		rbac.SetWorkspaceACLDisabled(true)
		t.Cleanup(func() { rbac.SetWorkspaceACLDisabled(false) })

		obj := ws.RBACObject()

		require.Empty(t, obj.ACLUserList, "user ACLs should be empty when disabled")
		require.Empty(t, obj.ACLGroupList, "group ACLs should be empty when disabled")
	})

	t.Run("ACLsIncludedWhenEnabled", func(t *testing.T) {
		rbac.SetWorkspaceACLDisabled(false)

		obj := ws.RBACObject()

		require.NotEmpty(t, obj.ACLUserList, "user ACLs should be present when enabled")
		require.NotEmpty(t, obj.ACLGroupList, "group ACLs should be present when enabled")
		require.Contains(t, obj.ACLUserList, uid)
		require.Contains(t, obj.ACLGroupList, gid)
	})
}

// TestOAuth2ProviderAppIsPublic pins IsPublic's contract directly, since it is
// what decides whether the token endpoint validates a client secret at all.
// Only the exact string "public" may read as public: anything else, including
// an unset column or a differently-cased value, must read as confidential so
// that a garbled value cannot silently skip client authentication.
func TestOAuth2ProviderAppIsPublic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		clientType string
		want       bool
	}{
		{name: "Public", clientType: "public", want: true},
		{name: "Confidential", clientType: "confidential", want: false},
		{name: "Empty", clientType: "", want: false},
		{name: "MixedCasePublic", clientType: "Public", want: false},
		{name: "AllCapsPublic", clientType: "PUBLIC", want: false},
		{name: "LeadingSpace", clientType: " public", want: false},
		{name: "TrailingSpace", clientType: "public ", want: false},
		{name: "Bogus", clientType: "bogus", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := OAuth2ProviderApp{ClientType: tt.clientType}
			require.Equal(t, tt.want, app.IsPublic())
		})
	}
}

// Helpers
func requirePermission(t *testing.T, s rbac.Scope, resource string, action policy.Action) {
	t.Helper()
	for _, p := range s.Site {
		if p.ResourceType == resource && p.Action == action {
			return
		}
	}
	t.Fatalf("permission not found: %s:%s", resource, action)
}

func requireAllowAll(t *testing.T, s rbac.Scope) {
	t.Helper()
	require.Len(t, s.AllowIDList, 1)
	require.Equal(t, policy.WildcardSymbol, s.AllowIDList[0].ID)
	require.Equal(t, policy.WildcardSymbol, s.AllowIDList[0].Type)
}
