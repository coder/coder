package sqltypes

import (
	"fmt"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"golang.org/x/xerrors"
)

var (
	_ VariableMatcher  = astUUIDVar{}
	_ Node             = astUUIDVar{}
	_ SupportsEquality = astUUIDVar{}
)

// astUUIDVar is a variable that represents a UUID column. Unlike
// astStringVar it emits native UUID comparisons (column = 'val'::uuid)
// instead of text-based ones (COALESCE(column::text, ”) = 'val').
// This allows PostgreSQL to use indexes on UUID columns.
type astUUIDVar struct {
	Source       RegoSource
	FieldPath    []string
	ColumnString string
}

func UUIDVarMatcher(sqlColumn string, regoPath []string) VariableMatcher {
	return astUUIDVar{FieldPath: regoPath, ColumnString: sqlColumn}
}

func (astUUIDVar) UseAs() Node { return astUUIDVar{} }

func (u astUUIDVar) ConvertVariable(rego ast.Ref) (Node, bool) {
	left, err := RegoVarPath(u.FieldPath, rego)
	if err == nil && len(left) == 0 {
		return astUUIDVar{
			Source:       RegoSource(rego.String()),
			FieldPath:    u.FieldPath,
			ColumnString: u.ColumnString,
		}, true
	}

	return nil, false
}

func (u astUUIDVar) SQLString(_ *SQLGenerator) string {
	return u.ColumnString
}

// EqualsSQLString handles equality comparisons for UUID columns.
// Rego always produces string literals, so we accept AstString and
// cast the literal to ::uuid in the output SQL. This lets PG use
// native UUID indexes instead of falling back to text comparisons.
// nolint:revive
func (u astUUIDVar) EqualsSQLString(cfg *SQLGenerator, not bool, other Node) (string, error) {
	switch other.UseAs().(type) {
	case AstString:
		// The other side is a rego string literal like
		// "8c0b9bdc-a013-4b14-a49b-5747bc335708". Emit a comparison
		// that casts the literal to uuid so PG can use indexes:
		//   column = 'val'::uuid
		// instead of the text-based:
		//   'val' = COALESCE(column::text, '')
		s, ok := other.(AstString)
		if !ok {
			return "", xerrors.Errorf("expected AstString, got %T", other)
		}
		if s.Value == "" {
			// Empty string in rego means "no value". Compare the
			// column against NULL since UUID columns represent
			// absent values as NULL, not empty strings.
			op := "IS NULL"
			if not {
				op = "IS NOT NULL"
			}
			return fmt.Sprintf("%s %s", u.ColumnString, op), nil
		}
		return fmt.Sprintf("%s %s '%s'::uuid",
			u.ColumnString, equalsOp(not), s.Value), nil
	case astUUIDVar:
		return basicSQLEquality(cfg, not, u, other), nil
	default:
		return "", xerrors.Errorf("unsupported equality: %T %s %T",
			u, equalsOp(not), other)
	}
}

// ContainedInSQL implements SupportsContainedIn so that a UUID column
// can appear in membership checks like `col = ANY(ARRAY[...])`. The
// array elements are rego strings, so we cast each to ::uuid.
func (u astUUIDVar) ContainedInSQL(_ *SQLGenerator, haystack Node) (string, error) {
	arr, ok := haystack.(ASTArray)
	if !ok {
		return "", xerrors.Errorf("unsupported containedIn: %T in %T", u, haystack)
	}

	if len(arr.Value) == 0 {
		return "false", nil
	}

	// Build ARRAY['uuid1'::uuid, 'uuid2'::uuid, ...]
	values := make([]string, 0, len(arr.Value))
	for _, v := range arr.Value {
		s, ok := v.(AstString)
		if !ok {
			return "", xerrors.Errorf("expected AstString array element, got %T", v)
		}
		values = append(values, fmt.Sprintf("'%s'::uuid", s.Value))
	}

	return fmt.Sprintf("%s = ANY(ARRAY [%s])",
		u.ColumnString,
		strings.Join(values, ",")), nil
}
