package sqltypes

import (
	"fmt"
	"strconv"
)

// AstBoolean is a literal true/false value.
type AstBoolean struct {
	Source RegoSource
	Value  bool
}

func Bool(t bool) BooleanNode {
	return AstBoolean{Value: t, Source: RegoSource(strconv.FormatBool(t))}
}

func (AstBoolean) IsBooleanNode() {}
func (AstBoolean) UseAs() Node    { return AstBoolean{} }

func (b AstBoolean) SQLString(_ *SQLGenerator) string {
	return strconv.FormatBool(b.Value)
}

func (b AstBoolean) EqualsSQLString(cfg *SQLGenerator, not bool, other Node) (string, error) {
	switch other.UseAs().(type) {
	case BooleanNode:
		bn, ok := other.(BooleanNode)
		if !ok {
			return "", fmt.Errorf("not a boolean node: %T", other)
		}

		return fmt.Sprintf("%s %s %s",
			b.SQLString(cfg),
			equalsOp(not),
			BoolParenthesis(bn).SQLString(cfg),
		), nil
	}

	return "", fmt.Errorf("unsupported equality: %T %s %T", b, equalsOp(not), other)

}
