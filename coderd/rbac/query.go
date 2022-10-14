package rbac

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"golang.org/x/xerrors"
)

type TermType string

const (
	VarTypeJsonbTextArray TermType = "jsonb-text-array"
	VarTypeText           TermType = "text"
	VarTypeBoolean        TermType = "boolean"
	// VarTypeSkip means this variable does not exist to use.
	VarTypeSkip TermType = "skip"
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
	// query is `variable ? 'value'` instead of `'value' = ANY(variable)`.
	// This type is only needed to be provided
	Type TermType
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
	//				Type: VarTypeJsonbTextArray
	//		}
	Variables []SQLColumn
}

func DefaultConfig() SQLConfig {
	return SQLConfig{
		Variables: []SQLColumn{
			{
				RegoMatch:    regexp.MustCompile(`^input\.object\.acl_group_list\.?(.*)$`),
				ColumnSelect: "group_acl->$1",
				Type:         VarTypeJsonbTextArray,
			},
			{
				RegoMatch:    regexp.MustCompile(`^input\.object\.acl_user_list\.?(.*)$`),
				ColumnSelect: "user_acl->$1",
				Type:         VarTypeJsonbTextArray,
			},
			{
				RegoMatch:    regexp.MustCompile(`^input\.object\.org_owner$`),
				ColumnSelect: "organization_id :: text",
				Type:         VarTypeText,
			},
			{
				RegoMatch:    regexp.MustCompile(`^input\.object\.owner$`),
				ColumnSelect: "owner_id :: text",
				Type:         VarTypeText,
			},
		},
	}
}

func NoACLConfig() SQLConfig {
	return SQLConfig{
		Variables: []SQLColumn{
			{
				RegoMatch:    regexp.MustCompile(`^input\.object\.acl_group_list\.?(.*)$`),
				ColumnSelect: "",
				Type:         VarTypeSkip,
			},
			{
				RegoMatch:    regexp.MustCompile(`^input\.object\.acl_user_list\.?(.*)$`),
				ColumnSelect: "",
				Type:         VarTypeSkip,
			},
			{
				RegoMatch:    regexp.MustCompile(`^input\.object\.org_owner$`),
				ColumnSelect: "organization_id :: text",
				Type:         VarTypeText,
			},
			{
				RegoMatch:    regexp.MustCompile(`^input\.object\.owner$`),
				ColumnSelect: "owner_id :: text",
				Type:         VarTypeText,
			},
		},
	}
}

type AuthorizeFilter interface {
	Expression
	// Eval is required for the fake in memory database to work. The in memory
	// database can use this function to filter the results.
	Eval(object Object) bool
}

// expressionTop handles Eval(object Object) for in memory expressions
type expressionTop struct {
	Expression
	Auth *PartialAuthorizer
}

func (e expressionTop) Eval(object Object) bool {
	return e.Auth.Authorize(context.Background(), object) == nil
}

