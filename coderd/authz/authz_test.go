package authz_test

import (
	"context"
	"testing"

	"github.com/coder/coder/coderd/authz"
	"github.com/coder/coder/coderd/authz/rbac"
	"github.com/stretchr/testify/require"
)

func TestAuthorize(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name          string
		Subject       authz.Subject
		Resource      authz.Object
		Action        rbac.Operation
		ExpectedError bool
	}{
		{
			Name: "Org admin trying to read all workspace",
			Subject: &authz.SubjectTODO{
				UserID: "org-admin",
				Site:   []rbac.Role{authz.SiteMember},
				Org: map[string]rbac.Roles{
					"default": {authz.OrganizationAdmin},
				},
			},
			Resource: authz.Object{
				ObjectType: authz.Workspaces,
			},
			Action:        authz.ReadAll,
			ExpectedError: true,
		},
		{
			Name: "Org admin read org workspace",
			Subject: &authz.SubjectTODO{
				UserID: "org-admin",
				Site:   []rbac.Role{authz.SiteMember},
				Org: map[string]rbac.Roles{
					"default": {authz.OrganizationAdmin},
				},
			},
			Resource: authz.Object{
				ObjectType: authz.Workspaces,
				OrgOwner:   "default",
			},
			Action: authz.ReadAll,
		},
		{
			Name: "Org admin read someone else's workspace",
			Subject: &authz.SubjectTODO{
				UserID: "org-admin",
				Site:   []rbac.Role{authz.SiteMember},
				Org: map[string]rbac.Roles{
					"default": {authz.OrganizationAdmin},
				},
			},
			Resource: authz.Object{
				Owner:      "org-member",
				ObjectType: authz.Workspaces,
				OrgOwner:   "default",
			},
			Action: authz.ReadOwn,
		},
		{
			Name: "Org member read their workspace",
			Subject: &authz.SubjectTODO{
				UserID: "org-member",
				Site:   []rbac.Role{authz.SiteMember},
				Org: map[string]rbac.Roles{
					"default": {authz.OrganizationMember},
				},
			},
			Resource: authz.Object{
				Owner:      "org-member",
				ObjectType: authz.Workspaces,
				OrgOwner:   "default",
			},
			Action: authz.ReadOwn,
		},
		{
			Name: "Site member read their workspace in other org",
			Subject: &authz.SubjectTODO{
				UserID: "site-member",
				Site:   []rbac.Role{authz.SiteMember},
				Org: map[string]rbac.Roles{
					"default": {authz.OrganizationMember},
				},
			},
			Resource: authz.Object{
				Owner:      "site-member",
				ObjectType: authz.Workspaces,
				OrgOwner:   "not-default",
			},
			Action:        authz.ReadOwn,
			ExpectedError: true,
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			err := authz.Authorize(context.Background(), c.Subject, c.Action, c.Resource)
			if c.ExpectedError {
				require.EqualError(t, err, authz.ErrUnauthorized.Error(), "unauth")
			} else {
				require.NoError(t, err, "exp auth succeed")
			}
		})
	}
}
