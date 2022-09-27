package rbac

import (
	"context"
	"fmt"
	"strings"

	"github.com/open-policy-agent/opa/ast"

	"golang.org/x/xerrors"

	"github.com/open-policy-agent/opa/rego"
)

// Example python: https://github.com/open-policy-agent/contrib/tree/main/data_filter_example
//

func Compile(ctx context.Context, partialQueries *rego.PartialQueries) (Expression, error) {
	if len(partialQueries.Support) > 0 {
		return nil, xerrors.Errorf("cannot convert support rules, expect 0 found %d", len(partialQueries.Support))
	}

	result := make([]Expression, 0, len(partialQueries.Queries))
	var builder strings.Builder
	for i := range partialQueries.Queries {
		query, err := processQuery(partialQueries.Queries[i])
		if err != nil {
			return nil, err
		}
		result = append(result, query)
		if i != 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(partialQueries.Queries[i].String())
	}
	return ExpOr{
		Base: Base{
			Rego: builder.String(),
		},
		Expressions: result,
	}, nil
}

func processQuery(query ast.Body) (Expression, error) {
	expressions := make([]Expression, 0, len(query))
	for _, astExpr := range query {
		expr, err := processExpression(astExpr)
		if err != nil {
			return nil, err
		}
		expressions = append(expressions, expr)
	}

	return ExpAnd{
		Base: Base{
			Rego: query.String(),
		},
		Expressions: expressions,
	}, nil
}

func processExpression(expr *ast.Expr) (Expression, error) {
	if !expr.IsCall() {
		return nil, xerrors.Errorf("invalid expression: function calls not supported")
	}

	op := expr.Operator().String()
	base := Base{Rego: op}
	switch op {
	case "neq", "eq", "equal":
		terms, err := processTerms(2, expr.Operands())
		if err != nil {
			return nil, xerrors.Errorf("invalid '%s' expression: %w", op, err)
		}
		return &OpEqual{
			Base:  base,
			Terms: [2]Term{terms[0], terms[1]},
			Not:   op == "neq",
		}, nil
	case "internal.member_2":
		terms, err := processTerms(2, expr.Operands())
		if err != nil {
			return nil, xerrors.Errorf("invalid '%s' expression: %w", op, err)
		}
		return &OpInternalMember2{
			Base:  base,
			Terms: [2]Term{terms[0], terms[1]},
		}, nil

	//case "eq", "equal":
	default:
		return nil, xerrors.Errorf("invalid expression: operator %s not supported", op)
	}
}

func processTerms(expected int, terms []*ast.Term) ([]Term, error) {
	if len(terms) != expected {
		return nil, xerrors.Errorf("too many arguments, expect %d found %d", expected, len(terms))
	}

	result := make([]Term, 0, len(terms))
	for _, term := range terms {
		processed, err := processTerm(term)
		if err != nil {
			return nil, xerrors.Errorf("invalid term: %w", err)
		}
		result = append(result, processed)
	}

	return result, nil
}

func processTerm(term *ast.Term) (Term, error) {
	base := Base{Rego: term.String()}
	switch v := term.Value.(type) {
	case ast.Ref:
		// A ref is a set of terms. If the first term is a var, then the
		// following terms are the path to the value.
		if v0, ok := v[0].Value.(ast.Var); ok {
			name := v0.String()
			for _, p := range v[1:] {
				name += "." + p.String()
			}
			return &TermVariable{
				Base: base,
				Name: name,
			}, nil
		} else {
			return nil, xerrors.Errorf("invalid term: ref must start with a var, started with %T", v[0])
		}
	case ast.Var:
		return &TermVariable{
			Name: v.String(),
			Base: base,
		}, nil
	case ast.String:
		return &TermString{
			Value: v.String(),
			Base:  base,
		}, nil
	case ast.Set:
		return &TermSet{
			Value: v,
			Base:  base,
		}, nil
	default:
		return nil, xerrors.Errorf("invalid term: %T not supported, %q", v, term.String())
	}
}

type Base struct {
	// Rego is the original rego string
	Rego string
}

func (b Base) RegoString() string {
	return b.Rego
}

// Expression comprises a set of terms, operators, and functions. All
// expressions return a boolean value.
//
// Eg: neq(input.object.org_owner, "")
type Expression interface {
	RegoString() string
	SQLString() string
}

type ExpAnd struct {
	Base
	Expressions []Expression
}

func (t ExpAnd) SQLString() string {
	exprs := make([]string, 0, len(t.Expressions))
	for _, expr := range t.Expressions {
		exprs = append(exprs, expr.SQLString())
	}
	return strings.Join(exprs, " AND ")
}

type ExpOr struct {
	Base
	Expressions []Expression
}

func (t ExpOr) SQLString() string {
	exprs := make([]string, 0, len(t.Expressions))
	for _, expr := range t.Expressions {
		exprs = append(exprs, expr.SQLString())
	}
	return strings.Join(exprs, " OR ")
}

// Operator joins terms together to form an expression.
// Operators are also expressions.
//
// Eg: "=", "neq", "internal.member_2", etc.
type Operator interface {
	RegoString() string
	SQLString() string
}

type OpEqual struct {
	Base
	Terms [2]Term
	// For NotEqual
	Not bool
}

func (t OpEqual) SQLString() string {
	op := "="
	if t.Not {
		op = "!="
	}
	return fmt.Sprintf("%s %s %s", t.Terms[0].SQLString(), op, t.Terms[1].SQLString())
}

type OpInternalMember2 struct {
	Base
	Terms [2]Term
}

func (t OpInternalMember2) SQLString() string {
	return fmt.Sprintf("%s = ANY(%s)", t.Terms[0].SQLString(), t.Terms[1].SQLString())
}

// Term is a single value in an expression. Terms can be variables or constants.
//
// Eg: "f9d6fb75-b59b-4363-ab6b-ae9d26b679d7", "input.object.org_owner",
// "{"f9d6fb75-b59b-4363-ab6b-ae9d26b679d7"}"
type Term interface {
	SQLString() string
	RegoString() string
}

type TermString struct {
	Base
	Value string
}

func (t TermString) SQLString() string {
	return t.Value
}

type TermVariable struct {
	Base
	Name string
}

func (t TermVariable) SQLString() string {
	return t.Name
}

type TermSet struct {
	Base
	Value ast.Set
}

func (t TermSet) SQLString() string {
	values := t.Value.Slice()
	elems := make([]string, 0, len(values))
	// TODO: Handle different typed terms?
	for _, v := range t.Value.Slice() {
		elems = append(elems, v.String())
	}

	return fmt.Sprintf("ARRAY [%s]", strings.Join(elems, ","))
}
