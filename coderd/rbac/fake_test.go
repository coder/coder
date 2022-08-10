package rbac_test

import (
	"context"

	"github.com/coder/coder/coderd/rbac"
)

type fakeAuthorizer struct {
	AuthFunc func(ctx context.Context, subjectID string, roleNames []string, action rbac.Action, object rbac.Object) error
}

func (f fakeAuthorizer) ByRoleName(ctx context.Context, subjectID string, roleNames []string, action rbac.Action, object rbac.Object) error {
	return f.AuthFunc(ctx, subjectID, roleNames, action, object)
}

func (f fakeAuthorizer) PrepareByRoleName(ctx context.Context, subjectID string, roles []string, action rbac.Action, objectType string) (rbac.PreparedAuthorized, error) {
	return &fakePreparedAuthorizer{
		Original:  f,
		SubjectID: subjectID,
		Roles:     roles,
		Action:    action,
	}, nil
}

type fakePreparedAuthorizer struct {
	Original  rbac.Authorizer
	SubjectID string
	Roles     []string
	Action    rbac.Action
}

func (f fakePreparedAuthorizer) Authorize(ctx context.Context, object rbac.Object) error {
	return f.Original.ByRoleName(ctx, f.SubjectID, f.Roles, f.Action, object)
}
