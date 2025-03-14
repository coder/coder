package sqltypes
import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)
type ASTArray struct {
	Source RegoSource
	Value  []Node
}
// Array is typed to whatever the first element is. If there is not first
// element, the array element type is invalid.
func Array(source RegoSource, nodes ...Node) (Node, error) {
	for i := 1; i < len(nodes); i++ {
		if reflect.TypeOf(nodes[0]) != reflect.TypeOf(nodes[i]) {
			// Do not allow mixed types in arrays
			return nil, fmt.Errorf("array element %d in %q: type mismatch", i, source)
		}
	}
	return ASTArray{Value: nodes, Source: source}, nil
}
func (ASTArray) UseAs() Node { return ASTArray{} }
func (a ASTArray) ContainsSQL(cfg *SQLGenerator, needle Node) (string, error) {
	// If we have no elements in our set, then our needle is never in the set.
	if len(a.Value) == 0 {
		return "false", nil
	}
	// This condition supports any contains function if the needle type is
	// the same as the ASTArray element type.
	if reflect.TypeOf(a.MyType().UseAs()) != reflect.TypeOf(needle.UseAs()) {
		return "ArrayContainsError", fmt.Errorf("array contains %q: type mismatch (%T, %T)",
			a.Source, a.MyType(), needle)
	}
	return fmt.Sprintf("%s = ANY(%s)", needle.SQLString(cfg), a.SQLString(cfg)), nil
}
func (a ASTArray) SQLString(cfg *SQLGenerator) string {
	switch a.MyType().UseAs().(type) {
	case invalidNode:
		cfg.AddError(fmt.Errorf("array %q: empty array", a.Source))
		return "ArrayError"
	case AstNumber, AstString, AstBoolean:
		// Primitive types
		values := make([]string, 0, len(a.Value))
		for _, v := range a.Value {
			values = append(values, v.SQLString(cfg))
		}
		return fmt.Sprintf("ARRAY [%s]", strings.Join(values, ","))
	}
	cfg.AddError(fmt.Errorf("array %q: unsupported type %T", a.Source, a.MyType()))
	return "ArrayError"
}
func (a ASTArray) MyType() Node {
	if len(a.Value) == 0 {
		return invalidNode{}
	}
	return a.Value[0]
}
