package regosql_test

import (
	"testing"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac/regosql"
	"github.com/coder/coder/v2/coderd/rbac/regosql/sqltypes"
)

// TestRegoQueriesNoVariables handles cases without variables. These should be
// very simple and straight forward.
func TestRegoQueries(t *testing.T) {
	t.Parallel()

	p := func(v string) string {
		return "(" + v + ")"
	}

	testCases := []struct {
		Name                string
		Queries             []string
		ExpectedSQL         string
		ExpectError         bool
		ExpectedSQLGenError bool

		VariableConverter sqltypes.VariableMatcher
	}{
		{
			Name:        "Empty",
			Queries:     []string{``},
			ExpectedSQL: "true",
		},
		{
			Name:        "True",
			Queries:     []string{`true`},
			ExpectedSQL: "true",
		},
		{
			Name:        "False",
			Queries:     []string{`false`},
			ExpectedSQL: "false",
		},
		{
			Name:        "MultipleBool",
			Queries:     []string{"true", "false"},
			ExpectedSQL: "(true OR false)",
		},
		{
			Name: "Numbers",
			Queries: []string{
				"(1 != 2) = true",
				"5 == 5",
			},
			ExpectedSQL: p("((1 != 2) = true) OR (5 = 5)"),
		},
		// Variables
		{
			// Always return a constant string for all variables.
			Name: "V_Basic",
			Queries: []string{
				`input.x = "hello_world"`,
			},
			ExpectedSQL: p("only_var = 'hello_world'"),
			VariableConverter: sqltypes.NewVariableConverter().RegisterMatcher(
				sqltypes.StringVarMatcher("only_var", []string{
					"input", "x",
				}),
			),
		},
		// Coder Variables
		{
			// Always return a constant string for all variables.
			Name: "GroupACL",
			Queries: []string{
				`"read" in input.object.acl_group_list.allUsers`,
			},
			ExpectedSQL:       "(group_acl->'allUsers' ? 'read')",
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			Name:              "GroupWildcard",
			Queries:           []string{`"*" in input.object.acl_group_list.allUsers`},
			ExpectedSQL:       "(group_acl->'allUsers' ? '*')",
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			// Always return a constant string for all variables.
			Name: "GroupACLWithVarField",
			Queries: []string{
				`"read" in input.object.acl_group_list[input.object.org_owner]`,
			},
			ExpectedSQL:       "(group_acl->organization_id :: text ? 'read')",
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			Name: "VarInArray",
			Queries: []string{
				`input.object.org_owner in {"a", "b", "c"}`,
			},
			ExpectedSQL:       p("organization_id :: text = ANY(ARRAY ['a','b','c'])"),
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			Name:              "SetDereference",
			Queries:           []string{`"*" in input.object.acl_group_list[input.object.org_owner]`},
			ExpectedSQL:       p("group_acl->organization_id :: text ? '*'"),
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			Name:              "JsonbLiteralDereference",
			Queries:           []string{`"*" in input.object.acl_group_list["4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75"]`},
			ExpectedSQL:       p("group_acl->'4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75' ? '*'"),
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			Name: "Complex",
			Queries: []string{
				`input.object.org_owner != ""`,
				`input.object.org_owner in {"a", "b", "c"}`,
				`input.object.org_owner != ""`,
				`"read" in input.object.acl_group_list.allUsers`,
				`"read" in input.object.acl_user_list.me`,
			},
			ExpectedSQL: `((organization_id :: text != '') OR ` +
				`(organization_id :: text = ANY(ARRAY ['a','b','c'])) OR ` +
				`(organization_id :: text != '') OR ` +
				`(group_acl->'allUsers' ? 'read') OR ` +
				`(user_acl->'me' ? 'read'))`,
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			Name: "NoACLs",
			Queries: []string{
				`"read" in input.object.acl_group_list[input.object.org_owner]`,
				`"*" in input.object.acl_group_list["4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75"]`,
			},
			// Special case where the bool is wrapped
			ExpectedSQL:       p("(false) OR (false)"),
			VariableConverter: regosql.NoACLConverter(),
		},
		{
			Name: "AllowList",
			Queries: []string{
				`input.object.id != "" `,
				`input.object.id in ["9046b041-58ed-47a3-9c3a-de302577875a"]`,
			},
			// Special case where the bool is wrapped
			ExpectedSQL: p(`(id :: text != '') OR ` +
				`(id :: text = ANY(ARRAY ['9046b041-58ed-47a3-9c3a-de302577875a']))`),
			VariableConverter: regosql.NoACLConverter(),
		},
		{
			Name: "TwoExpressions",
			Queries: []string{
				`true; true`,
			},
			ExpectedSQL:       p("true AND true"),
			VariableConverter: regosql.DefaultVariableConverter(),
		},

		// Actual vectors from production
		{
			Name: "FromOwner",
			Queries: []string{
				``,
				`"05f58202-4bfc-43ce-9ba4-5ff6e0174a71" = input.object.org_owner`,
				`"read" in input.object.acl_user_list["d5389ccc-57a4-4b13-8c3f-31747bcdc9f1"]`,
			},
			ExpectedSQL:       "true",
			VariableConverter: regosql.NoACLConverter(),
		},
		{
			Name: "OrgAdmin",
			Queries: []string{
				`input.object.org_owner != "";
                input.object.org_owner in {"05f58202-4bfc-43ce-9ba4-5ff6e0174a71"};
                input.object.owner != "";
                "d5389ccc-57a4-4b13-8c3f-31747bcdc9f1" = input.object.owner`,
			},
			ExpectedSQL: "((organization_id :: text != '') AND " +
				"(organization_id :: text = ANY(ARRAY ['05f58202-4bfc-43ce-9ba4-5ff6e0174a71'])) AND " +
				"(owner_id :: text != '') AND " +
				"('d5389ccc-57a4-4b13-8c3f-31747bcdc9f1' = owner_id :: text))",
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			Name: "UserACLAllow",
			Queries: []string{
				`"read" in input.object.acl_user_list["d5389ccc-57a4-4b13-8c3f-31747bcdc9f1"]`,
				`"*" in input.object.acl_user_list["d5389ccc-57a4-4b13-8c3f-31747bcdc9f1"]`,
			},
			ExpectedSQL: "((user_acl->'d5389ccc-57a4-4b13-8c3f-31747bcdc9f1' ? 'read') OR " +
				"(user_acl->'d5389ccc-57a4-4b13-8c3f-31747bcdc9f1' ? '*'))",
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			Name: "NoACLConfig",
			Queries: []string{
				`input.object.org_owner != "";
                input.object.org_owner in {"05f58202-4bfc-43ce-9ba4-5ff6e0174a71"};
                "read" in input.object.acl_group_list[input.object.org_owner]`,
			},
			ExpectedSQL:       "((organization_id :: text != '') AND (organization_id :: text = ANY(ARRAY ['05f58202-4bfc-43ce-9ba4-5ff6e0174a71'])) AND (false))",
			VariableConverter: regosql.NoACLConverter(),
		},
		{
			Name: "EmptyACLListNoACLs",
			Queries: []string{
				`input.object.org_owner != "";
				input.object.org_owner in set();
				"create" in input.object.acl_group_list[input.object.org_owner]`,

				`input.object.org_owner != "";
				input.object.org_owner in set();
				"*" in input.object.acl_group_list[input.object.org_owner]`,

				`"create" in input.object.acl_user_list.me`,

				`"*" in input.object.acl_user_list.me`,
			},
			ExpectedSQL: p(p("(organization_id :: text != '') AND (false) AND (group_acl->organization_id :: text ? 'create')") + " OR " +
				p("(organization_id :: text != '') AND (false) AND (group_acl->organization_id :: text ? '*')") + " OR " +
				p("user_acl->'me' ? 'create'") + " OR " +
				p("user_acl->'me' ? '*'")),
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			Name: "TemplateOwner",
			Queries: []string{
				`neq(input.object.org_owner, "");
internal.member_2(input.object.org_owner, {"3bf82434-e40b-44ae-b3d8-d0115bba9bad", "5630fda3-26ab-462c-9014-a88a62d7a415", "c304877a-bc0d-4e9b-9623-a38eae412929"});
neq(input.object.owner, "");
"806dd721-775f-4c85-9ce3-63fbbd975954" = input.object.owner`,
			},
			ExpectedSQL: p(p("organization_id :: text != ''") + " AND " +
				p("organization_id :: text = ANY(ARRAY ['3bf82434-e40b-44ae-b3d8-d0115bba9bad','5630fda3-26ab-462c-9014-a88a62d7a415','c304877a-bc0d-4e9b-9623-a38eae412929'])") + " AND " +
				p("false") + " AND " +
				p("false")),
			VariableConverter: regosql.TemplateConverter(),
		},
		{
			Name: "UserNoOrgOwner",
			Queries: []string{
				`input.object.org_owner != ""`,
			},
			ExpectedSQL:       p("'' != ''"),
			VariableConverter: regosql.UserConverter(),
		},
		{
			Name: "UserOwnsSelf",
			Queries: []string{
				`"10d03e62-7703-4df5-a358-4f76577d4e2f" = input.object.owner;
				input.object.owner != "";
				input.object.org_owner = ""`,
			},
			VariableConverter: regosql.UserConverter(),
			ExpectedSQL: p(
				p("'10d03e62-7703-4df5-a358-4f76577d4e2f' = id :: text") + " AND " + p("id :: text != ''") + " AND " + p("'' = ''"),
			),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			part := partialQueries(tc.Queries...)

			cfg := regosql.ConvertConfig{
				VariableConverter: tc.VariableConverter,
			}

			requireConvert(t, convertTestCase{
				part:               part,
				cfg:                cfg,
				expectSQL:          tc.ExpectedSQL,
				expectConvertError: tc.ExpectError,
				expectSQLGenError:  tc.ExpectedSQLGenError,
			})
		})
	}
}

