package rbac

import (
	"testing"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"

	"github.com/stretchr/testify/require"
)

func TestCompileQuery(t *testing.T) {
	t.Parallel()
	opts := ast.ParserOptions{
		AllFutureKeywords: true,
	}
	t.Run("EmptyQuery", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(&rego.PartialQueries{
			Queries: []ast.Body{
				must(ast.ParseBody("")),
			},
			Support: []*ast.Module{},
		})
		require.NoError(t, err, "compile empty")

		require.Equal(t, "true", expression.RegoString(), "empty query is rego 'true'")
		require.Equal(t, "true", expression.SQLString(SQLConfig{}), "empty query is sql 'true'")
	})

	t.Run("TrueQuery", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(&rego.PartialQueries{
			Queries: []ast.Body{
				must(ast.ParseBody("true")),
			},
			Support: []*ast.Module{},
		})
		require.NoError(t, err, "compile")

		require.Equal(t, "true", expression.RegoString(), "true query is rego 'true'")
		require.Equal(t, "true", expression.SQLString(SQLConfig{}), "true query is sql 'true'")
	})

	t.Run("ACLIn", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(&rego.PartialQueries{
			Queries: []ast.Body{
				ast.MustParseBodyWithOpts(`"*" in input.object.acl_group_list.allUsers`, opts),
			},
			Support: []*ast.Module{},
		})
		require.NoError(t, err, "compile")

		require.Equal(t, `internal.member_2("*", input.object.acl_group_list.allUsers)`, expression.RegoString(), "convert to internal_member")
		require.Equal(t, `group_acl->'allUsers' ? '*'`, expression.SQLString(DefaultConfig()), "jsonb in")
	})

	t.Run("Complex", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(&rego.PartialQueries{
			Queries: []ast.Body{
				ast.MustParseBodyWithOpts(`input.object.org_owner != ""`, opts),
				ast.MustParseBodyWithOpts(`input.object.org_owner in {"a", "b", "c"}`, opts),
				ast.MustParseBodyWithOpts(`input.object.org_owner != ""`, opts),
				ast.MustParseBodyWithOpts(`"read" in input.object.acl_group_list.allUsers`, opts),
				ast.MustParseBodyWithOpts(`"read" in input.object.acl_user_list.me`, opts),
			},
			Support: []*ast.Module{},
		})
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
		expression, err := Compile(&rego.PartialQueries{
			Queries: []ast.Body{
				ast.MustParseBodyWithOpts(`"*" in input.object.acl_group_list[input.object.org_owner]`, opts),
			},
			Support: []*ast.Module{},
		})
		require.NoError(t, err, "compile")
		require.Equal(t, `group_acl->organization_id :: text ? '*'`,
			expression.SQLString(DefaultConfig()), "set dereference")
	})

	t.Run("JsonbLiteralDereference", func(t *testing.T) {
		t.Parallel()
		expression, err := Compile(&rego.PartialQueries{
			Queries: []ast.Body{
				ast.MustParseBodyWithOpts(`"*" in input.object.acl_group_list["4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75"]`, opts),
			},
			Support: []*ast.Module{},
		})
		require.NoError(t, err, "compile")
		require.Equal(t, `group_acl->'4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75' ? '*'`,
			expression.SQLString(DefaultConfig()), "literal dereference")
	})
}
