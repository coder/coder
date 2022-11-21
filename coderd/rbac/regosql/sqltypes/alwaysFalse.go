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

func AlwaysFalse(m VariableMatcher) alwaysFalse {
	return alwaysFalse{
		Matcher: m,
	}
}

// AlwaysFalseNode is mainly used for unit testing to make a Node immediately.
func AlwaysFalseNode(n Node) alwaysFalse {
	return alwaysFalse{
		InnerNode: n,
		Matcher:   nil,
	}
}

// UseAs uses a type no one supports to always override with false.
func (f alwaysFalse) UseAs() Node { return alwaysFalse{} }
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

func (f alwaysFalse) SQLString(_ *SQLGenerator) string {
	return "false"
}

func (f alwaysFalse) ContainsSQL(_ *SQLGenerator, _ Node) (string, error) {
	return "false", nil
}

func (f alwaysFalse) ContainedInSQL(_ *SQLGenerator, _ Node) (string, error) {
	return "false", nil
}

func (f alwaysFalse) EqualsSQLString(_ *SQLGenerator, not bool, _ Node) (string, error) {
	return "false", nil
}
