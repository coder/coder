package rbac

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// AuthorizeAIModelUse permits unrestricted Gateway use or use of an eligible
// model configuration. Configurations are loaded only when unrestricted use is
// denied. The caller supplies live configurations matching the invocation target;
// configuration read ACLs are not model-use permissions.
func AuthorizeAIModelUse(ctx context.Context, auth Authorizer, subject Subject, configurations func(context.Context) ([]Object, error)) (bool, error) {
	err := auth.Authorize(ctx, subject, policy.ActionUse, ResourceAIGatewayUnrestricted)
	if err == nil {
		return true, nil
	}
	if !IsUnauthorizedError(err) {
		return false, err
	}
	configs, loadErr := configurations(ctx)
	if loadErr != nil {
		return false, xerrors.Errorf("load model configurations: %w", loadErr)
	}
	for _, config := range configs {
		err = auth.Authorize(ctx, subject, policy.ActionUse, config)
		if err == nil {
			return true, nil
		}
		if !IsUnauthorizedError(err) {
			return false, err
		}
	}
	return false, nil
}
