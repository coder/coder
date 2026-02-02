package searchquery_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
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
		Setup                 func(t *testing.T, db database.Store)
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
		{
			Name:  "Organization",
			Query: `organization:4fe722f0-49bc-4a90-a3eb-4ac439bfce20`,
			Setup: func(t *testing.T, db database.Store) {
				dbgen.Organization(t, db, database.Organization{
					ID: uuid.MustParse("4fe722f0-49bc-4a90-a3eb-4ac439bfce20"),
				})
			},
			Expected: database.GetWorkspacesParams{
				OrganizationID: uuid.MustParse("4fe722f0-49bc-4a90-a3eb-4ac439bfce20"),
			},
		},
		{
			Name:  "OrganizationByName",
			Query: `organization:foobar`,
			Setup: func(t *testing.T, db database.Store) {
				dbgen.Organization(t, db, database.Organization{
					ID:   uuid.MustParse("08eb6715-02f8-45c5-b86d-03786fcfbb4e"),
					Name: "foobar",
				})
			},
			Expected: database.GetWorkspacesParams{
				OrganizationID: uuid.MustParse("08eb6715-02f8-45c5-b86d-03786fcfbb4e"),
			},
		},
		{
			Name:  "HasAITaskTrue",
			Query: "has-ai-task:true",
			Expected: database.GetWorkspacesParams{
				HasAITask: sql.NullBool{
					Bool:  true,
					Valid: true,
				},
			},
		},
		{
			Name:  "HasAITaskFalse",
			Query: "has-ai-task:false",
			Expected: database.GetWorkspacesParams{
				HasAITask: sql.NullBool{
					Bool:  false,
					Valid: true,
				},
			},
		},
		{
			Name:  "HasAITaskMissing",
			Query: "",
			Expected: database.GetWorkspacesParams{
				HasAITask: sql.NullBool{
					Bool:  false,
					Valid: false,
				},
			},
		},
		{
			Name:  "HasExternalAgentTrue",
			Query: "has_external_agent:true",
			Expected: database.GetWorkspacesParams{
				HasExternalAgent: sql.NullBool{
					Bool:  true,
					Valid: true,
				},
			},
		},
		{
			Name:  "HasExternalAgentFalse",
			Query: "has_external_agent:false",
			Expected: database.GetWorkspacesParams{
				HasExternalAgent: sql.NullBool{
					Bool:  false,
					Valid: true,
				},
			},
		},
		{
			Name:  "HasExternalAgentMissing",
			Query: "",
			Expected: database.GetWorkspacesParams{
				HasExternalAgent: sql.NullBool{
					Bool:  false,
					Valid: false,
				},
			},
		},
		{
			Name:  "SharedTrue",
			Query: "shared:true",
			Expected: database.GetWorkspacesParams{
				Shared: sql.NullBool{
					Bool:  true,
					Valid: true,
				},
			},
		},
		{
			Name:  "SharedFalse",
			Query: "shared:false",
			Expected: database.GetWorkspacesParams{
				Shared: sql.NullBool{
					Bool:  false,
					Valid: true,
				},
			},
		},
		{
			Name:  "SharedMissing",
			Query: "",
			Expected: database.GetWorkspacesParams{
				Shared: sql.NullBool{
					Bool:  false,
					Valid: false,
				},
			},
		},
		{
			Name:  "HealthyTrue",
			Query: "healthy:true",
			Expected: database.GetWorkspacesParams{
				HasAgentStatuses: []string{"connected"},
			},
		},
		{
			Name:  "HealthyFalse",
			Query: "healthy:false",
			Expected: database.GetWorkspacesParams{
				HasAgentStatuses: []string{"disconnected", "timeout"},
			},
		},
		{
			Name:  "HealthyMissing",
			Query: "",
			Expected: database.GetWorkspacesParams{
				HasAgentStatuses: []string{},
			},
		},
		{
			Name:  "SharedWithUser",
			Query: `shared_with_user:3dd8b1b8-dff5-4b22-8ae9-c243ca136ecf`,
			Setup: func(t *testing.T, db database.Store) {
				dbgen.User(t, db, database.User{
					ID: uuid.MustParse("3dd8b1b8-dff5-4b22-8ae9-c243ca136ecf"),
				})
			},
			Expected: database.GetWorkspacesParams{
				SharedWithUserID: uuid.MustParse("3dd8b1b8-dff5-4b22-8ae9-c243ca136ecf"),
			},
		},
		{
			Name:  "SharedWithUserByName",
			Query: `shared_with_user:wibble`,
			Setup: func(t *testing.T, db database.Store) {
				dbgen.User(t, db, database.User{
					ID:       uuid.MustParse("3dd8b1b8-dff5-4b22-8ae9-c243ca136ecf"),
					Username: "wibble",
				})
			},
			Expected: database.GetWorkspacesParams{
				SharedWithUserID: uuid.MustParse("3dd8b1b8-dff5-4b22-8ae9-c243ca136ecf"),
			},
		},
		{
			Name:  "SharedWithGroupDefaultOrg",
			Query: "shared_with_group:wibble",
			Setup: func(t *testing.T, db database.Store) {
				org, err := db.GetOrganizationByName(t.Context(), database.GetOrganizationByNameParams{
					Name: "coder",
				})
				require.NoError(t, err)

				dbgen.Group(t, db, database.Group{
					ID:             uuid.MustParse("590f1006-15e6-4b21-a6e1-92e33af8a5c3"),
					Name:           "wibble",
					OrganizationID: org.ID,
				})
			},
			Expected: database.GetWorkspacesParams{
				SharedWithGroupID: uuid.MustParse("590f1006-15e6-4b21-a6e1-92e33af8a5c3"),
			},
		},
		{
			Name:  "SharedWithGroupInOrg",
			Query: "shared_with_group:wibble/wobble",
			Setup: func(t *testing.T, db database.Store) {
				org := dbgen.Organization(t, db, database.Organization{
					ID:   uuid.MustParse("dbeb1bd5-dce6-459c-ab7b-b7f8b9b10467"),
					Name: "wibble",
				})
				dbgen.Group(t, db, database.Group{
					ID:             uuid.MustParse("3c831688-0a5a-45a2-a796-f7648874df34"),
					Name:           "wobble",
					OrganizationID: org.ID,
				})
			},
			Expected: database.GetWorkspacesParams{
				SharedWithGroupID: uuid.MustParse("3c831688-0a5a-45a2-a796-f7648874df34"),
			},
		},
		{
			Name:  "SharedWithGroupID",
			Query: "shared_with_group:a7d1ba00-53c7-4aa6-92ea-83157dd57480",
			Setup: func(t *testing.T, db database.Store) {
				org := dbgen.Organization(t, db, database.Organization{
					ID: uuid.MustParse("8606620f-fee4-4c4e-83ba-f42db804139a"),
				})
				dbgen.Group(t, db, database.Group{
					ID:             uuid.MustParse("a7d1ba00-53c7-4aa6-92ea-83157dd57480"),
					OrganizationID: org.ID,
				})
			},
			Expected: database.GetWorkspacesParams{
				SharedWithGroupID: uuid.MustParse("a7d1ba00-53c7-4aa6-92ea-83157dd57480"),
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
		{
			Name:                  "SharedWithGroupTooManySegments",
			Query:                 `shared_with_group:acme/devs/extra`,
			ExpectedErrorContains: "the filter must be in the pattern of <organization name>/<group name>",
		},
		{
			Name:                  "SharedWithGroupEmptyOrg",
			Query:                 `shared_with_group:/devs`,
			ExpectedErrorContains: "invalid organization name",
		},
		{
			Name:                  "SharedWithGroupEmptyGroup",
			Query:                 `shared_with_group:acme/`,
			ExpectedErrorContains: "organization \"acme\" either does not exist",
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			// TODO: Replace this with the mock database.
			db, _ := dbtestutil.NewDB(t)
			if c.Setup != nil {
				c.Setup(t, db)
			}
			values, errs := searchquery.Workspaces(context.Background(), db, c.Query, codersdk.Pagination{}, 0)
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
				if len(c.Expected.HasAgentStatuses) == len(values.HasAgentStatuses) {
					// nil slice vs 0 len slice is equivalent for our purposes.
					c.Expected.HasAgentStatuses = values.HasAgentStatuses
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
		db, _ := dbtestutil.NewDB(t)
		values, errs := searchquery.Workspaces(context.Background(), db, query, codersdk.Pagination{}, timeout)
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
		ExpectedCountParams   database.CountAuditLogsParams
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
			ExpectedCountParams: database.CountAuditLogsParams{
				ResourceTarget: "foo",
			},
		},
		{
			Name:                  "RequestID",
			Query:                 "request_id:foo",
			ExpectedErrorContains: "valid uuid",
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			// Do not use a real database, this is only used for an
			// organization lookup.
			db, _ := dbtestutil.NewDB(t)
			values, countValues, errs := searchquery.AuditLogs(context.Background(), db, c.Query)
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
				require.Equal(t, c.ExpectedCountParams, countValues, "expected count values")
			}
		})
	}
}

