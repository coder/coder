package sqltypes
import (
	"errors"
	"fmt"
)
// SupportsEquality is an interface that can be implemented by types that
// support equality with other types. We defer to other types to implement this
// as it is much easier to implement this in the context of the type.
type SupportsEquality interface {
	// EqualsSQLString intentionally returns an error. This is so if
	// left = right is not supported, we can try right = left.
	EqualsSQLString(cfg *SQLGenerator, not bool, other Node) (string, error)
}
var (
	_ BooleanNode      = equality{}
	_ Node             = equality{}
	_ SupportsEquality = equality{}
)
type equality struct {
	Left  Node
	Right Node
	// Not just inverses the result of the comparison. We could implement this
	// as a Not node wrapping the equality, but this is more efficient.
	Not bool
}
func Equality(notEquals bool, a, b Node) BooleanNode {
	return equality{
		Left:  a,
		Right: b,
		Not:   notEquals,
	}
}
func (equality) IsBooleanNode() {}
// UseAs returns an ASTBoolean as equalities resolve to boolean values
func (equality) UseAs() Node { return AstBoolean{} }
func (e equality) SQLString(cfg *SQLGenerator) string {
	// Equalities can be flipped without changing the result, so we can
	// try both left = right and right = left.
	if eq, ok := e.Left.(SupportsEquality); ok {
		v, err := eq.EqualsSQLString(cfg, e.Not, e.Right)
		if err == nil {
			return v
		}
	}
	if eq, ok := e.Right.(SupportsEquality); ok {
		v, err := eq.EqualsSQLString(cfg, e.Not, e.Left)
		if err == nil {
			return v
		}
	}
	cfg.AddError(fmt.Errorf("unsupported equality: %T %s %T", e.Left, equalsOp(e.Not), e.Right))
	return "EqualityError"
}
func (e equality) EqualsSQLString(cfg *SQLGenerator, not bool, other Node) (string, error) {
	return boolEqualsSQLString(cfg, e, not, other)
}
func boolEqualsSQLString(cfg *SQLGenerator, a BooleanNode, not bool, other Node) (string, error) {
	switch other.UseAs().(type) {
	case BooleanNode:
		bn, ok := other.(BooleanNode)
		if !ok {
			return "", fmt.Errorf("not a boolean node: %T", other)
		}
		// Always wrap both sides in parens to ensure the correct precedence.
		return fmt.Sprintf("%s %s %s",
			BoolParenthesis(a).SQLString(cfg),
			equalsOp(not),
			BoolParenthesis(bn).SQLString(cfg),
		), nil
	default:
		return "", fmt.Errorf("unsupported equality: %T %s %T", a, equalsOp(not), other)
	}
}
// nolint:revive
func equalsOp(not bool) string {
	if not {
		return "!="
	}
	return "="
}
func basicSQLEquality(cfg *SQLGenerator, not bool, a, b Node) string {
	return fmt.Sprintf("%s %s %s",
		a.SQLString(cfg),
		equalsOp(not),
		b.SQLString(cfg),
	)
}
