package rbac

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/open-policy-agent/opa/rego"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/tracing"
)

type Authorizer interface {
	ByRoleName(ctx context.Context, subjectID string, roleNames []string, scope Scope, action Action, object Object) error
	PrepareByRoleName(ctx context.Context, subjectID string, roleNames []string, scope Scope, action Action, objectType string) (PreparedAuthorized, error)
}

type PreparedAuthorized interface {
	Authorize(ctx context.Context, object Object) error
}

// Filter takes in a list of objects, and will filter the list removing all
// the elements the subject does not have permission for. All objects must be
// of the same type.
func Filter[O Objecter](ctx context.Context, auth Authorizer, subjID string, subjRoles []string, scope Scope, action Action, objects []O) ([]O, error) {
	ctx, span := tracing.StartSpan(ctx, trace.WithAttributes(
		attribute.String("subject_id", subjID),
		attribute.StringSlice("subject_roles", subjRoles),
		attribute.Int("num_objects", len(objects)),
	))
	defer span.End()

	if len(objects) == 0 {
		// Nothing to filter
		return objects, nil
	}
	objectType := objects[0].RBACObject().Type

	filtered := make([]O, 0)
	prepared, err := auth.PrepareByRoleName(ctx, subjID, subjRoles, scope, action, objectType)
	if err != nil {
		return nil, xerrors.Errorf("prepare: %w", err)
	}

	for _, object := range objects {
		rbacObj := object.RBACObject()
		if rbacObj.Type != objectType {
			return nil, xerrors.Errorf("object types must be uniform across the set (%s), found %s", objectType, object.RBACObject().Type)
		}
		err := prepared.Authorize(ctx, rbacObj)
		if err == nil {
			filtered = append(filtered, object)
		}
	}

	return filtered, nil
}

// RegoAuthorizer will use a prepared rego query for performing authorize()
type RegoAuthorizer struct {
	query rego.PreparedEvalQuery
}

var _ Authorizer = (*RegoAuthorizer)(nil)

// Load the policy from policy.rego in this directory.
//
//go:embed policy.rego
var policy string

const (
	rolesOkCheck = "role_ok"
	scopeOkCheck = "scope_ok"
)

func NewAuthorizer() (*RegoAuthorizer, error) {
	ctx := context.Background()
	query, err := rego.New(
		// Bind the results to 2 variables for easy checking later.
		rego.Query(
			fmt.Sprintf("%s := data.authz.role_allow "+
				"%s := data.authz.scope_allow",
				rolesOkCheck, scopeOkCheck),
		),
		rego.Module("policy.rego", policy),
	).PrepareForEval(ctx)

	if err != nil {
		return nil, xerrors.Errorf("prepare query: %w", err)
	}
	return &RegoAuthorizer{query: query}, nil
}

type authSubject struct {
	ID    string `json:"id"`
	Roles []Role `json:"roles"`
	Scope Role   `json:"scope"`
}

// ByRoleName will expand all roleNames into roles before calling Authorize().
// This is the function intended to be used outside this package.
// The role is fetched from the builtin map located in memory.
func (a RegoAuthorizer) ByRoleName(ctx context.Context, subjectID string, roleNames []string, scope Scope, action Action, object Object) error {
	roles, err := RolesByNames(roleNames)
	if err != nil {
		return err
	}

	scopeRole, err := ScopeRole(scope)
	if err != nil {
		return err
	}

	err = a.Authorize(ctx, subjectID, roles, scopeRole, action, object)
	if err != nil {
		return err
	}

	return nil
}

// Authorize allows passing in custom Roles.
// This is really helpful for unit testing, as we can create custom roles to exercise edge cases.
func (a RegoAuthorizer) Authorize(ctx context.Context, subjectID string, roles []Role, scope Role, action Action, object Object) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	input := map[string]interface{}{
		"subject": authSubject{
			ID:    subjectID,
			Roles: roles,
			Scope: scope,
		},
		"object": object,
		"action": action,
	}

	results, err := a.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return ForbiddenWithInternal(xerrors.Errorf("eval rego: %w", err), input, results)
	}

	// We expect only the 2 bindings for scopes and roles checks.
	if len(results) == 1 && len(results[0].Bindings) == 2 {
		roleCheck, ok := results[0].Bindings[rolesOkCheck].(bool)
		if !ok || !roleCheck {
			return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), input, results)
		}

		scopeCheck, ok := results[0].Bindings[scopeOkCheck].(bool)
		if !ok || !scopeCheck {
			return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), input, results)
		}

		// This is purely defensive programming. The two above checks already
		// check for 'true' expressions. This is just a sanity check to make
		// sure we don't add non-boolean expressions to our query.
		// This is super cheap to do, and just adds in some extra safety for
		// programmer error.
		for _, exp := range results[0].Expressions {
			if b, ok := exp.Value.(bool); !ok || !b {
				return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), input, results)
			}
		}
		return nil
	}
	return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), input, results)
}

// Prepare will partially execute the rego policy leaving the object fields unknown (except for the type).
// This will vastly speed up performance if batch authorization on the same type of objects is needed.
func (RegoAuthorizer) Prepare(ctx context.Context, subjectID string, roles []Role, scope Role, action Action, objectType string) (*PartialAuthorizer, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	auth, err := newPartialAuthorizer(ctx, subjectID, roles, scope, action, objectType)
	if err != nil {
		return nil, xerrors.Errorf("new partial authorizer: %w", err)
	}

	return auth, nil
}

func (a RegoAuthorizer) PrepareByRoleName(ctx context.Context, subjectID string, roleNames []string, scope Scope, action Action, objectType string) (PreparedAuthorized, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	roles, err := RolesByNames(roleNames)
	if err != nil {
		return nil, err
	}

	scopeRole, err := ScopeRole(scope)
	if err != nil {
		return nil, err
	}

	return a.Prepare(ctx, subjectID, roles, scopeRole, action, objectType)
}
