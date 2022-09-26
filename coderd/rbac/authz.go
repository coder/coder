package rbac

import (
	"context"
	_ "embed"
	"sync"

	"github.com/open-policy-agent/opa/rego"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/tracing"
)

type Authorizer interface {
	ByRoleName(ctx context.Context, subjectID string, roleNames []string, scope Scope, groups []string, action Action, object Object) error
	PrepareByRoleName(ctx context.Context, subjectID string, roleNames []string, scope Scope, groups []string, action Action, objectType string) (PreparedAuthorized, error)
}

type PreparedAuthorized interface {
	Authorize(ctx context.Context, object Object) error
}

// Filter takes in a list of objects, and will filter the list removing all
// the elements the subject does not have permission for. This function slows
// down if the list contains objects of multiple types. Attempt to only
// filter objects of the same type for faster performance.
func Filter[O Objecter](ctx context.Context, auth Authorizer, subjID string, subjRoles []string, scope Scope, groups []string, action Action, objects []O) ([]O, error) {
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

	filtered := make([]O, 0)
	prepared := make(map[string]PreparedAuthorized)

	for i := range objects {
		object := objects[i]
		objectType := object.RBACObject().Type
		// objectAuth is the prepared authorization for the object type.
		objectAuth, ok := prepared[object.RBACObject().Type]
		if !ok {
			var err error
			objectAuth, err = auth.PrepareByRoleName(ctx, subjID, subjRoles, scope, groups, action, objectType)
			if err != nil {
				return nil, xerrors.Errorf("prepare: %w", err)
			}
			prepared[objectType] = objectAuth
		}

		rbacObj := object.RBACObject()
		err := objectAuth.Authorize(ctx, rbacObj)
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

var (
	// Load the policy from policy.rego in this directory.
	//
	//go:embed policy.rego
	policy    string
	queryOnce sync.Once
	query     rego.PreparedEvalQuery
)

const (
	rolesOkCheck = "role_ok"
	scopeOkCheck = "scope_ok"
)

func NewAuthorizer() *RegoAuthorizer {
	queryOnce.Do(func() {
		var err error
		query, err = rego.New(
			rego.Query("data.authz.allow"),
			rego.Module("policy.rego", policy),
		).PrepareForEval(context.Background())
		if err != nil {
			panic(xerrors.Errorf("compile rego: %w", err))
		}
	})
	return &RegoAuthorizer{query: query}
}

type authSubject struct {
	ID     string   `json:"id"`
	Roles  []Role   `json:"roles"`
	Groups []string `json:"groups"`
	Scope  Role     `json:"scope"`
}

// ByRoleName will expand all roleNames into roles before calling Authorize().
// This is the function intended to be used outside this package.
// The role is fetched from the builtin map located in memory.
func (a RegoAuthorizer) ByRoleName(ctx context.Context, subjectID string, roleNames []string, scope Scope, groups []string, action Action, object Object) error {
	roles, err := RolesByNames(roleNames)
	if err != nil {
		return err
	}

	scopeRole, err := ScopeRole(scope)
	if err != nil {
		return err
	}

	err = a.Authorize(ctx, subjectID, roles, scopeRole, groups, action, object)
	if err != nil {
		return err
	}

	return nil
}

// Authorize allows passing in custom Roles.
// This is really helpful for unit testing, as we can create custom roles to exercise edge cases.
func (a RegoAuthorizer) Authorize(ctx context.Context, subjectID string, roles []Role, scope Role, groups []string, action Action, object Object) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	input := map[string]interface{}{
		"subject": authSubject{
			ID:     subjectID,
			Roles:  roles,
			Groups: groups,
			Scope:  scope,
		},
		"object": object,
		"action": action,
	}

	results, err := a.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return ForbiddenWithInternal(xerrors.Errorf("eval rego: %w", err), input, results)
	}

	if !results.Allowed() {
		return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), input, results)
	}
	return nil
}

// Prepare will partially execute the rego policy leaving the object fields unknown (except for the type).
// This will vastly speed up performance if batch authorization on the same type of objects is needed.
func (RegoAuthorizer) Prepare(ctx context.Context, subjectID string, roles []Role, scope Role, groups []string, action Action, objectType string) (*PartialAuthorizer, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	auth, err := newPartialAuthorizer(ctx, subjectID, roles, scope, groups, action, objectType)
	if err != nil {
		return nil, xerrors.Errorf("new partial authorizer: %w", err)
	}

	return auth, nil
}

func (a RegoAuthorizer) PrepareByRoleName(ctx context.Context, subjectID string, roleNames []string, scope Scope, groups []string, action Action, objectType string) (PreparedAuthorized, error) {
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

	return a.Prepare(ctx, subjectID, roles, scopeRole, groups, action, objectType)
}
