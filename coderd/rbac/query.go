package rbac

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"golang.org/x/xerrors"
)

const (
	VarTypeJsonbArray = "jsonb-array"
	VarTypeUUID       = "uuid"
	VarTypeText       = "text"
)

type SQLColumn struct {
	// RegoMatch matches the original variable string.
	// If it is a match, then this variable config will apply.
	RegoMatch *regexp.Regexp
	// ColumnSelect is the name of the postgres column to select.
	// Can use capture groups from RegoMatch with $1, $2, etc.
	ColumnSelect string

	// Type indicates the postgres type of the column. Some expressions will
	// need to know this in order to determine what SQL to produce.
	// An example is if the variable is a jsonb array, the "contains" SQL
	// query is `"value"' @> variable.` instead of `'value' = ANY(variable)`.
	// This type is only needed to be provided
	Type string
}

type SQLConfig struct {
	// Variables is a map of rego variable names to SQL columns.
	//	Example:
	// 		"input\.object\.org_owner": SQLColumn{
	//			ColumnSelect: "organization_id",
	//			Type: VarTypeUUID
	//		}
	//		"input\.object\.owner": SQLColumn{
	//				ColumnSelect: "owner_id",
	//				Type: VarTypeUUID
	//		}
	//		"input\.object\.group_acl\.(.*)": SQLColumn{
	//				ColumnSelect: "group_acl->$1",
	//				Type: VarTypeJsonb
	//		}
	Variables []SQLColumn
}

func DefaultConfig() SQLConfig {
	return SQLConfig{
		Variables: []SQLColumn{
			{
				RegoMatch:    regexp.MustCompile(`^input\.object\.acl_group_list\.([^.]*)$`),
				ColumnSelect: "group_acl->$1",
				Type:         VarTypeJsonbArray,
			},
			{
				RegoMatch:    regexp.MustCompile(`^input\.object\.org_owner$`),
				ColumnSelect: "organization_id :: text",
				Type:         VarTypeUUID,
			},
			{
				RegoMatch:    regexp.MustCompile(`^input\.object\.owner$`),
				ColumnSelect: "owner_id :: text",
				Type:         VarTypeUUID,
			},
		},
	}
}

type AuthorizeFilter interface {
	// RegoString is used in debugging to see the original rego expression.
	RegoString() string
	// SQLString returns the SQL expression that can be used in a WHERE clause.
	SQLString(cfg SQLConfig) string
	// Eval is required for the fake in memory database to work. The in memory
	// database can use this function to filter the results.
	Eval(object Object) bool
}

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
		// This could be a single term that is a valid expression.
		if term, ok := expr.Terms.(*ast.Term); ok {
			value, err := processTerm(term)
			if err != nil {
				return nil, xerrors.Errorf("single term expression: %w", err)
			}
			if boolExp, ok := value.(Expression); ok {
				return boolExp, nil
			}
			// Default to error.
		}
		return nil, xerrors.Errorf("invalid expression: single non-boolean terms not supported")
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
	case ast.Boolean:
		return &termBoolean{
			base:  base,
			Value: bool(v),
		}, nil
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
		slice := v.Slice()
		set := make([]Term, 0, len(slice))
		for _, elem := range slice {
			processed, err := processTerm(elem)
			if err != nil {
				return nil, xerrors.Errorf("invalid set term %s: %w", elem.String(), err)
			}
			set = append(set, processed)
		}

		return &termSet{
			Value: set,
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
	AuthorizeFilter
}

type expAnd struct {
	base
	Expressions []Expression
}

func (t expAnd) SQLString(cfg SQLConfig) string {
	if len(t.Expressions) == 1 {
		return t.Expressions[0].SQLString(cfg)
	}

	exprs := make([]string, 0, len(t.Expressions))
	for _, expr := range t.Expressions {
		exprs = append(exprs, expr.SQLString(cfg))
	}
	return "(" + strings.Join(exprs, " AND ") + ")"
}

func (t expAnd) Eval(object Object) bool {
	for _, expr := range t.Expressions {
		if !expr.Eval(object) {
			return false
		}
	}
	return true
}

type expOr struct {
	base
	Expressions []Expression
}

func (t expOr) SQLString(cfg SQLConfig) string {
	if len(t.Expressions) == 1 {
		return t.Expressions[0].SQLString(cfg)
	}

	exprs := make([]string, 0, len(t.Expressions))
	for _, expr := range t.Expressions {
		exprs = append(exprs, expr.SQLString(cfg))
	}
	return "(" + strings.Join(exprs, " OR ") + ")"
}

