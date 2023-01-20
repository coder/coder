package authzquery

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

// A different syntax idea

type authz[ObjectType rbac.Objecter, Argument any] struct {
	DBQuery func(ctx context.Context, arg Argument) (ObjectType, error)
	DBExec  func(ctx context.Context, arg Argument) error

	authorizer rbac.Authorizer
	object     ObjectType
	err        error
}

func (a *authz[_, _]) Error() error {
	return a.err
}

func (a *authz[ObjectType, _]) Object() ObjectType {
	return a.object
}

func (a *authz[ObjectType, Argument]) Authorize(ctx context.Context, action rbac.Action) *authz[ObjectType, Argument] {
	if a.err != nil {
		return a
	}

	act, ok := actorFromContext(ctx)
	if !ok {
		a.err = xerrors.Errorf("no authorization actor in context")
		return a
	}

	err := a.authorizer.ByRoleName(ctx, act.ID.String(), act.Roles, act.Scope, act.Groups, action, a.object.RBACObject())
	if err != nil {
		a.err = xerrors.Errorf("unauthorized: %w", err)
		return a
	}

	return a
}

func (a *authz[ObjectType, Argument]) Query(ctx context.Context, arg Argument) *authz[ObjectType, Argument] {
	if a.err != nil {
		return a
	}

	queried, err := a.DBQuery(ctx, arg)
	if err != nil {
		a.err = err
	}
	a.object = queried

	return a
}

func (a *authz[_, Argument]) Exec(ctx context.Context, arg Argument) *authz[_, Argument] {
	if a.err != nil {
		return a
	}

	err := a.DBExec(ctx, arg)
	if err != nil {
		a.err = err
	}

	return a
}

func (q *AuthzQuerier) UpdateTemplateActiveVersionByID2(ctx context.Context, arg database.UpdateTemplateActiveVersionByIDParams) error {
	a := authz[database.Template, database.UpdateTemplateActiveVersionByIDParams]{
		authorizer: q.authorizer,
		DBQuery: func(ctx context.Context, arg database.UpdateTemplateActiveVersionByIDParams) (database.Template, error) {
			return q.database.GetTemplateByID(ctx, arg.ID)
		},
		DBExec: q.database.UpdateTemplateActiveVersionByID,
	}

	return a.Query(ctx, arg).Authorize(ctx, rbac.ActionRead).Exec(ctx, arg).Error()
}
