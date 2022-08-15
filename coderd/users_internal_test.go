package coderd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"

	"github.com/stretchr/testify/require"
)

func TestSearchUsers(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name                  string
		Query                 string
		Expected              database.GetUsersParams
		ExpectedErrorContains string
	}{
		{
			Name:     "Empty",
			Query:    "",
			Expected: database.GetUsersParams{},
		},
		{
			Name:  "Username",
			Query: "user-name",
			Expected: database.GetUsersParams{
				Search:   "user-name",
				Status:   []database.UserStatus{},
				RbacRole: []string{},
			},
		},
		{
			Name:  "UsernameWithSpaces",
			Query: "   user-name    ",
			Expected: database.GetUsersParams{
				Search:   "user-name",
				Status:   []database.UserStatus{},
				RbacRole: []string{},
			},
		},
		{
			Name:  "Username+Param",
			Query: "usEr-name stAtus:actiVe",
			Expected: database.GetUsersParams{
				Search:   "user-name",
				Status:   []database.UserStatus{database.UserStatusActive},
				RbacRole: []string{},
			},
		},
		{
			Name:  "OnlyParams",
			Query: "status:acTIve sEArch:User-Name role:Owner",
			Expected: database.GetUsersParams{
				Search:   "user-name",
				Status:   []database.UserStatus{database.UserStatusActive},
				RbacRole: []string{rbac.RoleOwner()},
			},
		},
		{
			Name:  "QuotedParam",
			Query: `status:SuSpenDeD sEArch:"User Name" role:meMber`,
			Expected: database.GetUsersParams{
				Search:   "user name",
				Status:   []database.UserStatus{database.UserStatusSuspended},
				RbacRole: []string{rbac.RoleMember()},
			},
		},
		{
			Name:  "QuotedKey",
			Query: `"status":acTIve "sEArch":User-Name "role":Owner`,
			Expected: database.GetUsersParams{
				Search:   "user-name",
				Status:   []database.UserStatus{database.UserStatusActive},
				RbacRole: []string{rbac.RoleOwner()},
			},
		},
		{
			// This will not return an error
			Name:  "ExtraKeys",
			Query: `foo:bar`,
			Expected: database.GetUsersParams{
				Search:   "",
				Status:   []database.UserStatus{},
				RbacRole: []string{},
			},
		},
		{
			// Quotes keep elements together
			Name:  "QuotedSpecial",
			Query: `search:"user:name"`,
			Expected: database.GetUsersParams{
				Search:   "user:name",
				Status:   []database.UserStatus{},
				RbacRole: []string{},
			},
		},

		// Failures
		{
			Name:                  "ExtraColon",
			Query:                 `search:name:extra`,
			ExpectedErrorContains: "can only contain 1 ':'",
		},
		{
			Name:                  "InvalidStatus",
			Query:                 "status:inActive",
			ExpectedErrorContains: "status: Query param \"status\" has invalid value: \"inactive\" is not a valid user status\n",
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			values, errs := userSearchQuery(c.Query)
			if c.ExpectedErrorContains != "" {
				require.True(t, len(errs) > 0, "expect some errors")
				var s strings.Builder
				for _, err := range errs {
					_, _ = s.WriteString(fmt.Sprintf("%s: %s\n", err.Field, err.Detail))
				}
				require.Contains(t, s.String(), c.ExpectedErrorContains)
			} else {
				require.Len(t, errs, 0, "expected no error")
				require.Equal(t, c.Expected, values, "expected values")
			}
		})
	}
}