type convertTestCase struct {
	part *rego.PartialQueries
	cfg  regosql.ConvertConfig

	expectConvertError bool
	expectSQL          string
	expectSQLGenError  bool
}

func requireConvert(t *testing.T, tc convertTestCase) {
	t.Helper()

	for i, q := range tc.part.Queries {
		t.Logf("Query %d: %s", i, q.String())
	}
	for i, s := range tc.part.Support {
		t.Logf("Support %d: %s", i, s.String())
	}

	root, err := regosql.ConvertRegoAst(tc.cfg, tc.part)
	if tc.expectConvertError {
		require.Error(t, err)
	} else {
		require.NoError(t, err, "compile")

		gen := sqltypes.NewSQLGenerator()
		sqlString := root.SQLString(gen)
		if tc.expectSQLGenError {
			require.True(t, len(gen.Errors()) > 0, "expected SQL generation error")
		} else {
			require.NoError(t, err, "sql gen")
			require.Equal(t, tc.expectSQL, sqlString, "sql match")
		}
	}
}

func partialQueries(queries ...string) *rego.PartialQueries {
	opts := ast.ParserOptions{
		AllFutureKeywords: true,
	}

	astQueries := make([]ast.Body, 0, len(queries))
	for _, q := range queries {
		astQueries = append(astQueries, ast.MustParseBodyWithOpts(q, opts))
	}

	return &rego.PartialQueries{
		Queries: astQueries,
		Support: []*ast.Module{},
	}
}
