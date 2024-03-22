package searchquery_test

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

func TestSearchWorkspace(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name                  string
		Query                 string
		Expected              database.GetWorkspacesParams
		ExpectedErrorContains string
	}{
		{
			Name:     "Empty",
			Query:    "",
			Expected: database.GetWorkspacesParams{},
		},
		{
			Name:  "Owner/Name",
			Query: "Foo/Bar",
			Expected: database.GetWorkspacesParams{
				OwnerUsername: "foo",
				Name:          "bar",
			},
		},
		{
			Name:  "Owner/NameWithSpaces",
			Query: "     Foo/Bar     ",
			Expected: database.GetWorkspacesParams{
				OwnerUsername: "foo",
				Name:          "bar",
			},
		},
		{
			Name:  "Name",
			Query: "workspace-name",
			Expected: database.GetWorkspacesParams{
				Name: "workspace-name",
			},
		},
		{
			Name:  "Name+Param",
			Query: "workspace-name TEMPLATE:docker",
			Expected: database.GetWorkspacesParams{
				Name:         "workspace-name",
				TemplateName: "docker",
			},
		},
		{
			Name:  "OnlyParams",
			Query: "name:workspace-name template:docker OWNER:Alice",
			Expected: database.GetWorkspacesParams{
				Name:          "workspace-name",
				TemplateName:  "docker",
				OwnerUsername: "alice",
			},
		},
		{
			Name:  "QuotedParam",
			Query: `name:workspace-name template:"docker template" owner:alice`,
			Expected: database.GetWorkspacesParams{
				Name:          "workspace-name",
				TemplateName:  "docker template",
				OwnerUsername: "alice",
			},
		},
		{
			Name:  "QuotedKey",
			Query: `"name":baz "template":foo "owner":bar`,
			Expected: database.GetWorkspacesParams{
				Name:          "baz",
				TemplateName:  "foo",
				OwnerUsername: "bar",
			},
		},
		{
			// Quotes keep elements together
			Name:  "QuotedSpecial",
			Query: `name:"workspace:name"`,
			Expected: database.GetWorkspacesParams{
				Name: "workspace:name",
			},
		},
		{
			Name:  "QuotedMadness",
			Query: `"name":"foo:bar:baz/baz/zoo:zonk"`,
			Expected: database.GetWorkspacesParams{
				Name: "foo:bar:baz/baz/zoo:zonk",
			},
		},
		{
			Name:  "QuotedName",
			Query: `"foo/bar"`,
			Expected: database.GetWorkspacesParams{
				Name: "foo/bar",
			},
		},
		{
			Name:  "QuotedOwner/Name",
			Query: `"foo"/"bar"`,
			Expected: database.GetWorkspacesParams{
				Name:          "bar",
				OwnerUsername: "foo",
			},
		},
		{
			Name:  "Outdated",
			Query: `outdated:true`,
			Expected: database.GetWorkspacesParams{
				UsingActive: sql.NullBool{
					Bool:  false,
					Valid: true,
				},
			},
		},
		{
			Name:  "Updated",
			Query: `outdated:false`,
			Expected: database.GetWorkspacesParams{
				UsingActive: sql.NullBool{
					Bool:  true,
					Valid: true,
				},
			},
		},
		{
			Name:  "ParamName",
			Query: "param:foo",
			Expected: database.GetWorkspacesParams{
				HasParam: []string{"foo"},
			},
		},
		{
			Name:  "MultipleParamNames",
			Query: "param:foo param:bar param:baz",
			Expected: database.GetWorkspacesParams{
				HasParam: []string{"foo", "bar", "baz"},
			},
		},
		{
			Name:  "ParamValue",
			Query: "param:foo=bar",
			Expected: database.GetWorkspacesParams{
				ParamNames:  []string{"foo"},
				ParamValues: []string{"bar"},
			},
		},
		{
			Name:  "QuotedParamValue",
			Query: `param:"image=ghcr.io/coder/coder-preview:main"`,
			Expected: database.GetWorkspacesParams{
				ParamNames:  []string{"image"},
				ParamValues: []string{"ghcr.io/coder/coder-preview:main"},
			},
		},
		{
			Name:  "MultipleParamValues",
			Query: "param:foo=bar param:fuzz=buzz",
			Expected: database.GetWorkspacesParams{
				ParamNames:  []string{"foo", "fuzz"},
				ParamValues: []string{"bar", "buzz"},
			},
		},
		{
			Name:  "MixedParams",
			Query: "param:dot    param:foo=bar param:fuzz=buzz param:tot",
			Expected: database.GetWorkspacesParams{
				HasParam:    []string{"dot", "tot"},
				ParamNames:  []string{"foo", "fuzz"},
				ParamValues: []string{"bar", "buzz"},
			},
		},
		{
			Name:  "ParamSpaces",
			Query: `param:"   dot "     param:"   foo=bar   "`,
			Expected: database.GetWorkspacesParams{
				HasParam:    []string{"dot"},
				ParamNames:  []string{"foo"},
				ParamValues: []string{"bar"},
			},
		},

		// Failures
		{
			Name:                  "ParamExcessValue",
			Query:                 "param:foo=bar=baz",
			ExpectedErrorContains: "can only contain 1 '='",
		},
		{
			Name:                  "ParamNoValue",
			Query:                 "param:foo=",
			ExpectedErrorContains: "omit the '=' to match",
		},
		{
			Name:                  "NoPrefix",
			Query:                 `:foo`,
			ExpectedErrorContains: "cannot start or end",
		},
		{
			Name:                  "Double",
			Query:                 `name:foo name:bar`,
			ExpectedErrorContains: "provided more than once",
		},
		{
			Name:                  "ExtraSlashes",
			Query:                 `foo/bar/baz`,
			ExpectedErrorContains: "can only contain 1 '/'",
		},
		{
			Name:                  "ExtraColon",
			Query:                 `owner:name:extra`,
			ExpectedErrorContains: "can only contain 1 ':'",
		},
		{
			Name:                  "ExtraKeys",
			Query:                 `foo:bar`,
			ExpectedErrorContains: `"foo" is not a valid query param`,
		},
		{
			Name:                  "ParamExtraColons",
			Query:                 "param:foo:value",
			ExpectedErrorContains: "can only contain 1 ':'",
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			values, errs := searchquery.Workspaces(c.Query, codersdk.Pagination{}, 0)
			if c.ExpectedErrorContains != "" {
				assert.True(t, len(errs) > 0, "expect some errors")
				var s strings.Builder
				for _, err := range errs {
					_, _ = s.WriteString(fmt.Sprintf("%s: %s\n", err.Field, err.Detail))
				}
				assert.Contains(t, s.String(), c.ExpectedErrorContains)
			} else {
				if len(c.Expected.WorkspaceIds) == len(values.WorkspaceIds) {
					// nil slice vs 0 len slice is equivalent for our purposes.
					c.Expected.WorkspaceIds = values.WorkspaceIds
				}
				if len(c.Expected.HasParam) == len(values.HasParam) {
					// nil slice vs 0 len slice is equivalent for our purposes.
					c.Expected.HasParam = values.HasParam
				}
				assert.Len(t, errs, 0, "expected no error")
				assert.Equal(t, c.Expected, values, "expected values")
			}
		})
	}
	t.Run("AgentInactiveDisconnectTimeout", func(t *testing.T) {
		t.Parallel()

		query := ``
		timeout := 1337 * time.Second
		values, errs := searchquery.Workspaces(query, codersdk.Pagination{}, timeout)
		require.Empty(t, errs)
		require.Equal(t, int64(timeout.Seconds()), values.AgentInactiveDisconnectTimeoutSeconds)
	})
}

