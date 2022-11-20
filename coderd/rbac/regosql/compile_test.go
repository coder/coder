package regosql_test

import (
	"context"
	"testing"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/rbac/regosql"
	"github.com/coder/coder/coderd/rbac/regosql/sqltypes"
)

//func TestSomething(t *testing.T) {
//	ConvertRegoAst(ConvertConfig{
//		ConvertVariable: func(rego ast.Ref) (sqltypes.Node, error) {
//			return nil, fmt.Errorf("not implemented")
//		},
//	}, &rego.PartialQueries{})
//}

//func TestPartialQueriesWithVariables(t *testing.T) {
//	cfg := regosql.CompileConfig{
//		VariableTypes: NewTree().
//			AddElement(strings.Split("input.post.deleted", "."), Boolean{}, StaticName("deleted")).
//			AddElement(strings.Split("input.post.author", "."), String{}, StaticName("author")).
//			AddElement(strings.Split("input.post.can", "."), String{}, StaticName("can")).
//			AddElement(strings.Split("input.post.authors", "."), Map{ValueType: String{}},
//				RegexColumnNameReplace(`input\.post\.authors\.(.*)`, "authors->$1")).
//			AddElement(strings.Split("input.post.posts", "."), Array{elemType: String{}}, StaticName("posts")).
//			AddElement(strings.Split("input.post.can_list", "."), Array{elemType: String{}}, StaticName("can_list")).
//			AddElement(strings.Split("input.post.list", "."), Array{elemType: String{}}, StaticName("list")).
//			AddElement(strings.Split("input.post.moderators", "."), Array{elemType: String{}}, StaticName("moderators")),
//	}
//	//opts := ast.ParserOptions{AllFutureKeywords: true}
//	testCases := []struct {
//		Name        string
//		Input       map[string]interface{}
//		Unknowns    []string
//		Rego        string
//		ExpectedSQL string
//		ExpectError bool
//	}{
//		{
//			Name: "AlwaysFalse",
//			Rego: `
// 			package example
//			allow = true {
//   	 			input.method = "GET"
//    			input.path = ["posts"]
//			}`,
//			Input: map[string]interface{}{
//				"method": "GET",
//				"path":   []string{"users"},
//				"user":   "bob",
//			},
//			ExpectedSQL: "false",
//			Unknowns:    []string{"none"},
//		},
//		{
//			Name: "AlwaysTrue",
//			Rego: `
// 			package example
//			allow = true {
//   	 			input.method = "GET"
//    			input.path = ["posts"]
//			}`,
//			Input: map[string]interface{}{
//				"method": "GET",
//				"path":   []string{"posts"},
//				"user":   "bob",
//			},
//			ExpectedSQL: "true",
//			Unknowns:    []string{"none"},
//		},
//		{
//			Name: "SingleObject",
//			// "bob" = input.post.author
//			Rego: `
//			package example
//			allow {
//				input.post.author = input.user
//			}
//			`,
//			Input: map[string]interface{}{
//				"user": "bob",
//			},
//			ExpectedSQL: "'bob' = author",
//			Unknowns:    []string{"input.post.author"},
//		},
//		{
//			Name: "RefBoolean",
//			// input.post.deleted
//			Rego: `
//			package example
//			allow {
//				input.post.deleted
//			}
//			`,
//			Input:       map[string]interface{}{},
//			ExpectedSQL: "deleted",
//			Unknowns:    []string{"input.post.deleted"},
//		},
//		{
//			Name: "RefWithNumber",
//			// Query 0: "bob" = input.post.authors.name; "bob" = input.post.list[0]
//			Rego: `
//			package example
//			allow {
//				input.post.authors["name"] = input.user
//				input.post.list[0] = input.user
//			}
//			`,
//			Input: map[string]interface{}{
//				"user": "bob",
//			},
//			ExpectedSQL: "authors->>name = 'bob AND list[0] = 'bob",
//			Unknowns:    []string{"input.post.authors", "input.post.list"},
//		},
//		{
//			Name: "Array",
//			// Query 0: "bob" = input.post.author
//			// Query 1: "bob" = input.post.moderators[_]
//			Rego: `
//			package example
//			allow {
//				can_edit
//			}
//
//			can_edit {
//				input.post.author = input.user
//			}
//			can_edit {
//				input.post.moderators[_] = input.user
//			}
//
//			`,
//			Input: map[string]interface{}{
//				"user": "bob",
//			},
//			ExpectedSQL: "'bob' = author OR 'bob' = ANY(moderators)",
//			Unknowns:    []string{"input.post.author", "input.post.moderators"},
//		},
//		{
//			Name: "ArrayIntersection",
//			// Query 0: internal.member_2(input.can_list[_], ["edit", "*"])
//			// Query 1: internal.member_2(input.can, ["edit", "*"])
//			Rego: `
//			package example
//			import future.keywords.in
//			allow {
//				input.can in ["edit", "*"]
//			}
//
//			allow {
//				input.can_list[_] in ["edit", "*"]
//			}
//			`,
//			Input: map[string]interface{}{},
//			// TODO: Convert vars to columns
//			ExpectedSQL: "input.can_list && ARRAY['edit', '*'] OR input.can = ANY(ARRAY ['edit', '*'])",
//			Unknowns:    []string{"input.can_list", "input.can"},
//		},
//		{
//			Name: "EveryTerm",
//			// "bob" = input.posts[_].author; input.posts[_]
//			Rego: `
//			package example
//			allow = true {
//				input.method = "GET"
//				input.path = ["posts"]
//				allowed[x]
//			}
//
//			allowed[x] {
//				x := input.posts[_]
//				x.author == input.user
//			}
//			`,
//			Input: map[string]interface{}{
//				"method": "GET",
//				"path":   []string{"posts"},
//				"user":   "bob",
//			},
//			ExpectedSQL: "true",
//			Unknowns:    []string{"input.posts"},
//		},
//
//		// Failures
//		{
//			Name: "RefString",
//			Rego: `
//			package example
//			allow {
//				input.post.author
//			}
//			`,
//			Input:       map[string]interface{}{},
//			Unknowns:    []string{"input.post.author"},
//			ExpectError: true,
//		},
//	}
//
//	for _, tc := range testCases {
//		tc := tc
//		t.Run(tc.Name, func(t *testing.T) {
//			t.Parallel()
//			ctx := context.Background()
//
//			part, err := rego.New(
//				rego.Query("data.example.allow == true"),
//				rego.Module("policy.rego", tc.Rego),
//				rego.Input(tc.Input),
//				rego.Unknowns(tc.Unknowns),
//			).Partial(ctx)
//			require.NoError(t, err)
//
//			for i, q := range part.Queries {
//				t.Logf("Query %d: %s", i, q.String())
//			}
//			for i, s := range part.Support {
//				t.Logf("Support %d: %s", i, s.String())
//			}
//
//			sql, err := CompileSQL(cfg, part)
//			if tc.ExpectError {
//				require.Error(t, err)
//			} else {
//				require.NoError(t, err, "compile")
//				require.Equal(t, tc.ExpectedSQL, sql, "sql match")
//			}
//		})
//	}
//}

