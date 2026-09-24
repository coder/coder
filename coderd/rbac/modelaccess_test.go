package rbac_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func TestAuthorizeAIModelUse(t *testing.T) {
	t.Parallel()
	auth := rbac.NewStrictAuthorizer(prometheus.NewRegistry())
	orgID := uuid.New()
	config := rbac.ResourceChatModelConfig.WithID(uuid.New()).InOrg(orgID)
	for _, tc := range []struct {
		name    string
		role    rbac.RoleIdentifier
		configs []rbac.Object
		allowed bool
		load    bool
	}{
		{name: "OwnerSkipsLookup", role: rbac.RoleOwner(), allowed: true},
		{name: "MemberConfigured", role: rbac.RoleIdentifier{Name: rbac.RoleOrgMember(), OrganizationID: orgID}, configs: []rbac.Object{config}, allowed: true, load: true},
		{name: "MemberUnconfigured", role: rbac.RoleIdentifier{Name: rbac.RoleOrgMember(), OrganizationID: orgID}, load: true},
		{name: "Nonmember", role: rbac.RoleMember(), configs: []rbac.Object{config}, load: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			role, err := rbac.RoleByName(tc.role)
			if tc.role.Name == rbac.RoleOrgMember() {
				perms := rbac.OrgMemberPermissions(rbac.OrgSettings{})
				role = rbac.Role{Identifier: tc.role, ByOrgID: map[string]rbac.OrgPermissions{
					orgID.String(): {Org: perms.Org, Member: perms.Member},
				}}
				err = nil
			}
			require.NoError(t, err)
			subject := rbac.Subject{ID: uuid.NewString(), Roles: rbac.Roles{role}, Scope: rbac.ScopeAll}
			loaded := false
			allowed, err := rbac.AuthorizeAIModelUse(t.Context(), auth, subject, func(context.Context) ([]rbac.Object, error) {
				loaded = true
				return tc.configs, nil
			})
			require.Equal(t, tc.load, loaded)
			require.NoError(t, err)
			require.Equal(t, tc.allowed, allowed)
		})
	}
}

type modelAuthorizer struct {
	rbac.Authorizer
	authorize func(rbac.Object) error
}

func (a modelAuthorizer) Authorize(_ context.Context, _ rbac.Subject, _ policy.Action, obj rbac.Object) error {
	return a.authorize(obj)
}

func TestAuthorizeAIModelUseErrors(t *testing.T) {
	t.Parallel()
	failure := xerrors.New("evaluation failed")
	for _, stage := range []string{"Unrestricted", "Load", "Configured"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			loaded := false
			auth := modelAuthorizer{authorize: func(obj rbac.Object) error {
				if stage == "Unrestricted" || obj.Type == rbac.ResourceChatModelConfig.Type {
					return failure
				}
				return &rbac.UnauthorizedError{}
			}}
			allowed, err := rbac.AuthorizeAIModelUse(t.Context(), auth, rbac.Subject{}, func(context.Context) ([]rbac.Object, error) {
				loaded = true
				if stage == "Load" {
					return nil, failure
				}
				return []rbac.Object{rbac.ResourceChatModelConfig}, nil
			})
			require.ErrorIs(t, err, failure)
			require.False(t, allowed)
			require.Equal(t, stage != "Unrestricted", loaded)
		})
	}
}