// Compile will convert a rego query AST into our custom types. The output is
// an AST that can be used to generate SQL.
func Compile(pa *PartialAuthorizer) (AuthorizeFilter, error) {
	partialQueries := pa.partialQueries
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
	exp := expOr{
		base: base{
			Rego: builder.String(),
		},
		Expressions: result,
	}
	return expressionTop{
		Expression: &exp,
		Auth:       pa,
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
			base:     base,
			Needle:   terms[0],
			Haystack: terms[1],
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
	termBase := base{Rego: term.String()}
	switch v := term.Value.(type) {
	case ast.Boolean:
		return &termBoolean{
			base:  termBase,
			Value: bool(v),
		}, nil
	case ast.Ref:
		obj := &termObject{
			base: termBase,
			Path: []Term{},
		}
		var idx int
		// A ref is a set of terms. If the first term is a var, then the
		// following terms are the path to the value.
		isRef := true
		var builder strings.Builder
		for _, term := range v {
			if idx == 0 {
				if _, ok := v[0].Value.(ast.Var); !ok {
					return nil, xerrors.Errorf("invalid term (%s): ref must start with a var, started with %T", v[0].String(), v[0])
				}
			}

			_, newRef := term.Value.(ast.Ref)
			if newRef ||
				// This is an unfortunate hack. To fix this, we need to rewrite
				// our SQL config as a path ([]string{"input", "object", "acl_group"}).
				// In the rego AST, there is no difference between selecting
				// a field by a variable, and selecting a field by a literal (string).
				// This was a misunderstanding.
				// Example (these are equivalent by AST):
				//	input.object.acl_group_list['4d30d4a8-b87d-45ac-b0d4-51b2e68e7e75']
				//	input.object.acl_group_list.organization_id
				//
				// This is not equivalent
				//	input.object.acl_group_list[input.object.organization_id]
				//
				// If this becomes even more hairy, we should fix the sql config.
				builder.String() == "input.object.acl_group_list" ||
				builder.String() == "input.object.acl_user_list" {
				if !newRef {
					isRef = false
				}
				// New obj
				obj.Path = append(obj.Path, termVariable{
					base: base{
						Rego: builder.String(),
					},
					Name: builder.String(),
				})
				builder.Reset()
				idx = 0
			}

			if builder.Len() != 0 {
				builder.WriteString(".")
			}
			builder.WriteString(trimQuotes(term.String()))
			idx++
		}

		if isRef {
			obj.Path = append(obj.Path, termVariable{
				base: base{
					Rego: builder.String(),
				},
				Name: builder.String(),
			})
		} else {
			obj.Path = append(obj.Path, termString{
				base: base{
					Rego: fmt.Sprintf("%q", builder.String()),
				},
				Value: builder.String(),
			})
		}
		return obj, nil
	case ast.Var:
		return &termVariable{
			Name: trimQuotes(v.String()),
			base: termBase,
		}, nil
	case ast.String:
		return &termString{
			Value: trimQuotes(v.String()),
			base:  termBase,
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
			base:  termBase,
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
	// RegoString is used in debugging to see the original rego expression.
	RegoString() string
	// SQLString returns the SQL expression that can be used in a WHERE clause.
	SQLString(cfg SQLConfig) string
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

// opInternalMember2 is checking if the first term is a member of the second term.
// The second term is a set or list.
type opInternalMember2 struct {
	base
	Needle   Term
	Haystack Term
}

func (t opInternalMember2) SQLString(cfg SQLConfig) string {
	if haystack, ok := t.Haystack.(*termObject); ok {
		// This is a special case where the haystack is a jsonb array.
		// There is a more general way to solve this, but that requires a lot
		// more code to cover a lot more cases that we do not care about.
		// To handle this more generally we should implement "Array" as a type.
		// Then have the `contains` function on the Array type. This would defer
		// knowing the element type to the Array and cover more cases without
		// having to add more "if" branches here.
		// But until we need more cases, our basic type system is ok, and
		// this is the only case we need to handle.
		sqlType := haystack.SQLType(cfg)
		if sqlType == VarTypeJsonbTextArray {
			return fmt.Sprintf("%s ? %s", haystack.SQLString(cfg), t.Needle.SQLString(cfg))
		}

		if sqlType == VarTypeSkip {
			return "true"
		}
	}

	return fmt.Sprintf("%s = ANY(%s)", t.Needle.SQLString(cfg), t.Haystack.SQLString(cfg))
}

// Term is a single value in an expression. Terms can be variables or constants.
//
// Eg: "f9d6fb75-b59b-4363-ab6b-ae9d26b679d7", "input.object.org_owner",
// "{"f9d6fb75-b59b-4363-ab6b-ae9d26b679d7"}"
type Term interface {
	RegoString() string
	SQLString(cfg SQLConfig) string
	SQLType(cfg SQLConfig) TermType
}

type termString struct {
	base
	Value string
}

func (t termString) SQLString(_ SQLConfig) string {
	return "'" + t.Value + "'"
}

func (termString) SQLType(_ SQLConfig) TermType {
	return VarTypeText
}

// termObject is a variable that can be dereferenced. We count some rego objects
// as single variables, eg: input.object.org_owner. In reality, it is a nested
// object.
// In rego, we can dereference the object with the "." operator, which we can
// handle with regex.
// Or we can dereference the object with the "[]", which we can handle with this
// term type.
type termObject struct {
	base
	Path []Term
}

func (t termObject) SQLType(cfg SQLConfig) TermType {
	// Without a full type system, let's just assume the type of the first var
	// is the resulting type. This is correct for our use case.
	// Solving this more generally requires a full type system, which is
	// excessive for our mostly static policy.
	return t.Path[0].SQLType(cfg)
}

func (t termObject) SQLString(cfg SQLConfig) string {
	if len(t.Path) == 1 {
		return t.Path[0].SQLString(cfg)
	}
	// Combine the last 2 variables into 1 variable.
	end := t.Path[len(t.Path)-1]
	before := t.Path[len(t.Path)-2]

	// Recursively solve the SQLString by removing the last nested reference.
	// This continues until we have a single variable.
	return termObject{
		base: t.base,
		Path: append(
			t.Path[:len(t.Path)-2],
			termVariable{
				base: base{
					Rego: before.RegoString() + "[" + end.RegoString() + "]",
				},
				// Convert the end to SQL string. We evaluate each term
				// one at a time.
				Name: before.RegoString() + "." + end.SQLString(cfg),
			},
		),
	}.SQLString(cfg)
}

type termVariable struct {
	base
	Name string
}

func (t termVariable) SQLType(cfg SQLConfig) TermType {
	if col := t.ColumnConfig(cfg); col != nil {
		return col.Type
	}
	return VarTypeText
}

func (t termVariable) SQLString(cfg SQLConfig) string {
	if col := t.ColumnConfig(cfg); col != nil {
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

// ColumnConfig returns the correct SQLColumn settings for the
// term. If there is no configured column, it will return nil.
func (t termVariable) ColumnConfig(cfg SQLConfig) *SQLColumn {
	for _, col := range cfg.Variables {
		matches := col.RegoMatch.MatchString(t.Name)
		if matches {
			return &col
		}
	}
	return nil
}

// termSet is a set of unique terms.
type termSet struct {
	base
	Value []Term
}

func (t termSet) SQLType(cfg SQLConfig) TermType {
	if len(t.Value) == 0 {
		return VarTypeText
	}
	// Without a full type system, let's just assume the type of the first var
	// is the resulting type. This is correct for our use case.
	// Solving this more generally requires a full type system, which is
	// excessive for our mostly static policy.
	return t.Value[0].SQLType(cfg)
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

func (termBoolean) SQLType(SQLConfig) TermType {
	return VarTypeBoolean
}

func (t termBoolean) Eval(_ Object) bool {
	return t.Value
}

func (t termBoolean) SQLString(_ SQLConfig) string {
	return strconv.FormatBool(t.Value)
}

func trimQuotes(s string) string {
	return strings.Trim(s, "\"")
}