func TestSearchAudit(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name                  string
		Query                 string
		Expected              database.GetAuditLogsOffsetParams
		ExpectedErrorContains string
	}{
		{
			Name:     "Empty",
			Query:    "",
			Expected: database.GetAuditLogsOffsetParams{},
		},
		// Failures
		{
			Name:                  "ExtraColon",
			Query:                 `search:name:extra`,
			ExpectedErrorContains: "can only contain 1 ':'",
		},
		{
			Name:                  "ExtraKeys",
			Query:                 `foo:bar`,
			ExpectedErrorContains: `"foo" is not a valid query param`,
		},
		{
			Name:                  "Dates",
			Query:                 "date_from:2006",
			ExpectedErrorContains: "valid date format",
		},
		{
			Name:  "ResourceTarget",
			Query: "resource_target:foo",
			Expected: database.GetAuditLogsOffsetParams{
				ResourceTarget: "foo",
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			values, errs := searchquery.AuditLogs(c.Query)
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

func TestSearchUsers(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name                  string
		Query                 string
		Expected              database.GetUsersParams
		ExpectedErrorContains string
	}{
		{
			Name:  "Empty",
			Query: "",
			Expected: database.GetUsersParams{
				Status:   []database.UserStatus{},
				RbacRole: []string{},
			},
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
			ExpectedErrorContains: "has invalid values",
		},
		{
			Name:                  "ExtraKeys",
			Query:                 `foo:bar`,
			ExpectedErrorContains: `"foo" is not a valid query param`,
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			values, errs := searchquery.Users(c.Query)
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
