package aibridgedserver

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/database"
)

// ErrAmbiguousOrganizationInput is returned when a request carries more than
// one organization input. The login paths are mutually exclusive.
var ErrAmbiguousOrganizationInput = xerrors.New("workspace and chat organization inputs are mutually exclusive")

// orgResolverInput holds one reference per login path. At most one is set.
// Each is a reference, not an organization: resolveOrganization looks up the
// organization from it, so callers never supply an organization ID.
type orgResolverInput struct {
	// WorkspaceID is set on the direct path, for a workspace session token.
	WorkspaceID uuid.NullUUID
	// ChatID is set on the delegated path, for a chatd request.
	// TODO(AIGOV-713): send the chat ID in IsAuthorizedRequest, set it here,
	// and resolve it to the chat's organization below.
	ChatID uuid.NullUUID
}

// resolvedOrganization is the organization a request acts in.
type resolvedOrganization struct {
	ID     uuid.UUID
	Source database.AIBridgeInterceptionOrganizationSource
}

// resolveOrganization determines the organization an AI Gateway request acts
// in. All org resolution for AI Gateway requests lives here. It runs once per
// request, in IsAuthorized. The budget check, the interception record and
// cost reuse the result.
//
// The inputs are tried in order:
//  1. A workspace session token resolves to the workspace's organization. The
//     workspace must still exist and userID must still be a member of its
//     organization.
//  2. A chat resolves to the chat's organization (AIGOV-713).
//  3. Otherwise, the organization of the user's site-wide effective group.
//
// Every AI Gateway request is scoped to an organization, so it returns
// [ErrNoOrganization] when none resolves, for example when the user belongs to
// no live organization.
func (s *Server) resolveOrganization(ctx context.Context, userID uuid.UUID, in orgResolverInput) (resolvedOrganization, error) {
	if in.WorkspaceID.Valid && in.ChatID.Valid {
		return resolvedOrganization{}, ErrAmbiguousOrganizationInput
	}

	if in.WorkspaceID.Valid {
		orgID, err := s.workspaceOrganization(ctx, userID, in.WorkspaceID.UUID)
		if err != nil {
			return resolvedOrganization{}, err
		}
		return resolvedOrganization{ID: orgID, Source: database.AibridgeInterceptionOrganizationSourceWorkspace}, nil
	}

	group, ok, err := budget.ResolveUserEffectiveGroup(ctx, s.store, userID, uuid.NullUUID{}, s.budgetPolicy)
	if err != nil {
		return resolvedOrganization{}, xerrors.Errorf("resolve effective AI group for user %s: %w", userID, err)
	}
	if !ok {
		return resolvedOrganization{}, ErrNoOrganization
	}
	return resolvedOrganization{ID: group.OrganizationID, Source: database.AibridgeInterceptionOrganizationSourceEffectiveGroup}, nil
}

// workspaceOrganization returns the organization of workspaceID. It rejects the
// request when the workspace is missing or deleted, or when userID is not a
// member of its organization.
func (s *Server) workspaceOrganization(ctx context.Context, userID, workspaceID uuid.UUID) (uuid.UUID, error) {
	workspace, err := s.store.GetWorkspaceByID(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrUnknownWorkspace
	}
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get workspace %s: %w", workspaceID, err)
	}
	// Workspaces are soft-deleted, so a deleted row still resolves by ID.
	if workspace.Deleted {
		return uuid.Nil, ErrUnknownWorkspace
	}

	orgs, err := s.store.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
		UserID:  userID,
		Deleted: sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get organizations for user %s: %w", userID, err)
	}
	if !slices.ContainsFunc(orgs, func(org database.Organization) bool {
		return org.ID == workspace.OrganizationID
	}) {
		return uuid.Nil, ErrUnknownOrganizationMember
	}

	return workspace.OrganizationID, nil
}
