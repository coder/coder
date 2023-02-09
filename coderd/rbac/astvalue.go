package rbac

import (
	"github.com/open-policy-agent/opa/ast"
	"golang.org/x/xerrors"
)

func regoInputValue(subject Subject, action Action, object Object) (ast.Value, error) {
	regoSubj, err := subject.regoValue()
	if err != nil {
		return nil, xerrors.Errorf("subject: %w", err)
	}

	s := [2]*ast.Term{
		ast.StringTerm("subject"),
		ast.NewTerm(regoSubj),
	}
	a := [2]*ast.Term{
		ast.StringTerm("action"),
		ast.StringTerm(string(action)),
	}
	o := [2]*ast.Term{
		ast.StringTerm("object"),
		ast.NewTerm(object.regoValue()),
	}

	input := ast.NewObject(s, a, o)

	return input, nil
}

func (s Subject) regoValue() (ast.Value, error) {
	subjRoles, err := s.Roles.Expand()
	if err != nil {
		return nil, xerrors.Errorf("expand roles: %w", err)
	}

	subjScope, err := s.Scope.Expand()
	if err != nil {
		return nil, xerrors.Errorf("expand scope: %w", err)
	}
	subj := ast.NewObject(
		[2]*ast.Term{
			ast.StringTerm("id"),
			ast.StringTerm(s.ID),
		},
		[2]*ast.Term{
			ast.StringTerm("roles"),
			ast.NewTerm(regoSlice(subjRoles)),
		},
		[2]*ast.Term{
			ast.StringTerm("scope"),
			ast.NewTerm(subjScope.regoValue()),
		},
		[2]*ast.Term{
			ast.StringTerm("groups"),
			ast.NewTerm(regoSliceString(s.Groups...)),
		},
	)

	return subj, nil
}

func (z Object) regoValue() ast.Value {
	userACL := ast.NewObject()
	for k, v := range z.ACLUserList {
		userACL.Insert(ast.StringTerm(k), ast.NewTerm(regoSlice(v)))
	}
	grpACL := ast.NewObject()
	for k, v := range z.ACLGroupList {
		grpACL.Insert(ast.StringTerm(k), ast.NewTerm(regoSlice(v)))
	}
	return ast.NewObject(
		[2]*ast.Term{
			ast.StringTerm("id"),
			ast.StringTerm(z.ID),
		},
		[2]*ast.Term{
			ast.StringTerm("owner"),
			ast.StringTerm(z.Owner),
		},
		[2]*ast.Term{
			ast.StringTerm("org_owner"),
			ast.StringTerm(z.OrgID),
		},
		[2]*ast.Term{
			ast.StringTerm("type"),
			ast.StringTerm(z.Type),
		},
		[2]*ast.Term{
			ast.StringTerm("acl_user_list"),
			ast.NewTerm(userACL),
		},
		[2]*ast.Term{
			ast.StringTerm("acl_group_list"),
			ast.NewTerm(grpACL),
		},
	)
}

func (role Role) regoValue() ast.Value {
	orgMap := ast.NewObject()
	for k, p := range role.Org {
		orgMap.Insert(ast.StringTerm(k), ast.NewTerm(regoSlice(p)))
	}
	return ast.NewObject(
		[2]*ast.Term{
			ast.StringTerm("site"),
			ast.NewTerm(regoSlice(role.Site)),
		},
		[2]*ast.Term{
			ast.StringTerm("org"),
			ast.NewTerm(orgMap),
		},
		[2]*ast.Term{
			ast.StringTerm("user"),
			ast.NewTerm(regoSlice(role.User)),
		},
	)
}

func (s Scope) regoValue() ast.Value {
	r := s.Role.regoValue().(ast.Object)
	r.Insert(
		ast.StringTerm("allow_list"),
		ast.NewTerm(regoSliceString(s.AllowIDList...)),
	)
	return r
}

func (perm Permission) regoValue() ast.Value {
	return ast.NewObject(
		[2]*ast.Term{
			ast.StringTerm("negate"),
			ast.BooleanTerm(perm.Negate),
		},
		[2]*ast.Term{
			ast.StringTerm("resource_type"),
			ast.StringTerm(perm.ResourceType),
		},
		[2]*ast.Term{
			ast.StringTerm("action"),
			ast.StringTerm(string(perm.Action)),
		},
	)
}

func (act Action) regoValue() ast.Value {
	return ast.StringTerm(string(act)).Value
}

type regoValue interface {
	regoValue() ast.Value
}

func regoSlice[T regoValue](slice []T) *ast.Array {
	terms := make([]*ast.Term, len(slice))
	for i, v := range slice {
		terms[i] = ast.NewTerm(v.regoValue())
	}
	return ast.NewArray(terms...)
}

func regoSliceString(slice ...string) *ast.Array {
	terms := make([]*ast.Term, len(slice))
	for i, v := range slice {
		terms[i] = ast.StringTerm(v)
	}
	return ast.NewArray(terms...)
}
