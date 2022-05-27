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
