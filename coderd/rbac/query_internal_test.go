package rbac

import (
	"context"
	"testing"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"

	"github.com/stretchr/testify/require"
)

func TestCompileQuery(t *testing.T) {
	t.Parallel()

	t.Run("EmptyQuery", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(partialQueries(t, ""))
		require.NoError(t, err, "compile empty")

		require.Equal(t, "true", expression.RegoString(), "empty query is rego 'true'")
		require.Equal(t, "true", expression.SQLString(SQLConfig{}), "empty query is sql 'true'")
	})

	t.Run("TrueQuery", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(partialQueries(t, "true"))
		require.NoError(t, err, "compile")

		require.Equal(t, "true", expression.RegoString(), "true query is rego 'true'")
		require.Equal(t, "true", expression.SQLString(SQLConfig{}), "true query is sql 'true'")
	})

	t.Run("ACLIn", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(partialQueries(t, `"*" in input.object.acl_group_list.allUsers`))
		require.NoError(t, err, "compile")

		require.Equal(t, `internal.member_2("*", input.object.acl_group_list.allUsers)`, expression.RegoString(), "convert to internal_member")
		require.Equal(t, `group_acl->'allUsers' ? '*'`, expression.SQLString(DefaultConfig()), "jsonb in")
	})

	t.Run("Complex", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(partialQueries(t,
			`input.object.org_owner != ""`,
			`input.object.org_owner in {"a", "b", "c"}`,
			`input.object.org_owner != ""`,
			`"read" in input.object.acl_group_list.allUsers`,
			`"read" in input.object.acl_user_list.me`,
		))
		require.NoError(t, err, "compile")
		require.Equal(t, `(organization_id :: text != '' OR `+
			`organization_id :: text = ANY(ARRAY ['a','b','c']) OR `+
			`organization_id :: text != '' OR `+
			`group_acl->'allUsers' ? 'read' OR `+
			`user_acl->'me' ? 'read')`,
			expression.SQLString(DefaultConfig()), "complex")
	})

	t.Run("SetDereference", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(partialQueries(t,
			`"*" in input.object.acl_group_list[input.object.org_owner]`,
		))
		require.NoError(t, err, "compile")
		require.Equal(t, `group_acl->organization_id :: text ? '*'`,
			expression.SQLString(DefaultConfig()), "set dereference")
	})

	t.Run("JsonbLiteralDereference", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(partialQueries(t,
			`"*" in input.object.acl_group_list["4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75"]`,
		))
		require.NoError(t, err, "compile")
		require.Equal(t, `group_acl->'4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75' ? '*'`,
			expression.SQLString(DefaultConfig()), "literal dereference")
	})

	t.Run("NoACLColumns", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(partialQueries(t,
			`"*" in input.object.acl_group_list["4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75"]`,
		))
		require.NoError(t, err, "compile")
		require.Equal(t, `false`,
			expression.SQLString(NoACLConfig()), "literal dereference")
	})
}

func TestEvalQuery(t *testing.T) {
	t.Parallel()

	t.Run("GroupACL", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(partialQueries(t,
			`"read" in input.object.acl_group_list["4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75"]`,
		))
		require.NoError(t, err, "compile")

		result := expression.Eval(Object{
			Owner:       "not-me",
			OrgID:       "random",
			Type:        "workspace",
			ACLUserList: map[string][]Action{},
			ACLGroupList: map[string][]Action{
				"4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75": {"read"},
			},
		})
		require.True(t, result, "eval")
	})
}

func partialQueries(t *testing.T, queries ...string) *PartialAuthorizer {
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
	return &PartialAuthorizer{
		partialQueries: &rego.PartialQueries{
			Queries: astQueries,
			Support: []*ast.Module{},
		},
		preparedQueries: prepareQueries,
		input:           nil,
		alwaysTrue:      false,
	}
}
