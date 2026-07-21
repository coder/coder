package rbac

import (
	"github.com/open-policy-agent/opa/ast"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// orgPermissionSetThreshold is the number of organizations a subject must belong
// to before partial evaluation (Prepare) uses the set-based org-permission map
// construction. Below it, the original per-membership construction has lower
// fixed overhead and is faster; at and above it, the set-based construction's
// linear (rather than quadratic) scaling wins. The crossover is ~10 orgs, see
// BenchmarkRBACManyOrgs.
const orgPermissionSetThreshold = 10

// regoInputValue returns a rego input value for the given subject, action, and
// object. This rego input is already parsed and can be used directly in a
// rego query.
func regoInputValue(subject Subject, action policy.Action, object Object) (ast.Value, error) {
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
	// use_org_perm_sets is always false for full evaluation: the set-based
	// org-permission construction only helps the partial-evaluation (Prepare)
	// path, so full evaluation always uses the original construction.
	u := [2]*ast.Term{
		ast.StringTerm("use_org_perm_sets"),
		ast.BooleanTerm(false),
	}

	input := ast.NewObject(s, a, o, u)

	return input, nil
}

// regoPartialInputValue is the same as regoInputValue but only includes the
// object type. This is for partial evaluations.
func regoPartialInputValue(subject Subject, action policy.Action, objectType string) (ast.Value, error) {
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
		ast.NewTerm(ast.NewObject(
			[2]*ast.Term{
				ast.StringTerm("type"),
				ast.StringTerm(objectType),
			}),
		),
	}
	// use_org_perm_sets selects the set-based org-permission construction. It is
	// worthwhile only for partial evaluation (this function) once the subject
	// belongs to enough organizations, so the value is (org count >= threshold).
	// Computing the comparison here and passing a plain boolean lets OPA prune
	// the unused branch at compile time; an `input.org_count >= N` comparison in
	// the policy does not fold as reliably during partial evaluation and would
	// leave both branches in the residual.
	orgCount, err := subject.orgMembershipCount()
	if err != nil {
		return nil, xerrors.Errorf("org membership count: %w", err)
	}
	u := [2]*ast.Term{
		ast.StringTerm("use_org_perm_sets"),
		ast.BooleanTerm(orgCount >= orgPermissionSetThreshold),
	}

	input := ast.NewObject(s, a, o, u)

	return input, nil
}

// orgMembershipCount returns the number of distinct organizations the subject
// belongs to. This mirrors how the policy derives `org_memberships` from
// `input.subject.roles[_].by_org_id`, and is used to decide whether partial
// evaluation should use the set-based org-permission construction. Expanding
// concrete roles is a no-op, so this is cheap on the request path.
func (s Subject) orgMembershipCount() (int, error) {
	roles, err := s.Roles.Expand()
	if err != nil {
		return 0, xerrors.Errorf("expand roles: %w", err)
	}

	orgs := make(map[string]struct{})
	for _, role := range roles {
		for orgID := range role.ByOrgID {
			orgs[orgID] = struct{}{}
		}
	}
	return len(orgs), nil
}

// regoValue returns the ast.Object representation of the subject.
func (s Subject) regoValue() (ast.Value, error) {
	if s.cachedASTValue != nil {
		return s.cachedASTValue, nil
	}

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
		userACL.Insert(ast.StringTerm(k), ast.NewTerm(regoSliceString(v...)))
	}
	grpACL := ast.NewObject()
	for k, v := range z.ACLGroupList {
		grpACL.Insert(ast.StringTerm(k), ast.NewTerm(regoSliceString(v...)))
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
			ast.StringTerm("any_org"),
			ast.BooleanTerm(z.AnyOrgOwner),
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

// withCachedRegoValue returns a copy of the role with the cachedRegoValue.
// It does not mutate the underlying role.
// Avoid using this function if possible, it should only be used if the
// caller can guarantee the role is static and will never change.
func (role Role) withCachedRegoValue() Role {
	tmp := role
	tmp.cachedRegoValue = role.regoValue()
	return tmp
}

func (role Role) regoValue() ast.Value {
	if role.cachedRegoValue != nil {
		return role.cachedRegoValue
	}
	byOrgIDMap := ast.NewObject()
	for k, p := range role.ByOrgID {
		byOrgIDMap.Insert(ast.StringTerm(k), ast.NewTerm(
			ast.NewObject(
				[2]*ast.Term{
					ast.StringTerm("org"),
					ast.NewTerm(regoSlice(p.Org)),
				},
				[2]*ast.Term{
					ast.StringTerm("member"),
					ast.NewTerm(regoSlice(p.Member)),
				},
			),
		))
	}
	return ast.NewObject(
		[2]*ast.Term{
			ast.StringTerm("site"),
			ast.NewTerm(regoSlice(role.Site)),
		},
		[2]*ast.Term{
			ast.StringTerm("user"),
			ast.NewTerm(regoSlice(role.User)),
		},
		[2]*ast.Term{
			ast.StringTerm("by_org_id"),
			ast.NewTerm(byOrgIDMap),
		},
	)
}

func (s Scope) regoValue() ast.Value {
	r, ok := s.Role.regoValue().(ast.Object)
	if !ok {
		panic("developer error: role is not an object")
	}

	terms := make([]*ast.Term, len(s.AllowIDList))
	for i, v := range s.AllowIDList {
		terms[i] = ast.NewTerm(ast.NewObject(
			[2]*ast.Term{
				ast.StringTerm("type"),
				ast.StringTerm(v.Type),
			},
			[2]*ast.Term{
				ast.StringTerm("id"),
				ast.StringTerm(v.ID),
			},
		),
		)
	}

	r.Insert(
		ast.StringTerm("allow_list"),
		ast.NewTerm(ast.NewArray(terms...)),
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

type regoValue interface {
	regoValue() ast.Value
}

// regoSlice returns the ast.Array representation of the slice.
// The slice must contain only types that implement the regoValue interface.
func regoSlice[T regoValue](slice []T) *ast.Array {
	terms := make([]*ast.Term, len(slice))
	for i, v := range slice {
		terms[i] = ast.NewTerm(v.regoValue())
	}
	return ast.NewArray(terms...)
}

func regoSliceString[T ~string](slice ...T) *ast.Array {
	terms := make([]*ast.Term, len(slice))
	for i, v := range slice {
		terms[i] = ast.StringTerm(string(v))
	}
	return ast.NewArray(terms...)
}
