package sqltypes

import (
	"github.com/open-policy-agent/opa/ast"
)

var _ Node = alwaysFalse{}
var _ VariableMatcher = alwaysFalse{}

type alwaysFalse struct {
	Matcher VariableMatcher

	InnerNode Node
}

// AlwaysFalse overrides the inner node with a constant "false".
func AlwaysFalse(m VariableMatcher) VariableMatcher {
	return alwaysFalse{
		Matcher: m,
	}
}

// AlwaysFalseNode is mainly used for unit testing to make a Node immediately.
func AlwaysFalseNode(n Node) Node {
	return alwaysFalse{
		InnerNode: n,
		Matcher:   nil,
	}
}

// UseAs uses a type no one supports to always override with false.
func (alwaysFalse) UseAs() Node { return alwaysFalse{} }
func (f alwaysFalse) ConvertVariable(rego ast.Ref) (Node, bool) {
	if f.Matcher != nil {
		n, ok := f.Matcher.ConvertVariable(rego)
		if ok {
			return alwaysFalse{
				Matcher:   f.Matcher,
				InnerNode: n,
			}, true
		}
	}

	return nil, false
}

func (alwaysFalse) SQLString(_ *SQLGenerator) string {
	return "false"
}

func (alwaysFalse) ContainsSQL(_ *SQLGenerator, _ Node) (string, error) {
	return "false", nil
}

func (alwaysFalse) ContainedInSQL(_ *SQLGenerator, _ Node) (string, error) {
	return "false", nil
}

func (alwaysFalse) EqualsSQLString(_ *SQLGenerator, _ bool, _ Node) (string, error) {
	return "false", nil
}
