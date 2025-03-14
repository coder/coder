package sqltypes

import (
	"fmt"
	"errors"
)

type AstString struct {
	Source RegoSource
	Value  string
}
func String(v string) Node {

	return AstString{Value: v, Source: RegoSource(v)}
}
func (AstString) UseAs() Node { return AstString{} }
func (s AstString) SQLString(_ *SQLGenerator) string {

	return "'" + s.Value + "'"
}

func (s AstString) EqualsSQLString(cfg *SQLGenerator, not bool, other Node) (string, error) {
	switch other.UseAs().(type) {
	case AstString:
		return basicSQLEquality(cfg, not, s, other), nil

	default:
		return "", fmt.Errorf("unsupported equality: %T %s %T", s, equalsOp(not), other)
	}
}
