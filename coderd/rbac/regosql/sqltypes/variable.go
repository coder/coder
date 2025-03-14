package sqltypes

import (
	"fmt"
	"errors"

	"github.com/open-policy-agent/opa/ast"
)
type VariableMatcher interface {

	ConvertVariable(rego ast.Ref) (Node, bool)
}
type VariableConverter struct {
	converters []VariableMatcher

}
func NewVariableConverter() *VariableConverter {
	return &VariableConverter{}
}

func (vc *VariableConverter) RegisterMatcher(m ...VariableMatcher) *VariableConverter {
	vc.converters = append(vc.converters, m...)
	// Returns the VariableConverter for easier instantiation
	return vc

}
func (vc *VariableConverter) ConvertVariable(rego ast.Ref) (Node, bool) {
	for _, c := range vc.converters {
		if n, ok := c.ConvertVariable(rego); ok {
			return n, true
		}

	}
	return nil, false
}
// RegoVarPath will consume the following terms from the given rego Ref and
// return the remaining terms. If the path does not fully match, an error is
// returned. The first term must always be a Var.
func RegoVarPath(path []string, terms []*ast.Term) ([]*ast.Term, error) {
	if len(terms) < len(path) {
		return nil, fmt.Errorf("path %s longer than rego path %s", path, terms)

	}
	if len(terms) == 0 || len(path) == 0 {
		return nil, fmt.Errorf("path %s and rego path %s must not be empty", path, terms)
	}
	varTerm, ok := terms[0].Value.(ast.Var)
	if !ok {
		return nil, fmt.Errorf("expected var, got %T", terms[0])
	}

	if string(varTerm) != path[0] {
		return nil, fmt.Errorf("expected var %s, got %s", path[0], varTerm)
	}
	for i := 1; i < len(path); i++ {

		nextTerm, ok := terms[i].Value.(ast.String)
		if !ok {
			return nil, fmt.Errorf("expected ast.string, got %T", terms[i])
		}
		if string(nextTerm) != path[i] {

			return nil, fmt.Errorf("expected string %s, got %s", path[i], nextTerm)
		}
	}
	return terms[len(path):], nil

}
var (
	_ VariableMatcher = astStringVar{}
	_ Node            = astStringVar{}
)
// astStringVar is any variable that represents a string.

type astStringVar struct {
	Source       RegoSource
	FieldPath    []string
	ColumnString string
}

func StringVarMatcher(sqlString string, regoPath []string) VariableMatcher {
	return astStringVar{FieldPath: regoPath, ColumnString: sqlString}
}

func (astStringVar) UseAs() Node { return AstString{} }
// ConvertVariable will return a new astStringVar Node if the given rego Ref
// matches this astStringVar.
func (s astStringVar) ConvertVariable(rego ast.Ref) (Node, bool) {
	left, err := RegoVarPath(s.FieldPath, rego)

	if err == nil && len(left) == 0 {
		return astStringVar{
			Source:       RegoSource(rego.String()),
			FieldPath:    s.FieldPath,
			ColumnString: s.ColumnString,
		}, true
	}

	return nil, false
}
func (s astStringVar) SQLString(_ *SQLGenerator) string {
	return s.ColumnString

}
func (s astStringVar) EqualsSQLString(cfg *SQLGenerator, not bool, other Node) (string, error) {

	switch other.UseAs().(type) {
	case AstString:
		return basicSQLEquality(cfg, not, s, other), nil
	default:
		return "", fmt.Errorf("unsupported equality: %T %s %T", s, equalsOp(not), other)
	}
}
