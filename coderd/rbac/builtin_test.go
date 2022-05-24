package rbac_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/rbac"
)

func TestIsOrgRole(t *testing.T) {
	t.Parallel()
	randomUUID := uuid.New()

	testCases := []struct {
		RoleName string
		OrgRole  bool
		OrgID    string
	}{
		// Not org roles
		{RoleName: rbac.RoleAdmin()},
		{RoleName: rbac.RoleMember()},
		{RoleName: "auditor"},

		{
			RoleName: "a:bad:role",
			OrgRole:  false,
		},
		{
			RoleName: "",
			OrgRole:  false,
		},

		// Org roles
		{
			RoleName: rbac.RoleOrgAdmin(randomUUID),
			OrgRole:  true,
			OrgID:    randomUUID.String(),
		},
		{
			RoleName: rbac.RoleOrgMember(randomUUID),
			OrgRole:  true,
			OrgID:    randomUUID.String(),
		},
		{
			RoleName: "test:example",
			OrgRole:  true,
			OrgID:    "example",
		},
	}

	// nolint:paralleltest
	for _, c := range testCases {
		t.Run(c.RoleName, func(t *testing.T) {
			orgID, ok := rbac.IsOrgRole(c.RoleName)
			require.Equal(t, c.OrgRole, ok, "match expected org role")
			require.Equal(t, c.OrgID, orgID, "match expected org id")
		})
	}
}

func TestListRoles(t *testing.T) {
	t.Parallel()

	siteRoles := rbac.SiteRoles()
	siteRoleNames := make([]string, 0, len(siteRoles))
	for _, role := range siteRoles {
		siteRoleNames = append(siteRoleNames, role.Name)
	}

	// If this test is ever failing, just update the list to the roles
	// expected from the builtin set.
	require.ElementsMatch(t, []string{
		"admin",
		"member",
		"auditor",
	},
		siteRoleNames)

	orgID := uuid.New()
	orgRoles := rbac.OrganizationRoles(orgID)
	orgRoleNames := make([]string, 0, len(orgRoles))
	for _, role := range orgRoles {
		orgRoleNames = append(orgRoleNames, role.Name)
	}

	require.ElementsMatch(t, []string{
		fmt.Sprintf("organization-admin:%s", orgID.String()),
		fmt.Sprintf("organization-member:%s", orgID.String()),
	},
		orgRoleNames)
}

func TestChangeSet(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name      string
		From      []string
		To        []string
		ExpAdd    []string
		ExpRemove []string
	}{
		{
			Name: "Empty",
		},
		{
			Name:      "Same",
			From:      []string{"a", "b", "c"},
			To:        []string{"a", "b", "c"},
			ExpAdd:    []string{},
			ExpRemove: []string{},
		},
		{
			Name:      "AllRemoved",
			From:      []string{"a", "b", "c"},
			ExpRemove: []string{"a", "b", "c"},
		},
		{
			Name:   "AllAdded",
			To:     []string{"a", "b", "c"},
			ExpAdd: []string{"a", "b", "c"},
		},
		{
			Name:      "AddAndRemove",
			From:      []string{"a", "b", "c"},
			To:        []string{"a", "b", "d", "e"},
			ExpAdd:    []string{"d", "e"},
			ExpRemove: []string{"c"},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			add, remove := rbac.ChangeRoleSet(c.From, c.To)
			require.ElementsMatch(t, c.ExpAdd, add, "expect added")
			require.ElementsMatch(t, c.ExpRemove, remove, "expect removed")
		})
	}
}
