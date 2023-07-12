package sqltypes

import (
	"strconv"

	"github.com/open-policy-agent/opa/ast"
)

var (
	_ Node            = constBoolean{}
	_ VariableMatcher = constBoolean{}
)

type constBoolean struct {
	Matcher  VariableMatcher
	constant bool

	InnerNode Node
}

// AlwaysFalse overrides the inner node with a constant "false".
func AlwaysFalse(m VariableMatcher) VariableMatcher {
	return constBoolean{
		Matcher:  m,
		constant: false,
	}
}

func AlwaysTrue(m VariableMatcher) VariableMatcher {
	return constBoolean{
		Matcher:  m,
		constant: true,
	}
}

// AlwaysFalseNode is mainly used for unit testing to make a Node immediately.
func AlwaysFalseNode(n Node) Node {
	return constBoolean{
		InnerNode: n,
		Matcher:   nil,
		constant:  false,
	}
}

// UseAs uses a type no one supports to always override with false.
func (constBoolean) UseAs() Node { return constBoolean{} }

func (f constBoolean) ConvertVariable(rego ast.Ref) (Node, bool) {
	if f.Matcher != nil {
		n, ok := f.Matcher.ConvertVariable(rego)
		if ok {
			return constBoolean{
				Matcher:   f.Matcher,
				InnerNode: n,
				constant:  f.constant,
			}, true
		}
	}

	return nil, false
}

func (c constBoolean) SQLString(_ *SQLGenerator) string {
	return strconv.FormatBool(c.constant)
}

func (c constBoolean) ContainsSQL(_ *SQLGenerator, _ Node) (string, error) {
	return strconv.FormatBool(c.constant), nil
}

func (c constBoolean) ContainedInSQL(_ *SQLGenerator, _ Node) (string, error) {
	return strconv.FormatBool(c.constant), nil
}

func (c constBoolean) EqualsSQLString(_ *SQLGenerator, _ bool, _ Node) (string, error) {
	return strconv.FormatBool(c.constant), nil
}
