package aibridgedserver

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
)

// ModelAccessReason is a bounded outcome of direct Gateway model authorization.
type ModelAccessReason string

// Model access reasons contain no organization, model, or user identifiers.
const (
	ModelAccessAllowed ModelAccessReason = "allowed"
	ModelAccessDenied  ModelAccessReason = "no_grant"
	ModelAccessError   ModelAccessReason = "evaluation_error"
)

// ModelAccess is a direct Gateway policy decision, independent of provider
// availability, key authentication, budgets, and credential availability.
type ModelAccess struct {
	Allowed bool
	Reason  ModelAccessReason
}

// ResolveModelAccess evaluates current model-use permissions constrained by the
// authenticated key's scopes. The caller must first validate the key and user.
func (s *Server) ResolveModelAccess(ctx context.Context, key database.APIKey, providerName, model string) (ModelAccess, error) {
	if s.authorizer == nil {
		return ModelAccess{Reason: ModelAccessError}, xerrors.New("model authorizer is not configured")
	}
	subject, status, err := httpmw.UserRBACSubject(ctx, s.store, key.UserID, key.ScopeSet())
	if err != nil {
		return ModelAccess{Reason: ModelAccessError}, xerrors.Errorf("load model authorization subject: %w", err)
	}
	if status != database.UserStatusActive {
		return ModelAccess{Reason: ModelAccessDenied}, nil
	}
	allowed, err := rbac.AuthorizeAIModelUse(ctx, s.authorizer, subject, func(ctx context.Context) ([]rbac.Object, error) {
		//nolint:gocritic // Resolve matching configurations without configuration read ACL filtering.
		configs, err := s.store.GetAIModelAccessConfigs(dbauthz.AsAIBridged(ctx), database.GetAIModelAccessConfigsParams{
			UserID:       key.UserID,
			ProviderName: providerName,
			Model:        model,
		})
		if err != nil {
			return nil, err
		}
		objects := make([]rbac.Object, 0, len(configs))
		for _, config := range configs {
			objects = append(objects, rbac.ResourceChatModelConfig.WithID(config.ID).InOrg(config.OrganizationID))
		}
		return objects, nil
	})
	if err != nil {
		return ModelAccess{Reason: ModelAccessError}, xerrors.Errorf("authorize model use: %w", err)
	}
	if allowed {
		return ModelAccess{Allowed: true, Reason: ModelAccessAllowed}, nil
	}
	return ModelAccess{Reason: ModelAccessDenied}, nil
}
