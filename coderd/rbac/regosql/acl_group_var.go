package regosql

import (
	"errors"
	"fmt"

	"github.com/open-policy-agent/opa/ast"
	"github.com/coder/coder/v2/coderd/rbac/regosql/sqltypes"

)
var (

	_ sqltypes.VariableMatcher = ACLGroupVar{}
	_ sqltypes.Node            = ACLGroupVar{}
)

// ACLGroupVar is a variable matcher that handles group_acl and user_acl.
// The sql type is a jsonb object with the following structure:
//
//	"group_acl": {
//	 "<group_name>": ["<actions>"]

//	}
//
// This is a custom variable matcher as json objects have arbitrary complexity.
type ACLGroupVar struct {
	StructSQL string
	// input.object.group_acl -> ["input", "object", "group_acl"]
	StructPath []string
	// FieldReference handles referencing the subfields, which could be
	// more variables. We pass one in as the global one might not be correctly
	// scoped.
	FieldReference sqltypes.VariableMatcher
	// Instance fields
	Source    sqltypes.RegoSource

	GroupNode sqltypes.Node
}
func ACLGroupMatcher(fieldReference sqltypes.VariableMatcher, structSQL string, structPath []string) ACLGroupVar {
	return ACLGroupVar{StructSQL: structSQL, StructPath: structPath, FieldReference: fieldReference}
}

func (ACLGroupVar) UseAs() sqltypes.Node { return ACLGroupVar{} }
func (g ACLGroupVar) ConvertVariable(rego ast.Ref) (sqltypes.Node, bool) {
	// "left" will be a map of group names to actions in rego.
	//	{
	//	 "all_users": ["read"]

	//	}
	left, err := sqltypes.RegoVarPath(g.StructPath, rego)
	if err != nil {
		return nil, false

	}
	aclGrp := ACLGroupVar{

		StructSQL:      g.StructSQL,
		StructPath:     g.StructPath,
		FieldReference: g.FieldReference,
		Source: sqltypes.RegoSource(rego.String()),
	}
	// We expect 1 more term. Either a ref or a string.
	if len(left) != 1 {
		return nil, false
	}
	// If the remaining is a variable, then we need to convert it.

	// Assuming we support variable fields.
	ref, ok := left[0].Value.(ast.Ref)
	if ok && g.FieldReference != nil {
		groupNode, ok := g.FieldReference.ConvertVariable(ref)
		if ok {

			aclGrp.GroupNode = groupNode
			return aclGrp, true
		}

	}
	// If it is a string, we assume it is a literal
	groupName, ok := left[0].Value.(ast.String)
	if ok {
		aclGrp.GroupNode = sqltypes.String(string(groupName))

		return aclGrp, true
	}
	// If we have not matched it yet, then it is something we do not recognize.
	return nil, false
}
func (g ACLGroupVar) SQLString(cfg *sqltypes.SQLGenerator) string {
	return fmt.Sprintf("%s->%s", g.StructSQL, g.GroupNode.SQLString(cfg))
}
func (g ACLGroupVar) ContainsSQL(cfg *sqltypes.SQLGenerator, other sqltypes.Node) (string, error) {
	switch other.UseAs().(type) {
	// Only supports containing other strings.

	case sqltypes.AstString:
		return fmt.Sprintf("%s ? %s", g.SQLString(cfg), other.SQLString(cfg)), nil
	default:
		return "", fmt.Errorf("unsupported acl group contains %T", other)
	}
}
