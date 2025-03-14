package sqltypes
import (
	"fmt"
	"errors"
	"strings"
)
type binaryOperator int
const (
	_ binaryOperator = iota
	binaryOpOR
	binaryOpAND
)
type binaryOp struct {
	source RegoSource
	op     binaryOperator
	Terms []BooleanNode
}
func (binaryOp) UseAs() Node    { return binaryOp{} }
func (binaryOp) IsBooleanNode() {}
func Or(source RegoSource, terms ...BooleanNode) BooleanNode {
	return newBinaryOp(source, binaryOpOR, terms...)
}
func And(source RegoSource, terms ...BooleanNode) BooleanNode {
	return newBinaryOp(source, binaryOpAND, terms...)
}
func newBinaryOp(source RegoSource, op binaryOperator, terms ...BooleanNode) BooleanNode {
	if len(terms) == 0 {
		// TODO: How to handle 0 terms?
		return Bool(false)
	}
	opTerms := make([]BooleanNode, 0, len(terms))
	for i := range terms {
		// Always wrap terms in parentheses to be safe.
		opTerms = append(opTerms, BoolParenthesis(terms[i]))
	}
	if len(opTerms) == 1 {
		return opTerms[0]
	}
	return binaryOp{
		Terms:  opTerms,
		op:     op,
		source: source,
	}
}
func (b binaryOp) SQLString(cfg *SQLGenerator) string {
	sqlOp := ""
	switch b.op {
	case binaryOpOR:
		sqlOp = "OR"
	case binaryOpAND:
		sqlOp = "AND"
	default:
		cfg.AddError(fmt.Errorf("unsupported binary operator: %s (%d)", b.source, b.op))
		return "BinaryOpError"
	}
	terms := make([]string, 0, len(b.Terms))
	for _, term := range b.Terms {
		termSQL := term.SQLString(cfg)
		terms = append(terms, termSQL)
	}
	return strings.Join(terms, " "+sqlOp+" ")
}
