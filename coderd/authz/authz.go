package authz

import (
	"context"
	"golang.org/x/xerrors"
)

var ErrUnauthorized = xerrors.New("unauthorized")

// TODO: Implement Authorize. This will be implmented in mainly rego.
func Authorize(ctx context.Context, subjID string, roles []Role, obj Object, action Action) error {
	// TODO: Cache authorizer
	authorizer, err := newAuthorizer()
	if err != nil {
		return ForbiddenWithInternal(xerrors.Errorf("new authorizer: %w", err), nil)
	}

	return authorizer.Authorize(ctx, subjID, roles, obj, action)
}
