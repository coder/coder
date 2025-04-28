package regosql

import (
	"encoding/json"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/regosql/sqltypes"
)

// ConvertConfig is required to generate SQL from the rego queries.
type ConvertConfig struct {
	// VariableConverter is called each time a var is encountered. This creates
	// the SQL ast for the variable. Without this, the SQL generator does not
	// know how to convert rego variables into SQL columns.
	VariableConverter sqltypes.VariableMatcher
}

// ConvertRegoAst converts partial rego queries into a single SQL where
// clause. If the query equates to "true" then the user should have access.
func ConvertRegoAst(cfg ConvertConfig, partial *rego.PartialQueries) (sqltypes.BooleanNode, error) {
	if len(partial.Queries) == 0 {
		// Always deny if there are no queries. This means there is no possible
		// way this user can access these resources.
		return sqltypes.Bool(false), nil
	}

	for _, q := range partial.Queries {
		// An empty query in rego means "true". If any query in the set is
		// empty, then the user should have access.
		if len(q) == 0 {
			// Always allow
			return sqltypes.Bool(true), nil
		}
	}

	var queries []sqltypes.BooleanNode
	var builder strings.Builder
	for i, q := range partial.Queries {
		converted, err := convertQuery(cfg, q)
		if err != nil {
			return nil, xerrors.Errorf("query %s: %w", q.String(), err)
		}

		if i != 0 {
			_, _ = builder.WriteString("\n")
		}
		_, _ = builder.WriteString(q.String())
		queries = append(queries, converted)
	}

	// All queries are OR'd together. This means that if any query is true,
	// then the user should have access.
	sqlClause := sqltypes.Or(sqltypes.RegoSource(builder.String()), queries...)
	// Always wrap in parens to ensure the correct precedence when combining with other
	// SQL clauses.
	return sqltypes.BoolParenthesis(sqlClause), nil
}

func convertQuery(cfg ConvertConfig, q ast.Body) (sqltypes.BooleanNode, error) {
	var expressions []sqltypes.BooleanNode
	for _, e := range q {
		exp, err := convertExpression(cfg, e)
		if err != nil {
			return nil, xerrors.Errorf("expression %s: %w", e.String(), err)
		}

		expressions = append(expressions, exp)
	}

	// All expressions in a single query are AND'd together. This means that
	// all expressions must be true for the user to have access.
	return sqltypes.And(sqltypes.RegoSource(q.String()), expressions...), nil
}

func convertExpression(cfg ConvertConfig, e *ast.Expr) (sqltypes.BooleanNode, error) {
	if e.IsCall() {
		//nolint:forcetypeassert
		n, err := convertCall(cfg, e.Terms.([]*ast.Term))
		if err != nil {
			return nil, xerrors.Errorf("call: %w", err)
		}

		boolN, ok := n.(sqltypes.BooleanNode)
		if !ok {
			return nil, xerrors.Errorf("call %q: not a boolean expression", e.String())
		}
		return boolN, nil
	}

	// If it's not a call, it is a single term
	if term, ok := e.Terms.(*ast.Term); ok {
		ty, err := convertTerm(cfg, term)
		if err != nil {
			return nil, xerrors.Errorf("convert term %s: %w", term.String(), err)
		}

		tyBool, ok := ty.(sqltypes.BooleanNode)
		if !ok {
			return nil, xerrors.Errorf("convert term %s is not a boolean: %w", term.String(), err)
		}

		return tyBool, nil
	}

	return nil, xerrors.Errorf("expression %s not supported", e.String())
}

// convertCall converts a function call to a SQL expression.
func convertCall(cfg ConvertConfig, call ast.Call) (sqltypes.Node, error) {
	if len(call) == 0 {
		return nil, xerrors.Errorf("empty call")
	}

	// Operator is the first term
	op := call[0]
	var args []*ast.Term
	if len(call) > 1 {
		args = call[1:]
	}

	opString := op.String()
	// Supported operators.
	switch op.String() {
	case "neq", "eq", "equals", "equal":
		args, err := convertTerms(cfg, args, 2)
		if err != nil {
			return nil, xerrors.Errorf("arguments: %w", err)
		}

		not := false
		if opString == "neq" || opString == "notequals" || opString == "notequal" {
			not = true
		}

		equality := sqltypes.Equality(not, args[0], args[1])
		return sqltypes.BoolParenthesis(equality), nil
	case "internal.member_2":
		args, err := convertTerms(cfg, args, 2)
		if err != nil {
			return nil, xerrors.Errorf("arguments: %w", err)
		}

		member := sqltypes.MemberOf(args[0], args[1])
		return sqltypes.BoolParenthesis(member), nil
	default:
		return nil, xerrors.Errorf("operator %s not supported", op)
	}
}

func convertTerms(cfg ConvertConfig, terms []*ast.Term, expected int) ([]sqltypes.Node, error) {
	if len(terms) != expected {
		return nil, xerrors.Errorf("expected %d terms, got %d", expected, len(terms))
	}

	result := make([]sqltypes.Node, 0, len(terms))
	for _, t := range terms {
		term, err := convertTerm(cfg, t)
		if err != nil {
			return nil, xerrors.Errorf("term: %w", err)
		}
		result = append(result, term)
	}

	return result, nil
}

func convertTerm(cfg ConvertConfig, term *ast.Term) (sqltypes.Node, error) {
	source := sqltypes.RegoSource(term.String())
	switch t := term.Value.(type) {
	case ast.Var:
		// All vars should be contained in ast.Ref's.
		return nil, xerrors.New("var not yet supported")
	case ast.Ref:
		if len(t) == 0 {
			// A reference with no text is a variable with no name?
			// This makes no sense.
			return nil, xerrors.New("empty ref not supported")
		}

		if cfg.VariableConverter == nil {
			return nil, xerrors.New("no variable converter provided to handle variables")
		}

		// The structure of references is as follows:
		// 1. All variables start with a regoAst.Var as the first term.
		// 2. The next term is either a regoAst.String or a regoAst.Var.
		//	- regoAst.String if a static field name or index.
		//	- regoAst.Var if the field reference is a variable itself. Such as
		//    the wildcard "[_]"
		// 3. Repeat 1-2 until the end of the reference.
		node, ok := cfg.VariableConverter.ConvertVariable(t)
		if !ok {
			return nil, xerrors.Errorf("variable %q cannot be converted", t.String())
		}
		return node, nil
	case ast.String:
		return sqltypes.String(string(t)), nil
	case ast.Number:
		return sqltypes.Number(source, json.Number(t)), nil
	case ast.Boolean:
		return sqltypes.Bool(bool(t)), nil
	case *ast.Array:
		elems := make([]sqltypes.Node, 0, t.Len())
		for i := 0; i < t.Len(); i++ {
			value, err := convertTerm(cfg, t.Elem(i))
			if err != nil {
				return nil, xerrors.Errorf("array element %d in %q: %w", i, t.String(), err)
			}
			elems = append(elems, value)
		}
		return sqltypes.Array(source, elems...)
	case ast.Object:
		return nil, xerrors.New("object not yet supported")
	case ast.Set:
		// Just treat a set like an array for now.
		arr := t.Sorted()
		return convertTerm(cfg, &ast.Term{
			Value:    arr,
			Location: term.Location,
		})
	case ast.Call:
		// This is a function call
		return convertCall(cfg, t)
	default:
		return nil, xerrors.Errorf("%T not yet supported", t)
	}
}
