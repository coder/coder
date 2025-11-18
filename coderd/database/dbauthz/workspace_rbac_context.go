package dbauthz

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac"
)

type workspaceRBACContextKey struct{}

// WithWorkspaceRBAC attaches a workspace RBAC object to the context.
// RBAC fields on this RBAC object should not be used.
//
// This is primarily used by the workspace agent RPC handler to cache workspace
// authorization data for the duration of an agent connection.
func WithWorkspaceRBAC(ctx context.Context, rbacObj rbac.Object) (context.Context, error) {
	if rbacObj.Type != rbac.ResourceWorkspace.Type {
		return ctx, xerrors.New("RBAC Object must be of type Workspace")
	}
	if rbacObj.IsEmpty() {
		return ctx, xerrors.Errorf("cannot attach empty RBAC object to context: %+v", rbacObj)
	}
	if len(rbacObj.ACLGroupList) != 0 || len(rbacObj.ACLUserList) != 0 {
		return ctx, xerrors.New("ACL fields for Workspace RBAC object must be nullified, the can be changed during runtime and should not be cached")
	}
	return context.WithValue(ctx, workspaceRBACContextKey{}, rbacObj), nil
}

// WorkspaceRBACFromContext attempts to retrieve the workspace RBAC object from context.
func WorkspaceRBACFromContext(ctx context.Context) (rbac.Object, bool) {
	obj, ok := ctx.Value(workspaceRBACContextKey{}).(rbac.Object)
	return obj, ok
}
