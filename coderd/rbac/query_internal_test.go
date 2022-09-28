package rbac

import (
	"testing"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"

	"github.com/stretchr/testify/require"
)

func TestCompileQuery(t *testing.T) {
	t.Run("EmptyQuery", func(t *testing.T) {
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
		expression, err := Compile(&rego.PartialQueries{
			Queries: []ast.Body{
				must(ast.ParseBody("true")),
			},
			Support: []*ast.Module{},
		})
		require.NoError(t, err, "compile empty")

		require.Equal(t, "true", expression.RegoString(), "true query is rego 'true'")
		require.Equal(t, "true", expression.SQLString(SQLConfig{}), "true query is sql 'true'")
	})
}
