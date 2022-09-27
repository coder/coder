package rbac

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"golang.org/x/xerrors"
)

// Compile will convert a rego query AST into our custom types. The output is
// an AST that can be used to generate SQL.
func Compile(partialQueries *rego.PartialQueries) (Expression, error) {
	if len(partialQueries.Support) > 0 {
		return nil, xerrors.Errorf("cannot convert support rules, expect 0 found %d", len(partialQueries.Support))
	}

	// 0 queries means the result is "undefined". This is the same as "false".
	if len(partialQueries.Queries) == 0 {
		return &termBoolean{
			base:  base{Rego: "false"},
			Value: false,
		}, nil
	}

	// Abort early if any of the "OR"'d expressions are the empty string.
	// This is the same as "true".
	for _, query := range partialQueries.Queries {
		if query.String() == "" {
			return &termBoolean{
				base:  base{Rego: "true"},
				Value: true,
			}, nil
		}
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
	return expOr{
		base: base{
			Rego: builder.String(),
		},
		Expressions: result,
	}, nil
}

// processQuery processes an entire set of expressions and joins them with
// "AND".
func processQuery(query ast.Body) (Expression, error) {
	expressions := make([]Expression, 0, len(query))
	for _, astExpr := range query {
		expr, err := processExpression(astExpr)
		if err != nil {
			return nil, err
		}
		expressions = append(expressions, expr)
	}

	return expAnd{
		base: base{
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
	base := base{Rego: op}
	switch op {
	case "neq", "eq", "equal":
		terms, err := processTerms(2, expr.Operands())
		if err != nil {
			return nil, xerrors.Errorf("invalid '%s' expression: %w", op, err)
		}
		return &opEqual{
			base:  base,
			Terms: [2]Term{terms[0], terms[1]},
			Not:   op == "neq",
		}, nil
	case "internal.member_2":
		terms, err := processTerms(2, expr.Operands())
		if err != nil {
			return nil, xerrors.Errorf("invalid '%s' expression: %w", op, err)
		}
		return &opInternalMember2{
			base:  base,
			Terms: [2]Term{terms[0], terms[1]},
		}, nil
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
	base := base{Rego: term.String()}
	switch v := term.Value.(type) {
	case ast.Ref:
		// A ref is a set of terms. If the first term is a var, then the
		// following terms are the path to the value.
		if v0, ok := v[0].Value.(ast.Var); ok {
			name := trimQuotes(v0.String())
			for _, p := range v[1:] {
				name += "." + trimQuotes(p.String())
			}
			return &termVariable{
				base: base,
				Name: name,
			}, nil
		} else {
			return nil, xerrors.Errorf("invalid term: ref must start with a var, started with %T", v[0])
		}
	case ast.Var:
		return &termVariable{
			Name: trimQuotes(v.String()),
			base: base,
		}, nil
	case ast.String:
		return &termString{
			Value: trimQuotes(v.String()),
			base:  base,
		}, nil
	case ast.Set:
		return &termSet{
			Value: v,
			base:  base,
		}, nil
	default:
		return nil, xerrors.Errorf("invalid term: %T not supported, %q", v, term.String())
	}
}

type base struct {
	// Rego is the original rego string
	Rego string
}

func (b base) RegoString() string {
	return b.Rego
}

// Expression comprises a set of terms, operators, and functions. All
// expressions return a boolean value.
//
// Eg: neq(input.object.org_owner, "") AND input.object.org_owner == "foo"
type Expression interface {
	RegoString() string
	SQLString() string
}

type expAnd struct {
	base
	Expressions []Expression
}

func (t expAnd) SQLString() string {
	exprs := make([]string, 0, len(t.Expressions))
	for _, expr := range t.Expressions {
		exprs = append(exprs, expr.SQLString())
	}
	return "(" + strings.Join(exprs, " AND ") + ")"
}

type expOr struct {
	base
	Expressions []Expression
}

func (t expOr) SQLString() string {
	exprs := make([]string, 0, len(t.Expressions))
	for _, expr := range t.Expressions {
		exprs = append(exprs, expr.SQLString())
	}

	return "(" + strings.Join(exprs, " OR ") + ")"
}

// Operator joins terms together to form an expression.
// Operators are also expressions.
//
// Eg: "=", "neq", "internal.member_2", etc.
type Operator interface {
	RegoString() string
	SQLString() string
}

type opEqual struct {
	base
	Terms [2]Term
	// For NotEqual
	Not bool
}

func (t opEqual) SQLString() string {
	op := "="
	if t.Not {
		op = "!="
	}
	return fmt.Sprintf("%s %s %s", t.Terms[0].SQLString(), op, t.Terms[1].SQLString())
}

// opInternalMember2 is checking if the first term is a member of the second term.
// The second term is a set or list.
type opInternalMember2 struct {
	base
	Terms [2]Term
}

func (t opInternalMember2) SQLString() string {
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

type termString struct {
	base
	Value string
}

func (t termString) SQLString() string {
	return "'" + t.Value + "'"
}

type termVariable struct {
	base
	Name string
}

func (t termVariable) SQLString() string {
	return t.Name
}

// termSet is a set of unique terms.
type termSet struct {
	base
	Value ast.Set
}

func (t termSet) SQLString() string {
	values := t.Value.Slice()
	elems := make([]string, 0, len(values))
	// TODO: Handle different typed terms?
	for _, v := range t.Value.Slice() {
		t, err := processTerm(v)
		if err != nil {
			panic(err)
		}
		elems = append(elems, t.SQLString())
	}

	return fmt.Sprintf("ARRAY [%s]", strings.Join(elems, ","))
}

type termBoolean struct {
	base
	Value bool
}

func (t termBoolean) SQLString() string {
	return strconv.FormatBool(t.Value)
}

func trimQuotes(s string) string {
	return strings.Trim(s, "\"")
}
