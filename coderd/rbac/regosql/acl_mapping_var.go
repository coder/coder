package regosql

import (
	"fmt"

	"github.com/open-policy-agent/opa/ast"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/regosql/sqltypes"
)

var (
	_ sqltypes.VariableMatcher = ACLMappingVar{}
	_ sqltypes.Node            = ACLMappingVar{}
)

// ACLMappingVar is a variable matcher that matches ACL map variables to their
// SQL storage. Usually the actual backing implementation is a pair of `jsonb`
// columns named `group_acl` and `user_acl`. Each column contains an object that
// looks like...
//
// ```json
//
//	{
//	  "<actor_id>": ["<action>", "<action>"]
//	}
//
// ```
type ACLMappingVar struct {
	// SelectSQL is used to `SELECT` the ACL mapping from the table for the
	// given resource. ie. if the full query might look like `SELECT group_acl
	// FROM things;` then you would want this to be `"group_acl"`.
	SelectSQL string
	// IndexMatcher handles variable references when indexing into the mapping.
	// (ie. `input.object.acl_group_list[input.object.org_owner]`). We need one
	// from the local context because the global one might not be correctly
	// scoped.
	IndexMatcher sqltypes.VariableMatcher
	// Used if the action list isn't directly in the ACL entry. For example, in
	// the `workspaces.group_acl` and `workspaces.user_acl` columns they're stored
	// under a `"permissions"` key.
	Subfield string

	// StructPath represents the path of the value in rego
	// ie. input.object.group_acl -> ["input", "object", "group_acl"]
	StructPath []string

	// Instance fields
	Source    sqltypes.RegoSource
	GroupNode sqltypes.Node
}

func ACLMappingMatcher(indexMatcher sqltypes.VariableMatcher, selectSQL string, structPath []string) ACLMappingVar {
	return ACLMappingVar{IndexMatcher: indexMatcher, SelectSQL: selectSQL, StructPath: structPath}
}

func (g ACLMappingVar) UsingSubfield(subfield string) ACLMappingVar {
	g.Subfield = subfield
	return g
}

func (ACLMappingVar) UseAs() sqltypes.Node { return ACLMappingVar{} }

func (g ACLMappingVar) ConvertVariable(rego ast.Ref) (sqltypes.Node, bool) {
	// left is the rego variable that maps the actor's id to the actions they
	// are allowed to take.
	// {
	//   "<actor_id>": ["<action>", "<action>"]
	// }
	left, err := sqltypes.RegoVarPath(g.StructPath, rego)
	if err != nil {
		return nil, false
	}

	aclGrp := ACLMappingVar{
		SelectSQL:    g.SelectSQL,
		IndexMatcher: g.IndexMatcher,
		Subfield:     g.Subfield,

		StructPath: g.StructPath,

		Source: sqltypes.RegoSource(rego.String()),
	}

	// We expect 1 more term. Either a ref or a string.
	if len(left) != 1 {
		return nil, false
	}

	// If the remaining is a variable, then we need to convert it.
	// Assuming we support variable fields.
	ref, ok := left[0].Value.(ast.Ref)
	if ok && g.IndexMatcher != nil {
		groupNode, ok := g.IndexMatcher.ConvertVariable(ref)
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

func (g ACLMappingVar) SQLString(cfg *sqltypes.SQLGenerator) string {
	if g.Subfield != "" {
		// We can't use subsequent -> operators because the first one might return
		// NULL, which would result in an error like "column does not exist"' from
		// the second.
		return fmt.Sprintf("%s#>array[%s, '%s']", g.SelectSQL, g.GroupNode.SQLString(cfg), g.Subfield)
	}
	return fmt.Sprintf("%s->%s", g.SelectSQL, g.GroupNode.SQLString(cfg))
}

func (g ACLMappingVar) ContainsSQL(cfg *sqltypes.SQLGenerator, other sqltypes.Node) (string, error) {
	switch other.UseAs().(type) {
	// Only supports containing other strings.
	case sqltypes.AstString:
		return fmt.Sprintf("%s ? %s", g.SQLString(cfg), other.SQLString(cfg)), nil
	default:
		return "", xerrors.Errorf("unsupported acl group contains %T", other)
	}
}
