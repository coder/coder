package aibridgedserver

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// ModelAccessReason is a bounded outcome of direct Gateway model authorization.
type ModelAccessReason string

// Model access reasons contain no organization, model, or user identifiers.
const (
	ModelAccessNotEntitled ModelAccessReason = "not_entitled"
	ModelAccessAllowed     ModelAccessReason = "allowed"
	ModelAccessDenied      ModelAccessReason = "no_grant"
	ModelAccessError       ModelAccessReason = "evaluation_error"
)

// ModelAccess is a direct Gateway policy decision, independent of provider
// availability, key authentication, budgets, and credential availability.
type ModelAccess struct {
	Allowed bool
	Reason  ModelAccessReason
}

// ResolveModelAccess evaluates current organization grants for an authenticated
// user. The caller must first validate the key and active, non-system user.
// Database evaluation failures return an error rather than an implicit grant.
func (s *Server) ResolveModelAccess(ctx context.Context, userID uuid.UUID, providerName, model string) (ModelAccess, error) {
	if s.entitlements == nil || !s.entitlements.Enabled(codersdk.FeatureMultipleOrganizations) {
		return ModelAccess{Allowed: true, Reason: ModelAccessNotEntitled}, nil
	}

	//nolint:gocritic // Gateway authorization evaluates unfiltered organization grants.
	ctx = dbauthz.AsAIBridged(ctx)
	allowed, err := s.store.HasAIModelAccess(ctx, database.HasAIModelAccessParams{
		UserID:       userID,
		ProviderName: providerName,
		Model:        model,
	})
	if err != nil {
		return ModelAccess{Reason: ModelAccessError}, xerrors.Errorf("resolve organization model access: %w", err)
	}
	if allowed {
		return ModelAccess{Allowed: true, Reason: ModelAccessAllowed}, nil
	}
	return ModelAccess{Reason: ModelAccessDenied}, nil
}
