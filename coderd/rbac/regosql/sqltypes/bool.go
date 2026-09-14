package sqltypes

import (
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
	return boolEqualsSQLString(cfg, b, not, other)
}