func (t expOr) Eval(object Object) bool {
	for _, expr := range t.Expressions {
		if expr.Eval(object) {
			return true
		}
	}
	return false
}

// Operator joins terms together to form an expression.
// Operators are also expressions.
//
// Eg: "=", "neq", "internal.member_2", etc.
type Operator interface {
	Expression
}

type opEqual struct {
	base
	Terms [2]Term
	// For NotEqual
	Not bool
}

func (t opEqual) SQLString(cfg SQLConfig) string {
	op := "="
	if t.Not {
		op = "!="
	}
	return fmt.Sprintf("%s %s %s", t.Terms[0].SQLString(cfg), op, t.Terms[1].SQLString(cfg))
}

func (t opEqual) Eval(object Object) bool {
	a, b := t.Terms[0].EvalTerm(object), t.Terms[1].EvalTerm(object)
	if t.Not {
		return a != b
	}
	return a == b
}

// opInternalMember2 is checking if the first term is a member of the second term.
// The second term is a set or list.
type opInternalMember2 struct {
	base
	Terms [2]Term
}

func (t opInternalMember2) Eval(object Object) bool {
	a, b := t.Terms[0].EvalTerm(object), t.Terms[1].EvalTerm(object)
	bset, ok := b.([]interface{})
	if !ok {
		return false
	}
	for _, elem := range bset {
		if a == elem {
			return true
		}
	}
	return false
}

func (t opInternalMember2) SQLString(cfg SQLConfig) string {
	return fmt.Sprintf("%s = ANY(%s)", t.Terms[0].SQLString(cfg), t.Terms[1].SQLString(cfg))
}

// Term is a single value in an expression. Terms can be variables or constants.
//
// Eg: "f9d6fb75-b59b-4363-ab6b-ae9d26b679d7", "input.object.org_owner",
// "{"f9d6fb75-b59b-4363-ab6b-ae9d26b679d7"}"
type Term interface {
	RegoString() string
	SQLString(cfg SQLConfig) string
	// Eval will evaluate the term
	// Terms can eval to any type. The operator/expression will type check.
	EvalTerm(object Object) interface{}
}

type termString struct {
	base
	Value string
}

func (t termString) EvalTerm(_ Object) interface{} {
	return t.Value
}

func (t termString) SQLString(_ SQLConfig) string {
	return "'" + t.Value + "'"
}

type termVariable struct {
	base
	Name string
}

func (t termVariable) EvalTerm(obj Object) interface{} {
	switch t.Name {
	case "input.object.org_owner":
		return obj.OrgID
	case "input.object.owner":
		return obj.Owner
	case "input.object.type":
		return obj.Type
	default:
		return fmt.Sprintf("'Unknown variable %s'", t.Name)
	}
}

func (t termVariable) SQLString(cfg SQLConfig) string {
	for _, col := range cfg.Variables {
		matches := col.RegoMatch.FindStringSubmatch(t.Name)
		if len(matches) > 0 {
			// This config matches this variable.
			replace := make([]string, 0, len(matches)*2)
			for i, m := range matches {
				replace = append(replace, fmt.Sprintf("$%d", i))
				replace = append(replace, m)
			}
			replacer := strings.NewReplacer(replace...)
			return replacer.Replace(col.ColumnSelect)
		}
	}

	return t.Name
}

// termSet is a set of unique terms.
type termSet struct {
	base
	Value []Term
}

func (t termSet) EvalTerm(obj Object) interface{} {
	set := make([]interface{}, 0, len(t.Value))
	for _, term := range t.Value {
		set = append(set, term.EvalTerm(obj))
	}

	return set
}

func (t termSet) SQLString(cfg SQLConfig) string {
	elems := make([]string, 0, len(t.Value))
	for _, v := range t.Value {
		elems = append(elems, v.SQLString(cfg))
	}

	return fmt.Sprintf("ARRAY [%s]", strings.Join(elems, ","))
}

type termBoolean struct {
	base
	Value bool
}

func (t termBoolean) Eval(_ Object) bool {
	return t.Value
}

func (t termBoolean) EvalTerm(_ Object) interface{} {
	return t.Value
}

func (t termBoolean) SQLString(_ SQLConfig) string {
	return strconv.FormatBool(t.Value)
}

func trimQuotes(s string) string {
	return strings.Trim(s, "\"")
}
