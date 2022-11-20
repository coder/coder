package regosql

import (
	"github.com/coder/coder/coderd/rbac/regosql/sqltypes"
	"github.com/open-policy-agent/opa/ast"
)

var _ sqltypes.Node = alwaysFalse{}
var _ sqltypes.VariableMatcher = alwaysFalse{}

type alwaysFalse struct {
	Matcher sqltypes.VariableMatcher

	InnerNode sqltypes.Node
}

func AlwaysFalse(m sqltypes.VariableMatcher) alwaysFalse {
	return alwaysFalse{
		Matcher: m,
	}
}

func (alwaysFalse) UseAs() sqltypes.Node { return sqltypes.AstBoolean{} }
func (g alwaysFalse) ConvertVariable(rego ast.Ref) (sqltypes.Node, bool) {
	n, ok := g.Matcher.ConvertVariable(rego)
	if ok {
		return alwaysFalse{
			Matcher:   g.Matcher,
			InnerNode: n,
		}, true
	}

	return nil, false
}

func (g alwaysFalse) SQLString(_ *sqltypes.SQLGenerator) string {
	return "false"
}

func (g alwaysFalse) ContainsSQL(_ *sqltypes.SQLGenerator, _ sqltypes.Node) (string, error) {
	return "false", nil
}

func (g alwaysFalse) EqualsSQLString(_ *sqltypes.SQLGenerator, not bool, _ sqltypes.Node) (string, error) {
	return "false", nil
}
