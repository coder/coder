package rbac

import "context"

type FakeAuthorizer struct {
	AuthFunc func(ctx context.Context, subjectID string, roleNames []string, action Action, object Object) error
}

func (f FakeAuthorizer) ByRoleName(ctx context.Context, subjectID string, roleNames []string, action Action, object Object) error {
	return f.AuthFunc(ctx, subjectID, roleNames, action, object)
}

func (f FakeAuthorizer) PrepareByRoleName(_ context.Context, subjectID string, roles []string, action Action, _ string) (PreparedAuthorized, error) {
	return &fakePreparedAuthorizer{
		Original:  f,
		SubjectID: subjectID,
		Roles:     roles,
		Action:    action,
	}, nil
}

type fakePreparedAuthorizer struct {
	Original  Authorizer
	SubjectID string
	Roles     []string
	Action    Action
}

func (f fakePreparedAuthorizer) Authorize(ctx context.Context, object Object) error {
	return f.Original.ByRoleName(ctx, f.SubjectID, f.Roles, f.Action, object)
}
