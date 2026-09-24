package aibridgedserver

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// authorizeModelUse evaluates current model-use permissions constrained by the
// authenticated key's scopes. The caller must first validate the key and user.
func (s *Server) authorizeModelUse(ctx context.Context, key database.APIKey, providerName, model string) (bool, error) {
	if s.authorizer == nil {
		return false, xerrors.New("model authorizer is not configured")
	}
	subject, status, err := httpmw.UserRBACSubject(ctx, s.store, key.UserID, key.ScopeSet())
	if err != nil {
		return false, xerrors.Errorf("load model authorization subject: %w", err)
	}
	if status != database.UserStatusActive {
		return false, nil
	}

	// Unrestricted grants do not depend on configuration availability.
	err = s.authorizer.Authorize(ctx, subject, policy.ActionUse, rbac.ResourceAIGatewayUnrestricted)
	if err == nil {
		return true, nil
	}
	if !rbac.IsUnauthorizedError(err) {
		return false, xerrors.Errorf("authorize unrestricted model use: %w", err)
	}

	//nolint:gocritic // Resolve matching configurations without configuration read ACL filtering.
	configs, err := s.store.GetAIModelAccessConfigs(dbauthz.AsAIBridged(ctx), database.GetAIModelAccessConfigsParams{
		UserID:       key.UserID,
		ProviderName: providerName,
		Model:        model,
	})
	if err != nil {
		return false, xerrors.Errorf("load model configurations: %w", err)
	}
	for _, config := range configs {
		err = s.authorizer.Authorize(ctx, subject, policy.ActionUse, rbac.ResourceChatModelConfig.WithID(config.ID).InOrg(config.OrganizationID))
		if err == nil {
			return true, nil
		}
		if !rbac.IsUnauthorizedError(err) {
			return false, xerrors.Errorf("authorize configured model use: %w", err)
		}
	}
	return false, nil
}
