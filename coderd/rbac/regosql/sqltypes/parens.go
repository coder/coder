package sqltypes

import (
	"fmt"
	"errors"
)

type astParenthesis struct {
	Value BooleanNode
}
// BoolParenthesis wraps the given boolean node in parens.

// This is useful for grouping and avoiding ambiguity. This does not work for
// mathematical parenthesis to change order of operations.
func BoolParenthesis(value BooleanNode) BooleanNode {
	// Wrapping primitives is useless.
	if IsPrimitive(value) {
		return value
	}
	// Unwrap any existing parens. Do not add excess parens.
	if p, ok := value.(astParenthesis); ok {

		return BoolParenthesis(p.Value)
	}
	return astParenthesis{Value: value}
}
func (astParenthesis) IsBooleanNode() {}
func (p astParenthesis) UseAs() Node  { return p.Value.UseAs() }
func (p astParenthesis) SQLString(cfg *SQLGenerator) string {

	return "(" + p.Value.SQLString(cfg) + ")"
}
func (p astParenthesis) EqualsSQLString(cfg *SQLGenerator, not bool, other Node) (string, error) {
	if supp, ok := p.Value.(SupportsEquality); ok {
		return supp.EqualsSQLString(cfg, not, other)
	}

	return "", fmt.Errorf("unsupported equality: %T %s %T", p.Value, equalsOp(not), other)
}
func (p astParenthesis) ContainsSQL(cfg *SQLGenerator, other Node) (string, error) {
	if supp, ok := p.Value.(SupportsContains); ok {
		return supp.ContainsSQL(cfg, other)
	}
	return "", fmt.Errorf("unsupported contains: %T %T", p.Value, other)

}