func TestSearchConnectionLogs(t *testing.T) {
	t.Parallel()
	t.Run("All", func(t *testing.T) {
		t.Parallel()

		orgID := uuid.New()
		workspaceOwnerID := uuid.New()
		workspaceID := uuid.New()
		connectionID := uuid.New()

		db, _ := dbtestutil.NewDB(t)
		dbgen.Organization(t, db, database.Organization{
			ID:   orgID,
			Name: "testorg",
		})
		dbgen.User(t, db, database.User{
			ID:       workspaceOwnerID,
			Username: "testowner",
			Email:    "owner@example.com",
		})

		query := fmt.Sprintf(`organization:testorg workspace_owner:testowner `+
			`workspace_owner_email:owner@example.com type:port_forwarding username:testuser `+
			`user_email:test@example.com connected_after:"2023-01-01T00:00:00Z" `+
			`connected_before:"2023-01-16T12:00:00+12:00" workspace_id:%s connection_id:%s status:ongoing`,
			workspaceID.String(), connectionID.String())

		values, _, errs := searchquery.ConnectionLogs(context.Background(), db, query, database.APIKey{})
		require.Len(t, errs, 0)

		expected := database.GetConnectionLogsOffsetParams{
			OrganizationID:      orgID,
			WorkspaceOwner:      "testowner",
			WorkspaceOwnerEmail: "owner@example.com",
			Type:                string(database.ConnectionTypePortForwarding),
			Username:            "testuser",
			UserEmail:           "test@example.com",
			ConnectedAfter:      time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			ConnectedBefore:     time.Date(2023, 1, 16, 0, 0, 0, 0, time.UTC),
			WorkspaceID:         workspaceID,
			ConnectionID:        connectionID,
			Status:              string(codersdk.ConnectionLogStatusOngoing),
		}

		require.Equal(t, expected, values)
	})

	t.Run("Me", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		db, _ := dbtestutil.NewDB(t)

		query := `username:me workspace_owner:me`
		values, _, errs := searchquery.ConnectionLogs(context.Background(), db, query, database.APIKey{UserID: userID})
		require.Len(t, errs, 0)

		expected := database.GetConnectionLogsOffsetParams{
			UserID:           userID,
			WorkspaceOwnerID: userID,
		}

		require.Equal(t, expected, values)
	})
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
				Status:    []database.UserStatus{},
				RbacRole:  []string{},
				LoginType: []database.LoginType{},
			},
		},
		{
			Name:  "Username",
			Query: "user-name",
			Expected: database.GetUsersParams{
				Search:    "user-name",
				Status:    []database.UserStatus{},
				RbacRole:  []string{},
				LoginType: []database.LoginType{},
			},
		},
		{
			Name:  "UsernameWithSpaces",
			Query: "   user-name    ",
			Expected: database.GetUsersParams{
				Search:    "user-name",
				Status:    []database.UserStatus{},
				RbacRole:  []string{},
				LoginType: []database.LoginType{},
			},
		},
		{
			Name:  "Username+Param",
			Query: "usEr-name stAtus:actiVe",
			Expected: database.GetUsersParams{
				Search:    "user-name",
				Status:    []database.UserStatus{database.UserStatusActive},
				RbacRole:  []string{},
				LoginType: []database.LoginType{},
			},
		},
		{
			Name:  "OnlyParams",
			Query: "status:acTIve sEArch:User-Name role:Owner",
			Expected: database.GetUsersParams{
				Search:    "user-name",
				Status:    []database.UserStatus{database.UserStatusActive},
				RbacRole:  []string{codersdk.RoleOwner},
				LoginType: []database.LoginType{},
			},
		},
		{
			Name:  "QuotedParam",
			Query: `status:SuSpenDeD sEArch:"User Name" role:meMber`,
			Expected: database.GetUsersParams{
				Search:    "user name",
				Status:    []database.UserStatus{database.UserStatusSuspended},
				RbacRole:  []string{codersdk.RoleMember},
				LoginType: []database.LoginType{},
			},
		},
		{
			Name:  "QuotedKey",
			Query: `"status":acTIve "sEArch":User-Name "role":Owner`,
			Expected: database.GetUsersParams{
				Search:    "user-name",
				Status:    []database.UserStatus{database.UserStatusActive},
				RbacRole:  []string{codersdk.RoleOwner},
				LoginType: []database.LoginType{},
			},
		},
		{
			// Quotes keep elements together
			Name:  "QuotedSpecial",
			Query: `search:"user:name"`,
			Expected: database.GetUsersParams{
				Search:    "user:name",
				Status:    []database.UserStatus{},
				RbacRole:  []string{},
				LoginType: []database.LoginType{},
			},
		},
		{
			Name:  "LoginType",
			Query: "login_type:github",
			Expected: database.GetUsersParams{
				Search:    "",
				Status:    []database.UserStatus{},
				RbacRole:  []string{},
				LoginType: []database.LoginType{database.LoginTypeGithub},
			},
		},
		{
			Name:  "MultipleLoginTypesWithSpaces",
			Query: "login_type:github login_type:password",
			Expected: database.GetUsersParams{
				Search:   "",
				Status:   []database.UserStatus{},
				RbacRole: []string{},
				LoginType: []database.LoginType{
					database.LoginTypeGithub,
					database.LoginTypePassword,
				},
			},
		},
		{
			Name:  "MultipleLoginTypesWithCommas",
			Query: "login_type:github,password,none,oidc",
			Expected: database.GetUsersParams{
				Search:   "",
				Status:   []database.UserStatus{},
				RbacRole: []string{},
				LoginType: []database.LoginType{
					database.LoginTypeGithub,
					database.LoginTypePassword,
					database.LoginTypeNone,
					database.LoginTypeOIDC,
				},
			},
		},

		// Name filter tests
		{
			Name:  "NameFilter",
			Query: "name:John",
			Expected: database.GetUsersParams{
				Name:      "john",
				Status:    []database.UserStatus{},
				RbacRole:  []string{},
				LoginType: []database.LoginType{},
			},
		},
		{
			Name:  "NameFilterQuoted",
			Query: `name:"John Doe"`,
			Expected: database.GetUsersParams{
				Name:      "john doe",
				Status:    []database.UserStatus{},
				RbacRole:  []string{},
				LoginType: []database.LoginType{},
			},
		},
		{
			Name:  "NameFilterWithSearch",
			Query: "name:John search:johnd",
			Expected: database.GetUsersParams{
				Search:    "johnd",
				Name:      "john",
				Status:    []database.UserStatus{},
				RbacRole:  []string{},
				LoginType: []database.LoginType{},
			},
		},
		{
			Name:  "NameFilterWithOtherParams",
			Query: "name:John status:active role:owner",
			Expected: database.GetUsersParams{
				Name:      "john",
				Status:    []database.UserStatus{database.UserStatusActive},
				RbacRole:  []string{codersdk.RoleOwner},
				LoginType: []database.LoginType{},
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

func TestSearchTemplates(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	testCases := []struct {
		Name                  string
		Query                 string
		Expected              database.GetTemplatesWithFilterParams
		ExpectedErrorContains string
	}{
		{
			Name:     "Empty",
			Query:    "",
			Expected: database.GetTemplatesWithFilterParams{},
		},
		{
			Name:  "OnlyName",
			Query: "foobar",
			Expected: database.GetTemplatesWithFilterParams{
				FuzzyDisplayName: "foobar",
			},
		},
		{
			Name:  "HasAITaskTrue",
			Query: "has-ai-task:true",
			Expected: database.GetTemplatesWithFilterParams{
				HasAITask: sql.NullBool{
					Bool:  true,
					Valid: true,
				},
			},
		},
		{
			Name:  "HasAITaskFalse",
			Query: "has-ai-task:false",
			Expected: database.GetTemplatesWithFilterParams{
				HasAITask: sql.NullBool{
					Bool:  false,
					Valid: true,
				},
			},
		},
		{
			Name:  "HasAITaskMissing",
			Query: "",
			Expected: database.GetTemplatesWithFilterParams{
				HasAITask: sql.NullBool{
					Bool:  false,
					Valid: false,
				},
			},
		},
		{
			Name:  "HasExternalAgent",
			Query: "has_external_agent:true",
			Expected: database.GetTemplatesWithFilterParams{
				HasExternalAgent: sql.NullBool{
					Bool:  true,
					Valid: true,
				},
			},
		},
		{
			Name:  "HasExternalAgentFalse",
			Query: "has_external_agent:false",
			Expected: database.GetTemplatesWithFilterParams{
				HasExternalAgent: sql.NullBool{
					Bool:  false,
					Valid: true,
				},
			},
		},
		{
			Name:  "HasExternalAgentMissing",
			Query: "",
			Expected: database.GetTemplatesWithFilterParams{
				HasExternalAgent: sql.NullBool{
					Bool:  false,
					Valid: false,
				},
			},
		},
		{
			Name:  "MyTemplates",
			Query: "author:me",
			Expected: database.GetTemplatesWithFilterParams{
				AuthorUsername: "",
				AuthorID:       userID,
			},
		},
		{
			Name:  "SearchOnDisplayName",
			Query: "test name",
			Expected: database.GetTemplatesWithFilterParams{
				FuzzyDisplayName: "test name",
			},
		},
		{
			Name:  "NameField",
			Query: "name:testname",
			Expected: database.GetTemplatesWithFilterParams{
				FuzzyName: "testname",
			},
		},
		{
			Name:  "QuotedValue",
			Query: `name:"test name"`,
			Expected: database.GetTemplatesWithFilterParams{
				FuzzyName: "test name",
			},
		},
		{
			Name:  "MultipleTerms",
			Query: `foo bar exact_name:"test display name"`,
			Expected: database.GetTemplatesWithFilterParams{
				ExactName:        "test display name",
				FuzzyDisplayName: "foo bar",
			},
		},
		{
			Name:  "FieldAndSpaces",
			Query: "deprecated:false test template",
			Expected: database.GetTemplatesWithFilterParams{
				Deprecated:       sql.NullBool{Bool: false, Valid: true},
				FuzzyDisplayName: "test template",
			},
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			// Do not use a real database, this is only used for an
			// organization lookup.
			db, _ := dbtestutil.NewDB(t)
			values, errs := searchquery.Templates(context.Background(), db, userID, c.Query)
			if c.ExpectedErrorContains != "" {
				require.True(t, len(errs) > 0, "expect some errors")
				var s strings.Builder
				for _, err := range errs {
					_, _ = s.WriteString(fmt.Sprintf("%s: %s\n", err.Field, err.Detail))
				}
				require.Contains(t, s.String(), c.ExpectedErrorContains)
			} else {
				require.Len(t, errs, 0, "expected no error")
				if c.Expected.IDs == nil {
					// Nil and length 0 are the same
					c.Expected.IDs = []uuid.UUID{}
				}
				require.Equal(t, c.Expected, values, "expected values")
			}
		})
	}
}

func TestSearchTasks(t *testing.T) {
	t.Parallel()

	userID := uuid.MustParse("10000000-0000-0000-0000-000000000001")
	orgID := uuid.MustParse("20000000-0000-0000-0000-000000000001")

	testCases := []struct {
		Name                  string
		Query                 string
		ActorID               uuid.UUID
		Expected              database.ListTasksParams
		ExpectedErrorContains string
		Setup                 func(t *testing.T, db database.Store)
	}{
		{
			Name:     "Empty",
			Query:    "",
			Expected: database.ListTasksParams{},
		},
		{
			Name:  "OwnerUsername",
			Query: "owner:alice",
			Setup: func(t *testing.T, db database.Store) {
				dbgen.User(t, db, database.User{
					ID:       userID,
					Username: "alice",
				})
			},
			Expected: database.ListTasksParams{
				OwnerID: userID,
			},
		},
		{
			Name:    "OwnerMe",
			Query:   "owner:me",
			ActorID: userID,
			Expected: database.ListTasksParams{
				OwnerID: userID,
			},
		},
		{
			Name:  "OwnerUUID",
			Query: fmt.Sprintf("owner:%s", userID),
			Expected: database.ListTasksParams{
				OwnerID: userID,
			},
		},
		{
			Name:  "StatusActive",
			Query: "status:active",
			Expected: database.ListTasksParams{
				Status: "active",
			},
		},
		{
			Name:  "StatusPending",
			Query: "status:pending",
			Expected: database.ListTasksParams{
				Status: "pending",
			},
		},
		{
			Name:  "Organization",
			Query: "organization:acme",
			Setup: func(t *testing.T, db database.Store) {
				dbgen.Organization(t, db, database.Organization{
					ID:   orgID,
					Name: "acme",
				})
			},
			Expected: database.ListTasksParams{
				OrganizationID: orgID,
			},
		},
		{
			Name:  "OrganizationUUID",
			Query: fmt.Sprintf("organization:%s", orgID),
			Expected: database.ListTasksParams{
				OrganizationID: orgID,
			},
		},
		{
			Name:  "Combined",
			Query: "owner:alice organization:acme status:active",
			Setup: func(t *testing.T, db database.Store) {
				dbgen.Organization(t, db, database.Organization{
					ID:   orgID,
					Name: "acme",
				})
				dbgen.User(t, db, database.User{
					ID:       userID,
					Username: "alice",
				})
			},
			Expected: database.ListTasksParams{
				OwnerID:        userID,
				OrganizationID: orgID,
				Status:         "active",
			},
		},
		{
			Name:  "QuotedOwner",
			Query: `owner:"alice"`,
			Setup: func(t *testing.T, db database.Store) {
				dbgen.User(t, db, database.User{
					ID:       userID,
					Username: "alice",
				})
			},
			Expected: database.ListTasksParams{
				OwnerID: userID,
			},
		},
		{
			Name:  "QuotedStatus",
			Query: `status:"pending"`,
			Expected: database.ListTasksParams{
				Status: "pending",
			},
		},
		{
			Name:  "DefaultToOwner",
			Query: "alice",
			Setup: func(t *testing.T, db database.Store) {
				dbgen.User(t, db, database.User{
					ID:       userID,
					Username: "alice",
				})
			},
			Expected: database.ListTasksParams{
				OwnerID: userID,
			},
		},
		{
			Name:                  "InvalidOwner",
			Query:                 "owner:nonexistent",
			ExpectedErrorContains: "does not exist",
		},
		{
			Name:                  "InvalidOrganization",
			Query:                 "organization:nonexistent",
			ExpectedErrorContains: "does not exist",
		},
		{
			Name:  "ExtraParam",
			Query: "owner:alice invalid:param",
			Setup: func(t *testing.T, db database.Store) {
				dbgen.User(t, db, database.User{
					ID:       userID,
					Username: "alice",
				})
			},
			ExpectedErrorContains: "is not a valid query param",
		},
		{
			Name:                  "ExtraColon",
			Query:                 "owner:alice:extra",
			ExpectedErrorContains: "can only contain 1 ':'",
		},
		{
			Name:                  "PrefixColon",
			Query:                 ":owner",
			ExpectedErrorContains: "cannot start or end with ':'",
		},
		{
			Name:                  "SuffixColon",
			Query:                 "owner:",
			ExpectedErrorContains: "cannot start or end with ':'",
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			db, _ := dbtestutil.NewDB(t)

			if c.Setup != nil {
				c.Setup(t, db)
			}

			values, errs := searchquery.Tasks(context.Background(), db, c.Query, c.ActorID)
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