// TestRegoQueriesNoVariables handles cases without variables. These should be
// very simple and straight forward.
func TestRegoQueries(t *testing.T) {
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
			ExpectedSQL: "(((1 != 2) = true) OR (5 = 5))",
		},
		// Variables
		{
			// Always return a constant string for all variables.
			Name: "V_Basic",
			Queries: []string{
				`input.x = "hello_world"`,
			},
			ExpectedSQL: "(only_var = 'hello_world')",
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
			ExpectedSQL:       "(organization_id :: text = ANY(ARRAY ['a','b','c']))",
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			Name:              "SetDereference",
			Queries:           []string{`"*" in input.object.acl_group_list[input.object.org_owner]`},
			ExpectedSQL:       "(group_acl->organization_id :: text ? '*')",
			VariableConverter: regosql.DefaultVariableConverter(),
		},
		{
			Name:              "JsonbLiteralDereference",
			Queries:           []string{`"*" in input.object.acl_group_list["4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75"]`},
			ExpectedSQL:       "(group_acl->'4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75' ? '*')",
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
			ExpectedSQL:       "((false) OR (false))",
			VariableConverter: regosql.NoACLConverter(),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			part := partialQueries(t, tc.Queries...)

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

func partialQueries(t *testing.T, queries ...string) *rego.PartialQueries {
	opts := ast.ParserOptions{
		AllFutureKeywords: true,
	}

	astQueries := make([]ast.Body, 0, len(queries))
	for _, q := range queries {
		astQueries = append(astQueries, ast.MustParseBodyWithOpts(q, opts))
	}

	prepareQueries := make([]rego.PreparedEvalQuery, 0, len(queries))
	for _, q := range astQueries {
		var prepped rego.PreparedEvalQuery
		var err error
		if q.String() == "" {
			prepped, err = rego.New(
				rego.Query("true"),
			).PrepareForEval(context.Background())
		} else {
			prepped, err = rego.New(
				rego.ParsedQuery(q),
			).PrepareForEval(context.Background())
		}
		require.NoError(t, err, "prepare query")
		prepareQueries = append(prepareQueries, prepped)
	}
	return &rego.PartialQueries{
		Queries: astQueries,
		Support: []*ast.Module{},
	}
}
