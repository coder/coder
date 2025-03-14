package sqltypes

import (
	"fmt"
	"errors"
)

type Node interface {
	SQLString(cfg *SQLGenerator) string
	// UseAs is a helper function to allow a node to be used as a different
	// Node in operators. For example, a variable is really just a "string", so
	// having the Equality operator check for "String" or "StringVar" is just
	// excessive. Instead, we can just have the variable implement this function.
	UseAs() Node
}
// BooleanNode is a node that returns a true/false when evaluated.

type BooleanNode interface {
	Node
	IsBooleanNode()
}
type RegoSource string
type invalidNode struct{}

func (invalidNode) UseAs() Node { return invalidNode{} }
func (invalidNode) SQLString(cfg *SQLGenerator) string {

	cfg.AddError(fmt.Errorf("invalid node called"))
	return "invalid_type"

}
func IsPrimitive(n Node) bool {

	switch n.(type) {
	case AstBoolean, AstString, AstNumber:
		return true
	}
	return false

}
