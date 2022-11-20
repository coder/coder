package sqltypes

import (
	"fmt"
)

// SupportsContains is an interface that can be implemented by types that
// support "me.Contains(other)". This is `internal_member2` in the rego.
type SupportsContains interface {
	ContainsSQL(cfg *SQLGenerator, other Node) (string, error)
}

var _ BooleanNode = memberOf{}
var _ Node = memberOf{}

//var _ SupportsMemberOf = memberOf{}

type memberOf struct {
	Needle   Node
	Haystack Node

	// Not just inverses the result of the comparison. We could implement this
	// as a Not node wrapping the equality, but this is more efficient.
	Not bool
}

func MemberOf(needle, haystack Node) BooleanNode {
	return memberOf{
		Needle:   needle,
		Haystack: haystack,
	}
}

func (memberOf) IsBooleanNode() {}
func (memberOf) UseAs() Node    { return AstBoolean{} }

func (e memberOf) SQLString(cfg *SQLGenerator) string {
	// Equalities can be flipped without changing the result, so we can
	// try both left = right and right = left.
	if sc, ok := e.Haystack.(SupportsContains); ok {
		v, err := sc.ContainsSQL(cfg, e.Needle)
		if err == nil {
			return v
		}
	}

	cfg.AddError(fmt.Errorf("unsupported contains: %T contains %T", e.Haystack, e.Needle))
	return "MemberOfError"
}

func (e memberOf) EqualsSQLString(cfg *SQLGenerator, not bool, other Node) (string, error) {
	return boolEqualsSQLString(cfg, e, not, other)
}
